package oracle

import (
	"context"
	b64 "encoding/base64"
	"errors"
	"fmt"
	"slices"

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

// OCI also serves DNS and NTP on 169.254.169.254, so only TCP outside port 53 is
// dropped. The raw table runs before docker's FORWARD rules, so steps are covered too.
var blockMetadataAPI = []string{
	"iptables -t raw -A PREROUTING -d 169.254.169.254/32 -p tcp ! --dport 53 -j DROP",
	"iptables -t raw -A OUTPUT -d 169.254.169.254/32 -p tcp ! --dport 53 -j DROP",
}

type provider struct {
	name                string
	config              *config.Config
	computeClient       ociapi.ComputeClient
	identityClient      ociapi.IdentityClient
	networkClient       ociapi.NetworkClient
	compartmentID       string
	subnetID            string
	availabilityDomains []string
	shapes              []shapeCandidate
	ocpus               float32
	memoryInGBs         float32
	bootVolumeSizeInGBs int64
	assignPublicIP      bool
	sshKey              string
	tags                map[string]string
}

type shapeCandidate struct {
	name     string
	flexible bool
	platform string
	imageID  string
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (types.Provider, error) {
	p := &provider{
		name:                "oracle",
		config:              config,
		compartmentID:       c.String("oracle-compartment-ocid"),
		subnetID:            c.String("oracle-subnet-ocid"),
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

	networkClient, err := core.NewVirtualNetworkClientWithConfigurationProvider(configProvider)
	if err != nil {
		return nil, fmt.Errorf("%s: NewVirtualNetworkClientWithConfigurationProvider: %w", p.name, err)
	}
	p.networkClient = networkClient

	if err := p.resolveAvailabilityDomains(ctx, c.StringSlice("oracle-availability-domains")); err != nil {
		return nil, err
	}
	if err := p.resolveSubnet(ctx); err != nil {
		return nil, err
	}
	if err := p.resolveShapes(ctx, c.StringSlice("oracle-shapes")); err != nil {
		return nil, err
	}
	if err := p.resolveImages(ctx,
		c.String("oracle-image-ocid"),
		c.String("oracle-image-operating-system"),
		c.String("oracle-image-operating-system-version"),
	); err != nil {
		return nil, err
	}

	for _, shape := range p.shapes {
		log.Info().
			Str("shape", shape.name).
			Bool("flexible", shape.flexible).
			Str("platform", shape.platform).
			Str("image_id", shape.imageID).
			Msgf("%s: shape ready", p.name)
	}
	log.Info().
		Strs("availability_domains", p.availabilityDomains).
		Msgf("%s: provider ready", p.name)

	return p, nil
}

func (p *provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent, capability types.Capability) error {
	if capability.Backend != types.BackendDocker {
		return fmt.Errorf("%s: only the docker backend is supported, got %q", p.name, capability.Backend)
	}

	metadata, err := p.instanceMetadata(agent)
	if err != nil {
		return err
	}

	matched := false
	var errs []error
	for _, shape := range p.shapes {
		if shape.platform != capability.Platform {
			continue
		}
		matched = true

		for _, availabilityDomain := range p.availabilityDomains {
			_, err := p.computeClient.LaunchInstance(ctx, core.LaunchInstanceRequest{
				LaunchInstanceDetails: p.launchDetails(agent, shape, availabilityDomain, metadata),
			})
			if err == nil {
				log.Info().Str("shape", shape.name).Str("availability_domain", availabilityDomain).
					Msgf("%s: create agent %s", p.name, agent.Name)
				return nil
			}
			if !isCapacityError(err) {
				return fmt.Errorf("%s: LaunchInstance: %w", p.name, err)
			}

			log.Warn().Str("shape", shape.name).Str("availability_domain", availabilityDomain).
				Msgf("%s: no capacity, trying next candidate: %s", p.name, err)
			errs = append(errs, err)
		}
	}
	if !matched {
		return fmt.Errorf("%s: %w: %s", p.name, ErrNoMatchingShape, capability.Platform)
	}

	return fmt.Errorf("%s: %w: %w", p.name, ErrNoCapacity, errors.Join(errs...))
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

func (p *provider) Capabilities(_ context.Context) ([]types.Capability, error) {
	var caps []types.Capability
	for _, shape := range p.shapes {
		capability := types.Capability{Platform: shape.platform, Backend: types.BackendDocker}
		if !slices.Contains(caps, capability) {
			caps = append(caps, capability)
		}
	}
	return caps, nil
}

func (p *provider) BillingModel() types.BillingModel {
	return types.BillingPerSecond
}

func (p *provider) instanceMetadata(agent *woodpecker.Agent) (map[string]string, error) {
	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, cloudinit.RenderOption{
		PreExec: blockMetadataAPI,
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

func (p *provider) launchDetails(agent *woodpecker.Agent, shape shapeCandidate, availabilityDomain string, metadata map[string]string) core.LaunchInstanceDetails {
	details := core.LaunchInstanceDetails{
		AvailabilityDomain: common.String(availabilityDomain),
		CompartmentId:      common.String(p.compartmentID),
		DisplayName:        common.String(agent.Name),
		Shape:              common.String(shape.name),
		FreeformTags:       p.tags,
		Metadata:           metadata,
		CreateVnicDetails: &core.CreateVnicDetails{
			SubnetId:       common.String(p.subnetID),
			AssignPublicIp: common.Bool(p.assignPublicIP),
		},
		SourceDetails: core.InstanceSourceViaImageDetails{
			ImageId:             common.String(shape.imageID),
			BootVolumeSizeInGBs: common.Int64(p.bootVolumeSizeInGBs),
		},
	}

	if shape.flexible {
		details.ShapeConfig = &core.LaunchInstanceShapeConfigDetails{
			Ocpus:       common.Float32(p.ocpus),
			MemoryInGBs: common.Float32(p.memoryInGBs),
		}
	}

	return details
}
