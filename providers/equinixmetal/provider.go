package equinixmetal

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"text/template"

	"github.com/packethost/packngo"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var (
	ErrProjectIDRequired    = errors.New("project ID is required")
	ErrPlanRequired         = errors.New("plan is required")
	ErrOperatingSysRequired = errors.New("operating system is required")
	ErrLocationRequired     = errors.New("either metro or facility must be set")
	ErrLocationConflict     = errors.New("metro and facility are mutually exclusive")
	ErrEmptyTag             = errors.New("tags must not be empty")
	ErrReservedTagPrefix    = errors.New("illegal tag prefix")
)

type devicesService interface {
	Create(*packngo.DeviceCreateRequest) (*packngo.Device, *packngo.Response, error)
	List(string, *packngo.ListOptions) ([]packngo.Device, *packngo.Response, error)
	Delete(string, bool) (*packngo.Response, error)
}

const deviceListPerPage = 1000

type Provider struct {
	name             string
	projectID        string
	metro            string
	facility         []string
	plan             string
	operatingSys     string
	billingCycle     string
	tags             []string
	projectSSHKeys   []string
	spotInstance     bool
	spotPriceMax     float64
	config           *config.Config
	userDataTemplate *template.Template
	devices          devicesService
}

func New(_ context.Context, c *cli.Command, config *config.Config) (types.Provider, error) {
	p := &Provider{
		name:           "equinixmetal",
		projectID:      c.String("equinixmetal-project-id"),
		metro:          c.String("equinixmetal-metro"),
		facility:       c.StringSlice("equinixmetal-facility"),
		plan:           c.String("equinixmetal-plan"),
		operatingSys:   c.String("equinixmetal-operating-system"),
		billingCycle:   c.String("equinixmetal-billing-cycle"),
		tags:           c.StringSlice("equinixmetal-tags"),
		projectSSHKeys: c.StringSlice("equinixmetal-project-ssh-keys"),
		spotInstance:   c.Bool("equinixmetal-spot-instance"),
		spotPriceMax:   c.Float64("equinixmetal-spot-price-max"),
		config:         config,
	}

	if err := p.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}

	// TODO: Deprecated remove in v2.0
	if u := c.String("equinixmetal-user-data"); u != "" {
		log.Warn().Msg("equinixmetal-user-data is deprecated, please use provider-user-data instead")
		userDataTmpl, err := template.New("user-data").Parse(u)
		if err != nil {
			return nil, fmt.Errorf("%s: template.New.Parse %w", p.name, err)
		}
		p.userDataTemplate = userDataTmpl
	}

	client, err := packngo.NewClient(packngo.WithAuth("woodpecker-autoscaler", c.String("equinixmetal-api-token")))
	if err != nil {
		return nil, fmt.Errorf("%s: packngo.NewClient: %w", p.name, err)
	}
	p.devices = client.Devices

	return p, nil
}

func (p *Provider) validate() error {
	switch {
	case p.projectID == "":
		return ErrProjectIDRequired
	case p.plan == "":
		return ErrPlanRequired
	case p.operatingSys == "":
		return ErrOperatingSysRequired
	case p.metro == "" && len(p.facility) == 0:
		return ErrLocationRequired
	case p.metro != "" && len(p.facility) > 0:
		return ErrLocationConflict
	}

	for _, tag := range p.tags {
		if strings.TrimSpace(tag) == "" {
			return ErrEmptyTag
		}
		key, _, _ := strings.Cut(tag, "=")
		if strings.HasPrefix(strings.TrimSpace(key), engine.LabelPrefix) {
			return fmt.Errorf("%w: %s", ErrReservedTagPrefix, engine.LabelPrefix)
		}
	}

	return nil
}

func (p *Provider) DeployAgent(_ context.Context, agent *woodpecker.Agent) error {
	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, p.userDataTemplate)
	if err != nil {
		return fmt.Errorf("%s: cloudinit.RenderUserDataTemplate: %w", p.name, err)
	}

	req := &packngo.DeviceCreateRequest{
		Hostname:       agent.Name,
		Plan:           p.plan,
		Metro:          p.metro,
		Facility:       slices.Clone(p.facility),
		OS:             p.operatingSys,
		BillingCycle:   p.billingCycle,
		ProjectID:      p.projectID,
		UserData:       userData,
		Tags:           p.deviceTags(),
		ProjectSSHKeys: slices.Clone(p.projectSSHKeys),
		SpotInstance:   p.spotInstance,
		SpotPriceMax:   p.spotPriceMax,
	}

	_, _, err = p.devices.Create(req)
	if err != nil {
		return fmt.Errorf("%s: Devices.Create: %w", p.name, err)
	}

	return nil
}

func (p *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	device, err := p.getAgent(ctx, agent.Name)
	if err != nil {
		return err
	}
	if device == nil {
		return nil
	}

	_, err = p.devices.Delete(device.ID, false)
	if err != nil {
		return fmt.Errorf("%s: Devices.Delete: %w", p.name, err)
	}

	return nil
}

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	devices, err := p.listPoolDevices(ctx)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(devices))
	for _, device := range devices {
		names = append(names, device.Hostname)
	}

	return names, nil
}

func (p *Provider) getAgent(ctx context.Context, hostname string) (*packngo.Device, error) {
	devices, err := p.listPoolDevices(ctx)
	if err != nil {
		return nil, err
	}

	var matches []packngo.Device
	for _, device := range devices {
		if device.Hostname == hostname {
			matches = append(matches, device)
		}
	}

	switch len(matches) {
	case 0:
		return nil, nil
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("%s: multiple devices found with hostname %s", p.name, hostname)
	}
}

func (p *Provider) listPoolDevices(_ context.Context) ([]packngo.Device, error) {
	devices, _, err := p.devices.List(p.projectID, &packngo.ListOptions{PerPage: deviceListPerPage})
	if err != nil {
		return nil, fmt.Errorf("%s: Devices.List: %w", p.name, err)
	}

	poolTag := poolTag(p.config.PoolID)
	filtered := make([]packngo.Device, 0, len(devices))
	for _, device := range devices {
		if slices.Contains(device.Tags, poolTag) {
			filtered = append(filtered, device)
		}
	}

	return filtered, nil
}

func (p *Provider) deviceTags() []string {
	tags := []string{
		poolTag(p.config.PoolID),
		imageTag(p.config.Image),
	}
	for _, tag := range p.tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed != "" {
			tags = append(tags, trimmed)
		}
	}
	return tags
}

func poolTag(poolID string) string {
	return engine.LabelPool + "=" + poolID
}

func imageTag(image string) string {
	return engine.LabelImage + "=" + image
}
