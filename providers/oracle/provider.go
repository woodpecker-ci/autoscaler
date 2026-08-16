package oracle

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/autoscaler/utils"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type provider struct {
	name               string
	config             *config.Config
	client             computeAPI
	compartmentID      string
	availabilityDomain string
	subnetID           string
	shape              string
	ocpus              float32
	memoryGBs          float32
	imageID            string
	sshAuthorizedKeys  string
	assignPublicIP     bool
	tags               map[string]string
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (types.Provider, error) {
	p := &provider{
		name:               "oracle",
		config:             config,
		compartmentID:      c.String("oracle-compartment-id"),
		availabilityDomain: c.String("oracle-availability-domain"),
		subnetID:           c.String("oracle-subnet-id"),
		shape:              c.String("oracle-shape"),
		ocpus:              float32(c.Float("oracle-ocpus")),
		memoryGBs:          float32(c.Float("oracle-memory-gbs")),
		imageID:            c.String("oracle-image-id"),
		sshAuthorizedKeys:  c.String("oracle-ssh-authorized-keys"),
		assignPublicIP:     c.Bool("oracle-assign-public-ip"),
	}

	operatingSystem := c.String("oracle-operating-system")
	operatingSystemVersion := c.String("oracle-operating-system-version")

	if err := p.validate(operatingSystem, operatingSystemVersion); err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}

	tags, err := parseFreeformTags(c.StringSlice("oracle-freeform-tags"))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}

	configProvider, err := newConfigurationProvider(c)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}
	client, err := core.NewComputeClientWithConfigurationProvider(configProvider)
	if err != nil {
		return nil, fmt.Errorf("%s: NewComputeClient: %w", p.name, err)
	}
	if region := c.String("oracle-region"); region != "" {
		client.SetRegion(region)
	}
	p.client = client

	if err := p.setup(ctx, operatingSystem, operatingSystemVersion, tags); err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}

	return p, nil
}

func (p *provider) validate(operatingSystem, operatingSystemVersion string) error {
	switch {
	case p.compartmentID == "":
		return ErrCompartmentIDRequired
	case p.availabilityDomain == "":
		return ErrAvailabilityDomainRequired
	case p.subnetID == "":
		return ErrSubnetIDRequired
	case p.shape == "":
		return ErrShapeRequired
	case p.imageID == "" && (operatingSystem == "" || operatingSystemVersion == ""):
		return ErrImageRequired
	}

	return nil
}

// setup resolves the image and assembles the instance tags. It is separate
// from New so tests can exercise it against a fake compute client.
func (p *provider) setup(ctx context.Context, operatingSystem, operatingSystemVersion string, tags map[string]string) error {
	if p.imageID == "" {
		imageID, err := p.resolveImage(ctx, operatingSystem, operatingSystemVersion)
		if err != nil {
			return err
		}
		p.imageID = imageID
		log.Info().Str("image", p.imageID).Msgf("%s: resolved %s %s image", p.name, operatingSystem, operatingSystemVersion)
	}

	p.tags = utils.MergeMaps(map[string]string{
		tagPool:  p.config.PoolID,
		tagImage: p.imageID,
	}, tags)

	return nil
}

func (p *provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, cloudinit.RenderOption{})
	if err != nil {
		return fmt.Errorf("%s: cloudinit.RenderUserDataTemplate: %w", p.name, err)
	}

	metadata := map[string]string{
		"user_data": base64.StdEncoding.EncodeToString([]byte(userData)),
	}
	if p.sshAuthorizedKeys != "" {
		metadata["ssh_authorized_keys"] = p.sshAuthorizedKeys
	}

	details := core.LaunchInstanceDetails{
		DisplayName:        &agent.Name,
		CompartmentId:      &p.compartmentID,
		AvailabilityDomain: &p.availabilityDomain,
		Shape:              &p.shape,
		SourceDetails: core.InstanceSourceViaImageDetails{
			ImageId: &p.imageID,
		},
		CreateVnicDetails: &core.CreateVnicDetails{
			SubnetId:       &p.subnetID,
			AssignPublicIp: &p.assignPublicIP,
		},
		Metadata:     metadata,
		FreeformTags: p.tags,
	}
	if isFlexShape(p.shape) {
		details.ShapeConfig = &core.LaunchInstanceShapeConfigDetails{}
		if p.ocpus > 0 {
			details.ShapeConfig.Ocpus = &p.ocpus
		}
		if p.memoryGBs > 0 {
			details.ShapeConfig.MemoryInGBs = &p.memoryGBs
		}
	}

	if _, err := p.client.LaunchInstance(ctx, core.LaunchInstanceRequest{LaunchInstanceDetails: details}); err != nil {
		return fmt.Errorf("%s: LaunchInstance: %w", p.name, err)
	}

	return nil
}

func (p *provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	instances, err := p.listPoolInstances(ctx, agent.Name)
	if err != nil {
		return fmt.Errorf("%s: %w", p.name, err)
	}

	if len(instances) == 0 {
		return nil
	}
	if len(instances) > 1 {
		return fmt.Errorf("%s: %w: %s", p.name, ErrMultipleInstances, agent.Name)
	}

	if _, err := p.client.TerminateInstance(ctx, core.TerminateInstanceRequest{InstanceId: instances[0].Id}); err != nil {
		return fmt.Errorf("%s: TerminateInstance: %w", p.name, err)
	}

	return nil
}

func (p *provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	instances, err := p.listPoolInstances(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}

	names := make([]string, 0, len(instances))
	for _, instance := range instances {
		if instance.DisplayName != nil {
			names = append(names, *instance.DisplayName)
		}
	}

	return names, nil
}

func (p *provider) BillingModel() types.BillingModel {
	return types.BillingPerSecond
}
