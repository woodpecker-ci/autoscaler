package oracle

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/urfave/cli/v3"
	"golang.org/x/exp/maps"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/autoscaler/utils"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var (
	ErrIllegalLabelPrefix = errors.New("illegal label prefix")
	ErrParameterNotSet    = errors.New("required parameter not set")
)

type computeClient interface {
	LaunchInstance(context.Context, core.LaunchInstanceRequest) (core.LaunchInstanceResponse, error)
	TerminateInstance(context.Context, core.TerminateInstanceRequest) (core.TerminateInstanceResponse, error)
	ListInstances(context.Context, core.ListInstancesRequest) (core.ListInstancesResponse, error)
}

type Provider struct {
	name               string
	compartmentID      string
	availabilityDomain string
	subnetID           string
	imageID            string
	shape              string
	ocpus              float32
	memoryInGBs        float32
	sshAuthorizedKey   string
	assignPublicIP     bool
	tags               map[string]string
	config             *config.Config
	client             computeClient
}

func New(_ context.Context, c *cli.Command, config *config.Config) (types.Provider, error) {
	p := &Provider{
		name:               "oracle",
		compartmentID:      c.String("oracle-compartment-id"),
		availabilityDomain: c.String("oracle-availability-domain"),
		subnetID:           c.String("oracle-subnet-id"),
		imageID:            c.String("oracle-image-id"),
		shape:              c.String("oracle-shape"),
		ocpus:              float32(c.Float("oracle-ocpus")),
		memoryInGBs:        float32(c.Float("oracle-memory-gbs")),
		sshAuthorizedKey:   c.String("oracle-ssh-authorized-key"),
		assignPublicIP:     c.Bool("oracle-assign-public-ip"),
		config:             config,
	}

	if err := p.validateConfig(); err != nil {
		return nil, err
	}

	defaultTags := map[string]string{
		engine.LabelPool:  config.PoolID,
		engine.LabelImage: p.imageID,
	}
	tags, err := utils.SliceToMap(c.StringSlice("oracle-freeform-tags"), "=")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}
	for _, key := range maps.Keys(tags) {
		if strings.HasPrefix(key, engine.LabelPrefix) {
			return nil, fmt.Errorf("%s: %w: %s", p.name, ErrIllegalLabelPrefix, engine.LabelPrefix)
		}
	}
	p.tags = utils.MergeMaps(defaultTags, tags)

	client, err := newComputeClient(c)
	if err != nil {
		return nil, fmt.Errorf("%s: new compute client: %w", p.name, err)
	}
	p.client = client

	return p, nil
}

func (p *Provider) validateConfig() error {
	required := map[string]string{
		"oracle-compartment-id":      p.compartmentID,
		"oracle-availability-domain": p.availabilityDomain,
		"oracle-subnet-id":           p.subnetID,
		"oracle-image-id":            p.imageID,
		"oracle-shape":               p.shape,
		"oracle-ssh-authorized-key":  p.sshAuthorizedKey,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: %s", ErrParameterNotSet, name)
		}
	}
	return nil
}

func newComputeClient(c *cli.Command) (computeClient, error) {
	provider := common.DefaultConfigProvider()
	if configFile := c.String("oracle-config-file"); configFile != "" {
		provider = common.CustomProfileConfigProvider(configFile, c.String("oracle-profile"))
	}
	if region := c.String("oracle-region"); region != "" {
		provider = regionOverrideProvider{
			ConfigurationProvider: provider,
			region:                region,
		}
	}

	return core.NewComputeClientWithConfigurationProvider(provider)
}

type regionOverrideProvider struct {
	common.ConfigurationProvider
	region string
}

