package oraclecloud

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type Provider struct {
	name               string
	config             *config.Config
	client             core.ComputeClient
	compartmentID      string
	availabilityDomain string
	imageID            string
	shape              string
	subnetID           string
	tags               map[string]string
}

func New(_ context.Context, c *cli.Command, cfg *config.Config) (engine.Provider, error) {
	tenancy := c.String("oci-tenancy")
	user := c.String("oci-user")
	region := c.String("oci-region")
	fingerprint := c.String("oci-fingerprint")
	privateKey := c.String("oci-private-key")

	if tenancy == "" {
		return nil, fmt.Errorf("oraclecloud: oci-tenancy is required")
	}
	if user == "" {
		return nil, fmt.Errorf("oraclecloud: oci-user is required")
	}
	if region == "" {
		return nil, fmt.Errorf("oraclecloud: oci-region is required")
	}
	if fingerprint == "" {
		return nil, fmt.Errorf("oraclecloud: oci-fingerprint is required")
	}
	if privateKey == "" {
		return nil, fmt.Errorf("oraclecloud: oci-private-key is required")
	}

	configProvider := common.NewRawConfigurationProvider(tenancy, user, region, fingerprint, privateKey, nil)

	client, err := core.NewComputeClientWithConfigurationProvider(configProvider)
	if err != nil {
		return nil, fmt.Errorf("oraclecloud: NewComputeClientWithConfigurationProvider: %w", err)
	}

	rawTags, err := engine.SliceToMap(c.StringSlice("oci-tags"), "=")
	if err != nil {
		return nil, fmt.Errorf("oraclecloud: %w", err)
	}

	// Always track the pool so we can list agents by pool ID.
	rawTags[engine.LabelPool] = cfg.PoolID

	p := &Provider{
		name:               "oraclecloud",
		config:             cfg,
		client:             client,
		compartmentID:      c.String("oci-compartment-id"),
		availabilityDomain: c.String("oci-availability-domain"),
		imageID:            c.String("oci-image-id"),
		shape:              c.String("oci-shape"),
		subnetID:           c.String("oci-subnet-id"),
		tags:               rawTags,
	}

	return p, nil
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := engine.RenderUserDataTemplate(p.config, agent, nil)
	if err != nil {
		return fmt.Errorf("%s: engine.RenderUserDataTemplate: %w", p.name, err)
	}

	encodedUserData := base64.StdEncoding.EncodeToString([]byte(userData))

	metadata := map[string]string{
		"user_data": encodedUserData,
	}

	req := core.LaunchInstanceRequest{
		LaunchInstanceDetails: core.LaunchInstanceDetails{
			CompartmentId:      common.String(p.compartmentID),
			AvailabilityDomain: common.String(p.availabilityDomain),
			DisplayName:        common.String(agent.Name),
			Shape:              common.String(p.shape),
			SourceDetails: core.InstanceSourceViaImageDetails{
				ImageId: common.String(p.imageID),
			},
			CreateVnicDetails: &core.CreateVnicDetails{
				SubnetId:       common.String(p.subnetID),
				AssignPublicIp: common.Bool(true),
			},
			Metadata:     metadata,
			FreeformTags: p.tags,
		},
	}

	log.Info().Msgf("%s: launching instance %s (shape=%s)", p.name, agent.Name, p.shape)

	resp, err := p.client.LaunchInstance(ctx, req)
	if err != nil {
		return fmt.Errorf("%s: LaunchInstance: %w", p.name, err)
	}

	log.Debug().Msgf("%s: launched instance %s with OCID %s", p.name, agent.Name, *resp.Instance.Id)

	return nil
}

func (p *Provider) getAgent(ctx context.Context, agent *woodpecker.Agent) (*core.Instance, error) {
	var page *string

	for {
		resp, err := p.client.ListInstances(ctx, core.ListInstancesRequest{
			CompartmentId: common.String(p.compartmentID),
			DisplayName:   common.String(agent.Name),
			Page:          page,
		})
		if err != nil {
			return nil, fmt.Errorf("%s: ListInstances: %w", p.name, err)
		}

		for i := range resp.Items {
			inst := &resp.Items[i]
			if inst.LifecycleState == core.InstanceLifecycleStateTerminated ||
				inst.LifecycleState == core.InstanceLifecycleStateTerminating {
				continue
			}
			if inst.DisplayName != nil && *inst.DisplayName == agent.Name {
				return inst, nil
			}
		}

		if resp.OpcNextPage == nil {
			break
		}
		page = resp.OpcNextPage
	}

	return nil, nil
}

func (p *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	inst, err := p.getAgent(ctx, agent)
	if err != nil {
		return fmt.Errorf("%s: getAgent: %w", p.name, err)
	}

	if inst == nil {
		return nil
	}

	_, err = p.client.TerminateInstance(ctx, core.TerminateInstanceRequest{
		InstanceId: inst.Id,
	})
	if err != nil {
		return fmt.Errorf("%s: TerminateInstance: %w", p.name, err)
	}

	log.Info().Msgf("%s: terminated instance %s (%s)", p.name, agent.Name, *inst.Id)

	return nil
}

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	var names []string
	var page *string

	poolTag := p.config.PoolID

	for {
		resp, err := p.client.ListInstances(ctx, core.ListInstancesRequest{
			CompartmentId: common.String(p.compartmentID),
			Page:          page,
		})
		if err != nil {
			return nil, fmt.Errorf("%s: ListInstances: %w", p.name, err)
		}

		for _, inst := range resp.Items {
			if inst.LifecycleState == core.InstanceLifecycleStateTerminated ||
				inst.LifecycleState == core.InstanceLifecycleStateTerminating {
				continue
			}

			// Only return instances that belong to this pool.
			if v, ok := inst.FreeformTags[engine.LabelPool]; !ok || v != poolTag {
				continue
			}

			if inst.DisplayName != nil {
				names = append(names, *inst.DisplayName)
			}
		}

		if resp.OpcNextPage == nil {
			break
		}
		page = resp.OpcNextPage
	}

	return names, nil
}
