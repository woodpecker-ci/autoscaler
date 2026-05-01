package equinixmetal

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"text/template"

	metalv1 "github.com/equinix/equinix-sdk-go/services/metalv1"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var (
	ErrProjectIDRequired    = errors.New("project ID is required")
	ErrPlanRequired         = errors.New("at least one plan is required")
	ErrOperatingSysRequired = errors.New("operating system is required")
	ErrLocationRequired     = errors.New("either metro or facility must be set")
	ErrLocationConflict     = errors.New("metro and facility are mutually exclusive")
	ErrEmptyTag             = errors.New("tags must not be empty")
	ErrReservedTagPrefix    = errors.New("illegal tag prefix")
)

const deviceListPerPage int32 = 100

type deviceCreateRequest struct {
	Hostname       string
	Plan           string
	Metro          string
	Facility       []string
	OperatingSys   string
	BillingCycle   string
	UserData       string
	Tags           []string
	ProjectSSHKeys []string
	SpotInstance   bool
	SpotPriceMax   float64
}

type deviceRecord struct {
	ID       string
	Hostname string
	Tags     []string
}

type devicesService interface {
	Create(context.Context, string, deviceCreateRequest) (*deviceRecord, error)
	List(context.Context, string, string) ([]deviceRecord, error)
	Delete(context.Context, string, bool) error
}

type metalDevicesService struct {
	client *metalv1.APIClient
	token  string
}

type Provider struct {
	name             string
	projectID        string
	metro            string
	facility         []string
	plans            []string
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
		plans:          c.StringSlice("equinixmetal-plan"),
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

	cfg := metalv1.NewConfiguration()
	cfg.UserAgent = "woodpecker-autoscaler"
	p.devices = &metalDevicesService{
		client: metalv1.NewAPIClient(cfg),
		token:  c.String("equinixmetal-api-token"),
	}

	return p, nil
}

func (p *Provider) validate() error {
	switch {
	case p.projectID == "":
		return ErrProjectIDRequired
	case len(p.plans) == 0:
		return ErrPlanRequired
	case p.operatingSys == "":
		return ErrOperatingSysRequired
	case p.metro == "" && len(p.facility) == 0:
		return ErrLocationRequired
	case p.metro != "" && len(p.facility) > 0:
		return ErrLocationConflict
	}

	for _, plan := range p.plans {
		if strings.TrimSpace(plan) == "" {
			return ErrPlanRequired
		}
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

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, p.userDataTemplate)
	if err != nil {
		return fmt.Errorf("%s: cloudinit.RenderUserDataTemplate: %w", p.name, err)
	}

	_, err = p.devices.Create(ctx, p.projectID, deviceCreateRequest{
		Hostname:       agent.Name,
		Plan:           p.primaryPlan(),
		Metro:          p.metro,
		Facility:       slices.Clone(p.facility),
		OperatingSys:   p.operatingSys,
		BillingCycle:   p.billingCycle,
		UserData:       userData,
		Tags:           p.deviceTags(),
		ProjectSSHKeys: slices.Clone(p.projectSSHKeys),
		SpotInstance:   p.spotInstance,
		SpotPriceMax:   p.spotPriceMax,
	})
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

	if err := p.devices.Delete(ctx, device.ID, false); err != nil {
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

func (p *Provider) getAgent(ctx context.Context, hostname string) (*deviceRecord, error) {
	devices, err := p.listPoolDevices(ctx)
	if err != nil {
		return nil, err
	}

	var matches []deviceRecord
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

func (p *Provider) listPoolDevices(ctx context.Context) ([]deviceRecord, error) {
	poolTag := poolTag(p.config.PoolID)
	devices, err := p.devices.List(ctx, p.projectID, poolTag)
	if err != nil {
		return nil, fmt.Errorf("%s: Devices.List: %w", p.name, err)
	}

	filtered := make([]deviceRecord, 0, len(devices))
	for _, device := range devices {
		if slices.Contains(device.Tags, poolTag) {
			filtered = append(filtered, device)
		}
	}

	return filtered, nil
}

func (p *Provider) primaryPlan() string {
	return strings.TrimSpace(p.plans[0])
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

func (m *metalDevicesService) Create(ctx context.Context, projectID string, req deviceCreateRequest) (*deviceRecord, error) {
	payload, err := createDevicePayload(req)
	if err != nil {
		return nil, err
	}

	device, _, err := m.client.DevicesApi.CreateDevice(m.withAuth(ctx), projectID).CreateDeviceRequest(payload).Execute()
	if err != nil {
		return nil, err
	}
	record := deviceRecordFromAPI(device)
	return &record, nil
}

func (m *metalDevicesService) List(ctx context.Context, projectID, tag string) ([]deviceRecord, error) {
	devices, err := m.client.DevicesApi.FindProjectDevices(m.withAuth(ctx), projectID).PerPage(deviceListPerPage).Tag(tag).ExecuteWithPagination()
	if err != nil {
		return nil, err
	}

	items := make([]deviceRecord, 0, len(devices.Devices))
	for i := range devices.Devices {
		items = append(items, deviceRecordFromAPI(&devices.Devices[i]))
	}
	return items, nil
}

func (m *metalDevicesService) Delete(ctx context.Context, id string, force bool) error {
	_, err := m.client.DevicesApi.DeleteDevice(m.withAuth(ctx), id).ForceDelete(force).Execute()
	return err
}

func (m *metalDevicesService) withAuth(ctx context.Context) context.Context {
	return context.WithValue(ctx, metalv1.ContextAPIKeys, map[string]metalv1.APIKey{
		"x_auth_token": {Key: m.token},
	})
}

func createDevicePayload(req deviceCreateRequest) (metalv1.CreateDeviceRequest, error) {
	billingCycle, err := metalv1.NewDeviceCreateInputBillingCycleFromValue(req.BillingCycle)
	if err != nil {
		return metalv1.CreateDeviceRequest{}, err
	}

	if req.Metro != "" {
		payload := metalv1.NewDeviceCreateInMetroInput(req.Metro, req.OperatingSys, req.Plan)
		payload.Hostname = &req.Hostname
		payload.BillingCycle = billingCycle
		payload.Userdata = &req.UserData
		payload.Tags = slices.Clone(req.Tags)
		payload.ProjectSshKeys = slices.Clone(req.ProjectSSHKeys)
		payload.SpotInstance = &req.SpotInstance
		payload.SpotPriceMax = float32Ptr(float32(req.SpotPriceMax))
		return metalv1.DeviceCreateInMetroInputAsCreateDeviceRequest(payload), nil
	}

	payload := metalv1.NewDeviceCreateInFacilityInput(slices.Clone(req.Facility), req.OperatingSys, req.Plan)
	payload.Hostname = &req.Hostname
	payload.BillingCycle = billingCycle
	payload.Userdata = &req.UserData
	payload.Tags = slices.Clone(req.Tags)
	payload.ProjectSshKeys = slices.Clone(req.ProjectSSHKeys)
	payload.SpotInstance = &req.SpotInstance
	payload.SpotPriceMax = float32Ptr(float32(req.SpotPriceMax))
	return metalv1.DeviceCreateInFacilityInputAsCreateDeviceRequest(payload), nil
}

func deviceRecordFromAPI(device *metalv1.Device) deviceRecord {
	if device == nil {
		return deviceRecord{}
	}

	return deviceRecord{
		ID:       device.GetId(),
		Hostname: device.GetHostname(),
		Tags:     slices.Clone(device.Tags),
	}
}

func float32Ptr(v float32) *float32 {
	return &v
}
