package oracle

import (
	"context"
	b64 "encoding/base64"
	"errors"
	"fmt"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/autoscaler/providers/oracle/ociapi"
	"go.woodpecker-ci.org/autoscaler/utils"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var blackholeMetadataAPI = []string{
	"ip -4 route add blackhole 169.254.169.254/32",
}

type provider struct {
	name                string
	config              *config.Config
	computeClient       ociapi.ComputeClient
	identityClient      ociapi.IdentityClient
	compartmentID       string
	subnetID            string
	availabilityDomains []string
	shape               string
	shapeIsFlexible     bool
	ocpus               float32
	memoryInGBs         float32
	imageID             string
	bootVolumeSizeInGBs int64
	assignPublicIP      bool
	sshKey              string
	tags                map[string]string
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (types.Provider, error) {
	p := &provider{
		name:                "oracle",
		config:              config,
		compartmentID:       c.String("oracle-compartment-ocid"),
		subnetID:            c.String("oracle-subnet-ocid"),
		shape:               c.String("oracle-shape"),
		ocpus:               c.Float32("oracle-ocpus"),
		memoryInGBs:         c.Float32("oracle-memory-in-gbs"),
		bootVolumeSizeInGBs: c.Int64("oracle-boot-volume-size"),
		assignPublicIP:      c.Bool("oracle-assign-public-ip"),
		sshKey:              c.String("oracle-ssh-key"),
	}

	if p.compartmentID == "" {
		return nil, fmt.Errorf("%s: %w", p.name, ErrCompartmentNotSet)
	}
	if p.subnetID == "" {
		return nil, fmt.Errorf("%s: %w", p.name, ErrSubnetNotSet)
	}
	if p.bootVolumeSizeInGBs < minBootVolumeGB {
		return nil, fmt.Errorf("%s: %w: %d is below %d GB",
			p.name, ErrBootVolumeTooSmall, p.bootVolumeSizeInGBs, minBootVolumeGB)
	}

	sanitizedTags, userTags, err := parseTags(c.StringSlice("oracle-tags"))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}
	if err := utils.CheckReservedTags(sanitizedTags, tagKeyPrefix(), ErrReservedTagPrefix); err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}
	userTags[poolTagKey()] = config.PoolID
	if len(userTags) > maxFreeformTags {
		return nil, fmt.Errorf("%s: %w: oracle cloud allows at most %d freeform tags per instance, got %d",
			p.name, ErrInvalidTag, maxFreeformTags, len(userTags))
	}
	p.tags = userTags

	configProvider, err := newConfigurationProvider(c)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}

	computeClient, err := core.NewComputeClientWithConfigurationProvider(configProvider)
	if err != nil {
		return nil, fmt.Errorf("%s: NewComputeClientWithConfigurationProvider: %w", p.name, err)
	}
	p.computeClient = computeClient

	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(configProvider)
	if err != nil {
		return nil, fmt.Errorf("%s: NewIdentityClientWithConfigurationProvider: %w", p.name, err)
	}
	p.identityClient = identityClient

	if err := p.resolveAvailabilityDomains(ctx, c.StringSlice("oracle-availability-domains")); err != nil {
		return nil, err
	}
	if err := p.resolveShape(ctx); err != nil {
		return nil, err
	}
	if err := p.resolveImage(ctx,
		c.String("oracle-image-ocid"),
		c.String("oracle-image-operating-system"),
		c.String("oracle-image-operating-system-version"),
	); err != nil {
		return nil, err
	}

	log.Info().
		Strs("availability_domains", p.availabilityDomains).
		Str("shape", p.shape).
		Bool("shape_is_flexible", p.shapeIsFlexible).
		Str("image_id", p.imageID).
		Msgf("%s: provider ready", p.name)

	return p, nil
}

func (p *provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	metadata, err := p.instanceMetadata(agent)
	if err != nil {
		return err
	}

	var errs []error
	for _, availabilityDomain := range p.availabilityDomains {
		_, err := p.computeClient.LaunchInstance(ctx, core.LaunchInstanceRequest{
			LaunchInstanceDetails: p.launchDetails(agent, availabilityDomain, metadata),
		})
		if err == nil {
			log.Info().Str("availability_domain", availabilityDomain).
				Msgf("%s: create agent %s", p.name, agent.Name)
			return nil
		}
		if !isCapacityError(err) {
			return fmt.Errorf("%s: LaunchInstance: %w", p.name, err)
		}

		log.Warn().Str("availability_domain", availabilityDomain).
			Msgf("%s: no capacity for shape %s, trying next availability domain: %s", p.name, p.shape, err)
		errs = append(errs, err)
	}

	return fmt.Errorf("%s: %w: %w", p.name, ErrAllAvailabilityDomainsOut, errors.Join(errs...))
}

func (p *provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	inst, err := p.getInstance(ctx, agent.Name)
	if err != nil {
		if errors.Is(err, ErrInstanceNotFound) {
			log.Warn().Str("agent", agent.Name).Msgf("%s: instance not found, nothing to remove", p.name)
			return nil
		}
		return err
	}

	_, err = p.computeClient.TerminateInstance(ctx, core.TerminateInstanceRequest{
		InstanceId:         inst.Id,
		PreserveBootVolume: common.Bool(false),
	})
	if err != nil {
		return fmt.Errorf("%s: TerminateInstance: %w", p.name, err)
	}

	return nil
}

func (p *provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	instances, err := p.listInstances(ctx, "")
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(instances))
	for _, inst := range instances {
		names = append(names, *inst.DisplayName)
	}

	return names, nil
}

func (p *provider) BillingModel() types.BillingModel {
	return types.BillingPerSecond
}

func (p *provider) instanceMetadata(agent *woodpecker.Agent) (map[string]string, error) {
	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, cloudinit.RenderOption{
		PreExec: blackholeMetadataAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: cloudinit.RenderUserDataTemplate: %w", p.name, err)
	}

	metadata := map[string]string{
		metadataUserKey: b64.StdEncoding.EncodeToString([]byte(userData)),
	}
	if p.sshKey != "" {
		metadata[metadataSSHKey] = p.sshKey
	}

	return metadata, nil
}

func (p *provider) launchDetails(agent *woodpecker.Agent, availabilityDomain string, metadata map[string]string) core.LaunchInstanceDetails {
	details := core.LaunchInstanceDetails{
		AvailabilityDomain: common.String(availabilityDomain),
		CompartmentId:      common.String(p.compartmentID),
		DisplayName:        common.String(agent.Name),
		Shape:              common.String(p.shape),
		FreeformTags:       p.tags,
		Metadata:           metadata,
		CreateVnicDetails: &core.CreateVnicDetails{
			SubnetId:       common.String(p.subnetID),
			AssignPublicIp: common.Bool(p.assignPublicIP),
		},
		SourceDetails: core.InstanceSourceViaImageDetails{
			ImageId:             common.String(p.imageID),
			BootVolumeSizeInGBs: common.Int64(p.bootVolumeSizeInGBs),
		},
	}

	if p.shapeIsFlexible {
		details.ShapeConfig = &core.LaunchInstanceShapeConfigDetails{
			Ocpus:       common.Float32(p.ocpus),
			MemoryInGBs: common.Float32(p.memoryInGBs),
		}
	}

	return details
}
