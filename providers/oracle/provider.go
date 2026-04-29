package oracle

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var ErrIncompleteExplicitAuth = errors.New("oracle-tenancy-ocid, oracle-user-ocid, oracle-fingerprint, and oracle-private-key-file must all be set together")

// ComputeClient is the subset of the OCI Compute API used by this provider.
// Declared as an interface so tests can inject a double without a real OCI account.
type ComputeClient interface {
	LaunchInstance(ctx context.Context, request core.LaunchInstanceRequest) (core.LaunchInstanceResponse, error)
	TerminateInstance(ctx context.Context, request core.TerminateInstanceRequest) (core.TerminateInstanceResponse, error)
	ListInstances(ctx context.Context, request core.ListInstancesRequest) (core.ListInstancesResponse, error)
}

type sdkComputeClient struct {
	inner core.ComputeClient
}

func (c *sdkComputeClient) LaunchInstance(ctx context.Context, req core.LaunchInstanceRequest) (core.LaunchInstanceResponse, error) {
	return c.inner.LaunchInstance(ctx, req)
}

func (c *sdkComputeClient) TerminateInstance(ctx context.Context, req core.TerminateInstanceRequest) (core.TerminateInstanceResponse, error) {
	return c.inner.TerminateInstance(ctx, req)
}

func (c *sdkComputeClient) ListInstances(ctx context.Context, req core.ListInstancesRequest) (core.ListInstancesResponse, error) {
	return c.inner.ListInstances(ctx, req)
}

type Provider struct {
	name               string
	compartmentID      string
	availabilityDomain string
	imageID            string
	shape              string
	subnetID           string
	sshAuthorizedKey   string
	shapeOCPUs         float32
	shapeMemoryGBs     float32
	config             *config.Config
	client             ComputeClient
}

func New(_ context.Context, c *cli.Command, cfg *config.Config) (types.Provider, error) {
	compartmentID := c.String("oracle-compartment-id")
	if compartmentID == "" {
		return nil, fmt.Errorf("oracle: oracle-compartment-id is required")
	}

	availabilityDomain := c.String("oracle-availability-domain")
	if availabilityDomain == "" {
		return nil, fmt.Errorf("oracle: oracle-availability-domain is required")
	}

	imageID := c.String("oracle-image-id")
	if imageID == "" {
		return nil, fmt.Errorf("oracle: oracle-image-id is required")
	}

	subnetID := c.String("oracle-subnet-id")
	if subnetID == "" {
		return nil, fmt.Errorf("oracle: oracle-subnet-id is required")
	}

	configProvider, err := buildConfigProvider(c)
	if err != nil {
		return nil, fmt.Errorf("oracle: %w", err)
	}

	inner, err := core.NewComputeClientWithConfigurationProvider(configProvider)
	if err != nil {
		return nil, fmt.Errorf("oracle: create compute client: %w", err)
	}

	return &Provider{
		name:               "oracle",
		compartmentID:      compartmentID,
		availabilityDomain: availabilityDomain,
		imageID:            imageID,
		shape:              c.String("oracle-shape"),
		subnetID:           subnetID,
		sshAuthorizedKey:   c.String("oracle-ssh-authorized-key"),
		shapeOCPUs:         float32(c.Float("oracle-shape-ocpus")),
		shapeMemoryGBs:     float32(c.Float("oracle-shape-memory-gbs")),
		config:             cfg,
		client:             &sdkComputeClient{inner: inner},
	}, nil
}