func (p regionOverrideProvider) Region() (string, error) {
	return p.region, nil
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, nil)
	if err != nil {
		return fmt.Errorf("%s: cloudinit.RenderUserDataTemplate: %w", p.name, err)
	}

	metadata := map[string]string{
		"ssh_authorized_keys": p.sshAuthorizedKey,
		"user_data":           base64.StdEncoding.EncodeToString([]byte(userData)),
	}

	shapeConfig := &core.LaunchInstanceShapeConfigDetails{}
	if p.ocpus > 0 {
		shapeConfig.Ocpus = float32Ptr(p.ocpus)
	}
	if p.memoryInGBs > 0 {
		shapeConfig.MemoryInGBs = float32Ptr(p.memoryInGBs)
	}
	if shapeConfig.Ocpus == nil && shapeConfig.MemoryInGBs == nil {
		shapeConfig = nil
	}

	_, err = p.client.LaunchInstance(ctx, core.LaunchInstanceRequest{
		LaunchInstanceDetails: core.LaunchInstanceDetails{
			AvailabilityDomain: strPtr(p.availabilityDomain),
			CompartmentId:      strPtr(p.compartmentID),
			DisplayName:        strPtr(agent.Name),
			FreeformTags:       p.tags,
			Metadata:           metadata,
			Shape:              strPtr(p.shape),
			ShapeConfig:        shapeConfig,
			CreateVnicDetails: &core.CreateVnicDetails{
				AssignPublicIp: boolPtr(p.assignPublicIP),
				SubnetId:       strPtr(p.subnetID),
			},
			SourceDetails: core.InstanceSourceViaImageDetails{
				ImageId: strPtr(p.imageID),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("%s: LaunchInstance: %w", p.name, err)
	}

	return nil
}

func (p *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	instance, err := p.getAgent(ctx, agent)
	if err != nil {
		return fmt.Errorf("%s: getAgent: %w", p.name, err)
	}
	if instance == nil {
		return nil
	}

	_, err = p.client.TerminateInstance(ctx, core.TerminateInstanceRequest{
		InstanceId: instance.Id,
	})
	if err != nil {
		return fmt.Errorf("%s: TerminateInstance: %w", p.name, err)
	}

	return nil
}

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	instances, err := p.listPoolInstances(ctx, "")
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(instances))
	for _, instance := range instances {
		if instance.DisplayName != nil {
			names = append(names, *instance.DisplayName)
		}
	}

	return names, nil
}

func (p *Provider) getAgent(ctx context.Context, agent *woodpecker.Agent) (*core.Instance, error) {
	instances, err := p.listPoolInstances(ctx, agent.Name)
	if err != nil {
		return nil, err
	}
	if len(instances) == 0 {
		return nil, nil
	}
	if len(instances) > 1 {
		return nil, fmt.Errorf("found multiple instances with display name %s", agent.Name)
	}

	return &instances[0], nil
}

func (p *Provider) listPoolInstances(ctx context.Context, displayName string) ([]core.Instance, error) {
	var page *string
	instances := make([]core.Instance, 0)

	for {
		req := core.ListInstancesRequest{
			AvailabilityDomain: strPtr(p.availabilityDomain),
			CompartmentId:      strPtr(p.compartmentID),
			Limit:              intPtr(100),
			Page:               page,
		}
		if displayName != "" {
			req.DisplayName = strPtr(displayName)
		}

		resp, err := p.client.ListInstances(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("%s: ListInstances: %w", p.name, err)
		}

		for _, instance := range resp.Items {
			if p.isPoolInstance(instance) && isActive(instance.LifecycleState) {
				instances = append(instances, instance)
			}
		}

		if resp.OpcNextPage == nil || *resp.OpcNextPage == "" {
			break
		}
		page = resp.OpcNextPage
	}

	return instances, nil
}

func (p *Provider) isPoolInstance(instance core.Instance) bool {
	if instance.FreeformTags == nil {
		return false
	}
	return instance.FreeformTags[engine.LabelPool] == p.config.PoolID
}

func isActive(state core.InstanceLifecycleStateEnum) bool {
	return state != core.InstanceLifecycleStateTerminating &&
		state != core.InstanceLifecycleStateTerminated
}

func strPtr(value string) *string {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func float32Ptr(value float32) *float32 {
	return &value
}

func intPtr(value int) *int {
	return &value
}
