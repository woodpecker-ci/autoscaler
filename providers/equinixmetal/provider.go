package equinixmetal

import (
	"context"
	"errors"
	"fmt"
	"text/template"

	equinix "github.com/equinix/equinix-sdk-go/services/metalv1"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var (
	ErrSSHKeyNotFound = errors.New("SSH key not found")
)

type Provider struct {
	name             string
	projectID        string
	metro            string
	plan             string
	operatingSystem  string
	tags             []string
	sshKeyIDs        []string
	userDataTemplate *template.Template
	config           *config.Config
	client           *equinix.APIClient
}

func New(_ context.Context, c *cli.Command, cfg *config.Config) (engine.Provider, error) {
	configuration := equinix.NewConfiguration()
	configuration.AddDefaultHeader("X-Auth-Token", c.String("equinixmetal-api-token"))

	client := equinix.NewAPIClient(configuration)

	p := &Provider{
		name:            "equinixmetal",
		projectID:       c.String("equinixmetal-project-id"),
		metro:           c.String("equinixmetal-metro"),
		plan:            c.String("equinixmetal-plan"),
		operatingSystem: c.String("equinixmetal-os"),
		config:          cfg,
		client:          client,
	}

	if u := c.String("equinixmetal-user-data"); u != "" {
		log.Warn().Msg("equinixmetal-user-data is deprecated, please use provider-user-data instead")
		userDataTmpl, err := template.New("user-data").Parse(u)
		if err != nil {
			return nil, fmt.Errorf("%s: template.New.Parse %w", p.name, err)
		}
		p.userDataTemplate = userDataTmpl
	}

	p.tags = c.StringSlice("equinixmetal-tags")

	// Add pool tag for agent tracking
	poolTag := fmt.Sprintf("%s=%s", engine.LabelPool, cfg.PoolID)
	p.tags = append(p.tags, poolTag)

	err := p.setupSSHKeys(context.Background())
	if err != nil {
		return nil, fmt.Errorf("%s: setupSSHKeys: %w", p.name, err)
	}

	return p, nil
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := engine.RenderUserDataTemplate(p.config, agent, p.userDataTemplate)
	if err != nil {
		return fmt.Errorf("%s: engine.RenderUserDataTemplate: %w", p.name, err)
	}

	hostname := agent.Name
	metro := p.metro
	plan := p.plan
	os := p.operatingSystem

	req := equinix.DeviceCreateInMetroInput{
		Hostname:        &hostname,
		Metro:           metro,
		Plan:            plan,
		OperatingSystem: os,
		Tags:            p.tags,
		Userdata:        &userData,
		ProjectSshKeys:  p.sshKeyIDs,
	}

	createReq := equinix.DeviceCreateInMetroInputAsCreateDeviceRequest(&req)

	log.Info().Msgf("creating Equinix Metal device: metro=%s plan=%s os=%s", metro, plan, os)

	_, _, err = p.client.DevicesApi.CreateDevice(ctx, p.projectID).CreateDeviceRequest(createReq).Execute()
	if err != nil {
		return fmt.Errorf("%s: DevicesApi.CreateDevice: %w", p.name, err)
	}

	return nil
}

func (p *Provider) findDeviceByHostname(ctx context.Context, hostname string) (*equinix.Device, error) {
	page := int32(1) //nolint:mnd

	for {
		devices, _, err := p.client.DevicesApi.
			FindProjectDevices(ctx, p.projectID).
			Hostname(hostname).
			Page(page).
			PerPage(100). //nolint:mnd
			Execute()
		if err != nil {
			return nil, fmt.Errorf("%s: FindProjectDevices: %w", p.name, err)
		}

		for i := range devices.GetDevices() {
			d := devices.GetDevices()[i]
			if d.GetHostname() == hostname {
				return &d, nil
			}
		}

		meta := devices.GetMeta()
		if meta.GetCurrentPage() >= meta.GetLastPage() {
			break
		}
		page++
	}

	return nil, nil
}

func (p *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	device, err := p.findDeviceByHostname(ctx, agent.Name)
	if err != nil {
		return fmt.Errorf("%s: findDeviceByHostname: %w", p.name, err)
	}

	if device == nil {
		return nil
	}

	_, err = p.client.DevicesApi.DeleteDevice(ctx, device.GetId()).Execute()
	if err != nil {
		return fmt.Errorf("%s: DevicesApi.DeleteDevice: %w", p.name, err)
	}

	return nil
}

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	var names []string

	poolTag := fmt.Sprintf("%s=%s", engine.LabelPool, p.config.PoolID)
	page := int32(1) //nolint:mnd

	for {
		devices, _, err := p.client.DevicesApi.
			FindProjectDevices(ctx, p.projectID).
			Tag(poolTag).
			Page(page).
			PerPage(100). //nolint:mnd
			Execute()
		if err != nil {
			return nil, fmt.Errorf("%s: FindProjectDevices: %w", p.name, err)
		}

		for _, d := range devices.GetDevices() {
			names = append(names, d.GetHostname())
		}

		meta := devices.GetMeta()
		if meta.GetCurrentPage() >= meta.GetLastPage() {
			break
		}
		page++
	}

	return names, nil
}

func (p *Provider) setupSSHKeys(ctx context.Context) error {
	keys, _, err := p.client.SSHKeysApi.FindProjectSSHKeys(ctx, p.projectID).Execute()
	if err != nil {
		return fmt.Errorf("FindProjectSSHKeys: %w", err)
	}

	sshKeys := keys.GetSshKeys()

	// Try to find a woodpecker-named key first
	for _, key := range sshKeys {
		name := key.GetLabel()
		if name == "woodpecker" || name == "id_rsa_woodpecker" {
			p.sshKeyIDs = append(p.sshKeyIDs, key.GetId())
			return nil
		}
	}

	// Fall back to first available key
	if len(sshKeys) > 0 {
		p.sshKeyIDs = append(p.sshKeyIDs, sshKeys[0].GetId())
		return nil
	}

	return ErrSSHKeyNotFound
}