// buildConfigProvider returns an OCI ConfigurationProvider.
// When all four explicit credential flags are provided it builds a raw provider
// from them; otherwise it falls back to the SDK default chain (~/.oci/config,
// instance principal, etc.).
func buildConfigProvider(c *cli.Command) (common.ConfigurationProvider, error) {
	tenancy := c.String("oracle-tenancy-ocid")
	user := c.String("oracle-user-ocid")
	fingerprint := c.String("oracle-fingerprint")
	keyFile := c.String("oracle-private-key-file")
	region := c.String("oracle-region")

	explicitCount := 0
	for _, v := range []string{tenancy, user, fingerprint, keyFile} {
		if v != "" {
			explicitCount++
		}
	}

	switch explicitCount {
	case 0:
		return common.DefaultConfigProvider(), nil
	case 4:
		pem, err := os.ReadFile(keyFile)
		if err != nil {
			return nil, fmt.Errorf("read private key %q: %w", keyFile, err)
		}
		return common.NewRawConfigurationProvider(tenancy, user, region, fingerprint, string(pem), nil), nil
	default:
		return nil, ErrIncompleteExplicitAuth
	}
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, nil)
	if err != nil {
		return fmt.Errorf("%s: RenderUserDataTemplate: %w", p.name, err)
	}

	details := core.LaunchInstanceDetails{
		CompartmentId:      common.String(p.compartmentID),
		AvailabilityDomain: common.String(p.availabilityDomain),
		DisplayName:        common.String(agent.Name),
		ImageId:            common.String(p.imageID),
		Shape:              common.String(p.shape),
		CreateVnicDetails: &core.CreateVnicDetails{
			SubnetId: common.String(p.subnetID),
		},
		Metadata: map[string]string{
			"user_data":           base64.StdEncoding.EncodeToString([]byte(userData)),
			"ssh_authorized_keys": p.sshAuthorizedKey,
		},
		FreeformTags: map[string]string{
			engine.LabelPool: p.config.PoolID,
		},
	}

	if p.shapeOCPUs > 0 {
		details.ShapeConfig = &core.LaunchInstanceShapeConfigDetails{
			Ocpus:       common.Float32(p.shapeOCPUs),
			MemoryInGBs: common.Float32(p.shapeMemoryGBs),
		}
	}

	resp, err := p.client.LaunchInstance(ctx, core.LaunchInstanceRequest{
		LaunchInstanceDetails: details,
	})
	if err != nil {
		return fmt.Errorf("%s: LaunchInstance %q: %w", p.name, agent.Name, err)
	}

	log.Debug().
		Str("agent", agent.Name).
		Str("provider", p.name).
		Str("instance_id", *resp.Instance.Id).
		Msg("instance launched")

	return nil
}

func (p *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	instance, err := p.findInstance(ctx, agent.Name)
	if err != nil {
		return fmt.Errorf("%s: findInstance: %w", p.name, err)
	}

	if instance == nil {
		log.Warn().
			Str("agent", agent.Name).
			Str("provider", p.name).
			Msg("instance not found, skipping removal")
		return nil
	}

	if _, err = p.client.TerminateInstance(ctx, core.TerminateInstanceRequest{
		InstanceId: instance.Id,
	}); err != nil {
		return fmt.Errorf("%s: TerminateInstance %q: %w", p.name, agent.Name, err)
	}

	log.Debug().
		Str("agent", agent.Name).
		Str("provider", p.name).
		Msg("instance terminated")

	return nil
}

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	instances, err := p.listPoolInstances(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: listPoolInstances: %w", p.name, err)
	}

	names := make([]string, 0, len(instances))
	for _, inst := range instances {
		if inst.DisplayName != nil {
			names = append(names, *inst.DisplayName)
		}
	}

	return names, nil
}

func (p *Provider) findInstance(ctx context.Context, name string) (*core.Instance, error) {
	instances, err := p.listPoolInstances(ctx)
	if err != nil {
		return nil, err
	}

	for i := range instances {
		if instances[i].DisplayName != nil && *instances[i].DisplayName == name {
			return &instances[i], nil
		}
	}

	return nil, nil
}

func (p *Provider) listPoolInstances(ctx context.Context) ([]core.Instance, error) {
	var (
		page      *string
		instances []core.Instance
	)

	for {
		resp, err := p.client.ListInstances(ctx, core.ListInstancesRequest{
			CompartmentId: common.String(p.compartmentID),
			Page:          page,
		})
		if err != nil {
			return nil, fmt.Errorf("ListInstances: %w", err)
		}

		for _, inst := range resp.Items {
			if isTerminal(inst.LifecycleState) {
				continue
			}
			if inst.FreeformTags[engine.LabelPool] != p.config.PoolID {
				continue
			}
			instances = append(instances, inst)
		}

		if resp.OpcNextPage == nil {
			break
		}
		page = resp.OpcNextPage
	}

	return instances, nil
}

func isTerminal(state core.InstanceLifecycleStateEnum) bool {
	return state == core.InstanceLifecycleStateTerminating ||
		state == core.InstanceLifecycleStateTerminated
}
