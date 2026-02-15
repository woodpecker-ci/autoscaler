package equinixmetal

import (
	"context"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/packethost/packngo"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type Provider struct {
	name             string
	config           *config.Config
	projectID        string
	plan             string
	metro            string
	os               string
	tags             []string
	client           *packngo.Client
	userDataTemplate *template.Template
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (engine.Provider, error) {
	apiToken := c.String("equinixmetal-api-token")
	if apiToken == "" {
		return nil, fmt.Errorf("equinixmetal-api-token must be set")
	}

	projectID := c.String("equinixmetal-project-id")
	if projectID == "" {
		return nil, fmt.Errorf("equinixmetal-project-id must be set")
	}

	if c.String("equinixmetal-metro") == "" {
		return nil, fmt.Errorf("equinixmetal-metro must be set")
	}

	p := &Provider{
		name:      "equinixmetal",
		config:    config,
		projectID: projectID,
		plan:      c.String("equinixmetal-plan"),
		metro:     c.String("equinixmetal-metro"),
		os:        c.String("equinixmetal-os"),
		tags:      c.StringSlice("equinixmetal-tags"),
	}

	// Setup Equinix Metal client
	p.client = packngo.NewClientWithAuth("woodpecker-autoscaler", apiToken, nil)

	// User data template
	if u := c.String("provider-user-data"); u != "" {
		userDataTmpl, err := template.New("user-data").Parse(u)
		if err != nil {
			return nil, fmt.Errorf("%s: template.New.Parse %w", p.name, err)
		}
		p.userDataTemplate = userDataTmpl
	}

	return p, nil
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := engine.RenderUserDataTemplate(p.config, agent, p.userDataTemplate)
	if err != nil {
		return fmt.Errorf("%s: engine.RenderUserDataTemplate: %w", p.name, err)
	}

	// Generate tags
	tags := []string{
		agent.Name,
		engine.LabelPool,
		p.config.PoolID,
	}

	// Append user specified tags
	for _, tag := range p.tags {
		tags = append(tags, strings.TrimSpace(tag))
	}

	// Create device request
	createRequest := &packngo.DeviceCreateRequest{
		Hostname:  agent.Name,
		Plan:      p.plan,
		Metro:     p.metro,
		OS:        p.os,
		ProjectID: p.projectID,
		Tags:      tags,
		UserData:  userData,
	}

	device, _, err := p.client.Devices.Create(createRequest)
	if err != nil {
		return fmt.Errorf("%s: Devices.Create: %w", p.name, err)
	}

	log.Debug().Msgf("created device %s (%s)", device.ID, device.Hostname)

	// Wait until device is active
	log.Debug().Msgf("waiting for device %s to become active", device.ID)
	for range 60 { // Max 60 seconds for provisioning
		d, _, err := p.client.Devices.Get(device.ID, nil)
		if err != nil {
			return fmt.Errorf("%s: Devices.Get: %w", p.name, err)
		}

		if d.State == "active" {
			return nil
		}

		if d.State == "failed" {
			return fmt.Errorf("device %s failed to provision", device.ID)
		}

		log.Debug().Msgf("device state: %s", d.State)
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("device %s did not become active in time", device.ID)
}

func (p *Provider) getDeviceByHostname(hostname string) (*packngo.Device, error) {
	// List all devices in project and find by hostname
	devices, _, err := p.client.Devices.List(p.projectID, nil)
	if err != nil {
		return nil, err
	}

	for _, d := range devices {
		if d.Hostname == hostname {
			return &d, nil
		}
	}

	return nil, fmt.Errorf("device with hostname %s not found", hostname)
}

func (p *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	device, err := p.getDeviceByHostname(agent.Name)
	if err != nil {
		return err
	}

	_, err = p.client.Devices.Delete(device.ID, true) // Force delete
	if err != nil {
		return fmt.Errorf("%s: Devices.Delete: %w", p.name, err)
	}

	log.Debug().Msgf("deleted device %s (%s)", device.ID, agent.Name)
	return nil
}

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	log.Debug().Msgf("list deployed agent names")

	var names []string

	devices, _, err := p.client.Devices.List(p.projectID, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: Devices.List: %w", p.name, err)
	}

	for _, device := range devices {
		// Check if device has the pool tag
		for _, tag := range device.Tags {
			if tag == p.config.PoolID {
				// Check if device is active or provisioning
				if device.State == "active" || device.State == "provisioning" {
					log.Debug().Msgf("found agent %s (state: %s)", device.Hostname, device.State)
					names = append(names, device.Hostname)
				}
				break
			}
		}
	}

	return names, nil
}
