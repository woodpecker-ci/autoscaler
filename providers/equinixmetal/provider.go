package equinixmetal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/packethost/packngo"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	"golang.org/x/exp/maps"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var (
	ErrIllegalLabelPrefix = errors.New("illegal label prefix")
	ErrSSHKeyNotFound     = errors.New("SSH key not found")
)

type Provider struct {
	name             string
	projectID        string
	metro            string
	plan             string
	operatingSystem  string
	sshKeyIDs        []string
	tags             []string
	labels           map[string]string
	spotInstance     bool
	spotPriceMax     float64
	userDataTemplate *template.Template
	config           *config.Config
	client           *packngo.Client
}

func New(_ context.Context, c *cli.Command, config *config.Config) (engine.Provider, error) {
	p := &Provider{
		name:            "equinixmetal",
		projectID:       c.String("equinixmetal-project-id"),
		metro:           c.String("equinixmetal-metro"),
		plan:            c.String("equinixmetal-plan"),
		operatingSystem: c.String("equinixmetal-os"),
		spotInstance:    c.Bool("equinixmetal-spot-instance"),
		spotPriceMax:    c.Float64("equinixmetal-spot-price-max"),
		config:          config,
	}

	// Setup client
	p.client = packngo.NewClientWithAuth("woodpecker-autoscaler", c.String("equinixmetal-api-token"), nil)

	// Setup SSH keys
	if err := p.setupSSHKeys(c.StringSlice("equinixmetal-ssh-keys")); err != nil {
		return nil, fmt.Errorf("%s: setupSSHKeys: %w", p.name, err)
	}

	// Setup default labels/tags
	defaultLabels := make(map[string]string)
	defaultLabels[engine.LabelPool] = p.config.PoolID
	defaultLabels[engine.LabelImage] = p.operatingSystem

	// Parse user-provided labels
	labels, err := engine.SliceToMap(c.StringSlice("equinixmetal-tags"), "=")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}

	for _, key := range maps.Keys(labels) {
		if strings.HasPrefix(key, engine.LabelPrefix) {
			return nil, fmt.Errorf("%s: %w: %s", p.name, ErrIllegalLabelPrefix, engine.LabelPrefix)
		}
	}
	p.labels = engine.MergeMaps(defaultLabels, labels)

	// Convert labels to tags
	p.tags = make([]string, 0, len(p.labels))
	for key, value := range p.labels {
		tag := fmt.Sprintf("%s=%s", key, value)
		p.tags = append(p.tags, tag)
	}

	return p, nil
}

func (p *Provider) setupSSHKeys(keyNames []string) error {
	keys, _, err := p.client.SSHKeys.List()
	if err != nil {
		return err
	}

	// If specific keys requested, find them
	if len(keyNames) > 0 {
		for _, name := range keyNames {
			found := false
			for _, key := range keys {
				if key.Label == name || key.ID == name {
					p.sshKeyIDs = append(p.sshKeyIDs, key.ID)
					found = true
					break
				}
			}
			if !found {
				log.Warn().Msgf("SSH key not found: %s", name)
			}
		}
		if len(p.sshKeyIDs) > 0 {
			return nil
		}
	}

	// Try to find keys by naming convention
	index := make(map[string]string)
	for _, key := range keys {
		index[key.Label] = key.ID
	}

	for _, name := range []string{"woodpecker", "id_rsa_woodpecker"} {
		if id, ok := index[name]; ok {
			p.sshKeyIDs = append(p.sshKeyIDs, id)
			return nil
		}
	}

	// Use first available key if any
	if len(keys) > 0 {
		p.sshKeyIDs = append(p.sshKeyIDs, keys[0].ID)
		return nil
	}

	return ErrSSHKeyNotFound
}

func (p *Provider) DeployAgent(_ context.Context, agent *woodpecker.Agent) error {
	userData, err := engine.RenderUserDataTemplate(p.config, agent, p.userDataTemplate)
	if err != nil {
		return fmt.Errorf("%s: engine.RenderUserDataTemplate: %w", p.name, err)
	}

	createRequest := &packngo.DeviceCreateRequest{
		Hostname:              agent.Name,
		Plan:                  p.plan,
		Metro:                 p.metro,
		OS:                    p.operatingSystem,
		ProjectID:             p.projectID,
		UserData:              userData,
		Tags:                  p.tags,
		ProjectSSHKeys:        p.sshKeyIDs,
		SpotInstance:          p.spotInstance,
		SpotPriceMax:          p.spotPriceMax,
		BillingCycle:          "hourly",
		AlwaysPXE:             false,
		HardwareReservationID: "",
	}

	_, _, err = p.client.Devices.Create(createRequest)
	if err != nil {
		return fmt.Errorf("%s: Devices.Create: %w", p.name, err)
	}

	return nil
}

func (p *Provider) getAgent(agent *woodpecker.Agent) (*packngo.Device, error) {
	devices, _, err := p.client.Devices.List(p.projectID, &packngo.ListOptions{
		Search: agent.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: Devices.List: %w", p.name, err)
	}

	for _, device := range devices {
		if device.Hostname == agent.Name {
			return &device, nil
		}
	}

	return nil, nil
}

func (p *Provider) RemoveAgent(_ context.Context, agent *woodpecker.Agent) error {
	device, err := p.getAgent(agent)
	if err != nil {
		return fmt.Errorf("%s: getAgent: %w", p.name, err)
	}

	if device == nil {
		return nil
	}

	_, err = p.client.Devices.Delete(device.ID, false)
	if err != nil {
		return fmt.Errorf("%s: Devices.Delete: %w", p.name, err)
	}

	return nil
}

func (p *Provider) ListDeployedAgentNames(_ context.Context) ([]string, error) {
	var names []string

	// Create tag for filtering by pool
	poolTag := fmt.Sprintf("%s=%s", engine.LabelPool, p.config.PoolID)

	devices, _, err := p.client.Devices.List(p.projectID, &packngo.ListOptions{
		Search: poolTag,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: Devices.List: %w", p.name, err)
	}

	for _, device := range devices {
		// Verify device has our pool tag
		hasTag := false
		for _, tag := range device.Tags {
			if tag == poolTag {
				hasTag = true
				break
			}
		}
		if hasTag {
			names = append(names, device.Hostname)
		}
	}

	return names, nil
}
