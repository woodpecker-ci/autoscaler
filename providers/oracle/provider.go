package oracle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/urfave/cli/v3"
	"golang.org/x/exp/maps"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var ErrIllegalLabelPrefix = errors.New("illegal label prefix")

type Provider struct {
	name               string
	compartmentOCID    string
	availabilityDomain string
	subnetOCID         string
	shape              string
	ocpus              int
	memoryGB           int
	imageOCID          string
	sshPublicKey       string
	freeformTags       map[string]string
	userDataTemplate   *template.Template
	config             *config.Config
	computeClient      core.ComputeClient
}

func New(_ context.Context, c *cli.Command, config *config.Config) (engine.Provider, error) {
	p := &Provider{
		name:               "oracle",
		compartmentOCID:    c.String("oracle-compartment-ocid"),
		availabilityDomain: c.String("oracle-availability-domain"),
		subnetOCID:         c.String("oracle-subnet-ocid"),
		shape:              c.String("oracle-shape"),
		ocpus:              c.Int("oracle-ocpus"),
		memoryGB:           c.Int("oracle-memory-gb"),
		imageOCID:          c.String("oracle-image-ocid"),
		sshPublicKey:       c.String("oracle-ssh-public-key"),
		config:             config,
	}

	// Setup OCI configuration provider
	configProvider := common.NewRawConfigurationProvider(
		c.String("oracle-tenancy-ocid"),
		c.String("oracle-user-ocid"),
		c.String("oracle-region"),
		c.String("oracle-fingerprint"),
		c.String("oracle-private-key"),
		nil,
	)

	// Create compute client
	computeClient, err := core.NewComputeClientWithConfigurationProvider(configProvider)
	if err != nil {
		return nil, fmt.Errorf("%s: NewComputeClient: %w", p.name, err)
	}
	p.computeClient = computeClient

	// Setup default freeform tags
	defaultTags := make(map[string]string)
	defaultTags[engine.LabelPool] = p.config.PoolID
	defaultTags[engine.LabelImage] = p.imageOCID

	// Parse user-provided tags (OCI uses freeform tags for custom metadata)
	labels, err := engine.SliceToMap(c.StringSlice("oracle-tags"), "=")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}

	for _, key := range maps.Keys(labels) {
		if strings.HasPrefix(key, engine.LabelPrefix) {
			return nil, fmt.Errorf("%s: %w: %s", p.name, ErrIllegalLabelPrefix, engine.LabelPrefix)
		}
	}
	p.freeformTags = engine.MergeMaps(defaultTags, labels)

	return p, nil
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := engine.RenderUserDataTemplate(p.config, agent, p.userDataTemplate)
	if err != nil {
		return fmt.Errorf("%s: engine.RenderUserDataTemplate: %w", p.name, err)
	}

	// Build instance details
	instanceDetails := core.LaunchInstanceDetails{
		CompartmentId:      common.String(p.compartmentOCID),
		AvailabilityDomain: common.String(p.availabilityDomain),
		DisplayName:        common.String(agent.Name),
		Shape:              common.String(p.shape),
		FreeformTags:       p.freeformTags,
		Metadata: map[string]string{
			"user_data": userData,
		},
		SourceDetails: core.InstanceSourceViaImageDetails{
			ImageId: common.String(p.imageOCID),
		},
		CreateVnicDetails: &core.CreateVnicDetails{
			SubnetId:       common.String(p.subnetOCID),
			AssignPublicIp: common.Bool(true),
		},
	}

	// Add SSH key if provided
	if p.sshPublicKey != "" {
		instanceDetails.Metadata["ssh_authorized_keys"] = p.sshPublicKey
	}

	// Add shape config for flex shapes
	if strings.Contains(p.shape, "Flex") {
		instanceDetails.ShapeConfig = &core.LaunchInstanceShapeConfigDetails{
			Ocpus:       common.Float32(float32(p.ocpus)),
			MemoryInGBs: common.Float32(float32(p.memoryGB)),
		}
	}

	request := core.LaunchInstanceRequest{
		LaunchInstanceDetails: instanceDetails,
	}

	_, err = p.computeClient.LaunchInstance(ctx, request)
	if err != nil {
		return fmt.Errorf("%s: LaunchInstance: %w", p.name, err)
	}

	return nil
}

func (p *Provider) getAgent(ctx context.Context, agent *woodpecker.Agent) (*core.Instance, error) {
	request := core.ListInstancesRequest{
		CompartmentId: common.String(p.compartmentOCID),
		DisplayName:   common.String(agent.Name),
	}

	response, err := p.computeClient.ListInstances(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("%s: ListInstances: %w", p.name, err)
	}

	for _, instance := range response.Items {
		if instance.LifecycleState != core.InstanceLifecycleStateTerminated &&
			*instance.DisplayName == agent.Name {
			return &instance, nil
		}
	}

	return nil, nil
}

func (p *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	instance, err := p.getAgent(ctx, agent)
	if err != nil {
		return fmt.Errorf("%s: getAgent: %w", p.name, err)
	}

	if instance == nil {
		return nil
	}

	request := core.TerminateInstanceRequest{
		InstanceId:         instance.Id,
		PreserveBootVolume: common.Bool(false),
	}

	_, err = p.computeClient.TerminateInstance(ctx, request)
	if err != nil {
		return fmt.Errorf("%s: TerminateInstance: %w", p.name, err)
	}

	return nil
}

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	var names []string

	request := core.ListInstancesRequest{
		CompartmentId: common.String(p.compartmentOCID),
	}

	response, err := p.computeClient.ListInstances(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("%s: ListInstances: %w", p.name, err)
	}

	poolTag := p.config.PoolID

	for _, instance := range response.Items {
		// Skip terminated instances
		if instance.LifecycleState == core.InstanceLifecycleStateTerminated {
			continue
		}

		// Check if instance belongs to our pool
		if pool, ok := instance.FreeformTags[engine.LabelPool]; ok && pool == poolTag {
			names = append(names, *instance.DisplayName)
		}
	}

	return names, nil
}
