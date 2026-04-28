package equinixmetal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/equinix/equinix-sdk-go/services/metalv1"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/autoscaler/utils"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var ErrIllegalLabelPrefix = errors.New("illegal label prefix")

// DevicesAPI is the subset of the Equinix Metal API used by this provider.
// Declared as an interface so tests can inject a mock without a real API token.
type DevicesAPI interface {
	CreateDevice(ctx context.Context, projectID string, body metalv1.CreateDeviceRequest) (*metalv1.Device, error)
	DeleteDevice(ctx context.Context, deviceID string) error
	FindProjectDevicesByTag(ctx context.Context, projectID string, tag string) ([]metalv1.Device, error)
}

// sdkClient adapts metalv1.DevicesApiService to satisfy DevicesAPI.
type sdkClient struct {
	api *metalv1.DevicesApiService
}

func (c *sdkClient) CreateDevice(ctx context.Context, projectID string, body metalv1.CreateDeviceRequest) (*metalv1.Device, error) {
	device, _, err := c.api.CreateDevice(ctx, projectID).CreateDeviceRequest(body).Execute()
	return device, err
}

func (c *sdkClient) DeleteDevice(ctx context.Context, deviceID string) error {
	_, err := c.api.DeleteDevice(ctx, deviceID).Execute()
	return err
}

func (c *sdkClient) FindProjectDevicesByTag(ctx context.Context, projectID string, tag string) ([]metalv1.Device, error) {
	list, err := c.api.FindProjectDevices(ctx, projectID).Tag(tag).ExecuteWithPagination()
	if err != nil || list == nil {
		return nil, err
	}
	return list.GetDevices(), nil
}

// Provider implements types.Provider for Equinix Metal.
type Provider struct {
	name      string
	projectID string
	metro     string
	plan      string
	os        string
	sshKeys   []string
	labels    map[string]string
	config    *config.Config
	client    DevicesAPI
}

func New(_ context.Context, c *cli.Command, cfg *config.Config) (types.Provider, error) {
	token := c.String("equinixmetal-api-token")
	if token == "" {
		return nil, fmt.Errorf("equinixmetal: api token is required")
	}

	projectID := c.String("equinixmetal-project-id")
	if projectID == "" {
		return nil, fmt.Errorf("equinixmetal: project ID is required")
	}

	operatingSystem := c.String("equinixmetal-os")

	extraTags, err := utils.SliceToMap(c.StringSlice("equinixmetal-tags"), "=")
	if err != nil {
		return nil, fmt.Errorf("equinixmetal: parse tags: %w", err)
	}
	for key := range extraTags {
		if strings.HasPrefix(key, engine.LabelPrefix) {
			return nil, fmt.Errorf("equinixmetal: %w: %s", ErrIllegalLabelPrefix, engine.LabelPrefix)
		}
	}

	defaultLabels := map[string]string{
		engine.LabelPool:  cfg.PoolID,
		engine.LabelImage: operatingSystem,
	}

	metalCfg := metalv1.NewConfiguration()
	metalCfg.AddDefaultHeader("X-Auth-Token", token)
	metalCfg.UserAgent = "woodpecker-autoscaler"
	raw := metalv1.NewAPIClient(metalCfg)

	return &Provider{
		name:      "equinixmetal",
		projectID: projectID,
		metro:     c.String("equinixmetal-metro"),
		plan:      c.String("equinixmetal-plan"),
		os:        operatingSystem,
		sshKeys:   c.StringSlice("equinixmetal-ssh-keys"),
		labels:    utils.MergeMaps(defaultLabels, extraTags),
		config:    cfg,
		client:    &sdkClient{api: raw.DevicesApi},
	}, nil
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, nil)
	if err != nil {
		return fmt.Errorf("%s: RenderUserDataTemplate: %w", p.name, err)
	}

	tags := make([]string, 0, len(p.labels))
	for k, v := range p.labels {
		tags = append(tags, fmt.Sprintf("%s=%s", k, v))
	}

	input := metalv1.NewDeviceCreateInMetroInput(p.metro, p.os, p.plan)
	input.SetHostname(agent.Name)
	input.SetTags(tags)
	input.SetUserdata(userData)
	if len(p.sshKeys) > 0 {
		input.SetProjectSshKeys(p.sshKeys)
	}

	body := metalv1.DeviceCreateInMetroInputAsCreateDeviceRequest(input)
	if _, err = p.client.CreateDevice(ctx, p.projectID, body); err != nil {
		return fmt.Errorf("%s: CreateDevice %q: %w", p.name, agent.Name, err)
	}

	log.Debug().Str("agent", agent.Name).Str("provider", p.name).Msg("device created")
	return nil
}

func (p *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	poolTag := fmt.Sprintf("%s=%s", engine.LabelPool, p.config.PoolID)
	devices, err := p.client.FindProjectDevicesByTag(ctx, p.projectID, poolTag)
	if err != nil {
		return fmt.Errorf("%s: FindProjectDevicesByTag: %w", p.name, err)
	}

	for _, d := range devices {
		if d.GetHostname() == agent.Name {
			if err := p.client.DeleteDevice(ctx, d.GetId()); err != nil {
				return fmt.Errorf("%s: DeleteDevice %q: %w", p.name, agent.Name, err)
			}
			return nil
		}
	}

	log.Warn().Str("agent", agent.Name).Str("provider", p.name).Msg("device not found, skipping removal")
	return nil
}

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	poolTag := fmt.Sprintf("%s=%s", engine.LabelPool, p.config.PoolID)
	devices, err := p.client.FindProjectDevicesByTag(ctx, p.projectID, poolTag)
	if err != nil {
		return nil, fmt.Errorf("%s: FindProjectDevicesByTag: %w", p.name, err)
	}

	names := make([]string, 0, len(devices))
	for _, d := range devices {
		names = append(names, d.GetHostname())
	}
	return names, nil
}
