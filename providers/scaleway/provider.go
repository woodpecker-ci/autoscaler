package scaleway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/docker/go-units"
	"github.com/rs/zerolog/log"
	"github.com/scaleway/scaleway-sdk-go/api/block/v1"
	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var (
	ErrInvalidZone          = errors.New("invalid zone")
	ErrInvalidRegion        = errors.New("invalid region")
	ErrParameterNotSet      = errors.New("required parameter not set")
	ErrRegionOrZoneNotSet   = errors.New("region or zone not set")
	ErrInstanceTypeNotFound = errors.New("instance type not found in zone")
)

type provider struct {
	secretKey        string
	accessKey        string
	defaultProjectID string
	zones            []scw.Zone
	region           *scw.Region
	projectID        *string
	prefix           string
	tags             []string
	commercialType   string
	image            string
	enableIPv6       bool
	storage          scw.Size
	storageType      instance.VolumeVolumeType
	config           *config.Config
	client           *scw.Client
	// resolved config: one ServerType entry per zone (zones where the type is
	// not available or end-of-service are silently dropped).
	serverTypes map[scw.Zone]*instance.ServerType
}

func New(_ context.Context, c *cli.Command, config *config.Config) (types.Provider, error) {
	p := &provider{
		secretKey:        c.String("scaleway-secret-key"),
		accessKey:        c.String("scaleway-access-key"),
		defaultProjectID: c.String("scaleway-project"),
		projectID:        scw.StringPtr(c.String("scaleway-project")),
		prefix:           c.String("scaleway-prefix"),
		tags:             c.StringSlice("scaleway-tags"),
		commercialType:   c.String("scaleway-instance-type"),
		image:            c.String("scaleway-image"),
		enableIPv6:       c.Bool("scaleway-enable-ipv6"),
		storage:          scw.Size(c.Uint64("scaleway-storage-size") * units.GB),
		storageType:      instance.VolumeVolumeType(c.String("scaleway-storage-type")),
		config:           config,
		serverTypes:      make(map[scw.Zone]*instance.ServerType),
	}

	rawZones := c.StringSlice("scaleway-zones")
	for _, z := range rawZones {
		p.zones = append(p.zones, scw.Zone(z))
	}

	var err error
	p.client, err = scw.NewClient(scw.WithDefaultProjectID(p.defaultProjectID), scw.WithAuth(p.accessKey, p.secretKey))
	if err != nil {
		return nil, err
	}

	// Resolve zone first — availability of instance types is per-zone.
	if err := p.resolveZones(); err != nil {
		return nil, err
	}

	// Then resolve the configured instance type in each zone.
	if err := p.resolveServerTypes(context.Background()); err != nil {
		return nil, err
	}

	p.printResolvedConfig()

	return p, nil
}

// resolveServerTypes queries each zone for the configured commercialType and
// stores the result. Zones where the type is absent or end-of-service are
// logged and skipped rather than treated as hard errors, because a valid
// multi-zone config may have the type available in only a subset.
func (p *provider) resolveServerTypes(ctx context.Context) error {
	api := instance.NewAPI(p.client)
	for _, zone := range p.zones {
		resp, err := api.ListServersTypes(&instance.ListServersTypesRequest{
			Zone: zone,
		}, scw.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("scaleway: ListServersTypes zone=%s: %w", zone, err)
		}
		st, ok := resp.Servers[p.commercialType]
		if !ok {
			log.Warn().Str("zone", zone.String()).Str("type", p.commercialType).Msg("instance type not available in zone, skipping")
			continue
		}
		if st.EndOfService {
			log.Warn().Str("zone", zone.String()).Str("type", p.commercialType).Msg("instance type is end-of-service in zone, skipping")
			continue
		}
		p.serverTypes[zone] = st
	}
	if len(p.serverTypes) == 0 {
		return fmt.Errorf("%w: %s is not available in any configured zone", ErrInstanceTypeNotFound, p.commercialType)
	}
	return nil
}

func (p *provider) printResolvedConfig() {
	for zone, st := range p.serverTypes {
		log.Info().
			Str("zone", zone.String()).
			Str("type", p.commercialType).
			Str("arch", string(st.Arch)).
			Uint32("ncpus", st.Ncpus).
			Uint64("ram_bytes", st.RAM).
			Msg("deploy instance type")
	}
}

func (p *provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent, cb types.Capability) error {
	// Validate the requested capability against what the resolved server types
	// actually support before touching the API.
	if err := p.validateCapability(cb); err != nil {
		return err
	}

	_, err := p.getInstance(ctx, agent.Name)
	if err != nil {
		return err
	}

	inst, err := p.createInstance(ctx, agent)
	if err != nil {
		return err
	}

	err = p.setCloudInit(ctx, agent, inst)
	if err != nil {
		return err
	}

	// NB(raskyld): use the value for logging purpose once we implement slog
	_, err = p.bootInstance(ctx, inst)
	return err
}

// validateCapability checks the requested (platform, backend) pair against
// every resolved zone's server type. Returns nil if at least one zone can
// satisfy the request.
func (p *provider) validateCapability(cb types.Capability) error {
	for _, st := range p.serverTypes {
		goarch := scwArchToGoArch(st.Arch)
		if goarch == "" {
			continue
		}
		if "linux/"+goarch == cb.Platform && cb.Backend == types.BackendDocker {
			return nil
		}
	}
	return fmt.Errorf("scaleway: %s does not support requested capability platform=%s backend=%s",
		p.commercialType, cb.Platform, cb.Backend)
}

func (p *provider) Capabilities(_ context.Context) ([]types.Capability, error) {
	seen := make(map[string]struct{})
	var caps []types.Capability
	for _, st := range p.serverTypes {
		goarch := scwArchToGoArch(st.Arch)
		if goarch == "" {
			continue
		}
		platform := "linux/" + goarch
		if _, exists := seen[platform]; exists {
			continue
		}
		seen[platform] = struct{}{}
		caps = append(caps, types.Capability{
			Platform: platform,
			Backend:  types.BackendDocker,
		})
	}
	return caps, nil
}

// scwArchToGoArch maps Scaleway Arch values to Go GOARCH strings used in
// the woodpecker platform label ("linux/<goarch>").
func scwArchToGoArch(a instance.Arch) string {
	switch a {
	case instance.ArchX86_64:
		return "amd64"
	case instance.ArchArm64:
		return "arm64"
	case instance.ArchArm:
		return "arm"
	default:
		return ""
	}
}

func (p *provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	inst, err := p.getInstance(ctx, agent.Name)
	if err != nil {
		return err
	}

	return p.deleteInstance(ctx, inst)
}

func (p *provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	instances, err := p.getAllInstances(ctx)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(instances))
	for _, inst := range instances {
		names = append(names, inst.Name)
	}

	return names, nil
}

func (p *provider) getInstance(ctx context.Context, name string) (*instance.Server, error) {
	api := instance.NewAPI(p.client)
	project := p.projectID

	if project == nil {
		project = &p.defaultProjectID
	}

	for zone := range p.serverTypes {
		req := instance.ListServersRequest{
			Zone:    zone,
			Project: project,
			Name:    scw.StringPtr(name),
			Tags:    p.tags,
		}

		resp, err := api.ListServers(&req, scw.WithContext(ctx))
		if err != nil {
			return nil, err
		}

		if resp.TotalCount > 0 {
			if resp.TotalCount > 1 {
				log.Warn().Msg("found multiple instances with the same name, this may indicate orphaned resources")
			}
			return resp.Servers[0], nil
		}
	}

	return nil, nil
}

func (p *provider) getAllInstances(ctx context.Context) ([]*instance.Server, error) {
	api := instance.NewAPI(p.client)
	instances := make([]*instance.Server, 0)

	for zone := range p.serverTypes {
		req := instance.ListServersRequest{
			Zone:    zone,
			Project: p.projectID,
			Tags:    p.tags,
			PerPage: scw.Uint32Ptr(100), //nolint:mnd
		}

		resp, err := api.ListServers(&req, scw.WithContext(ctx), scw.WithAllPages())
		if err != nil {
			return nil, err
		}

		if resp.TotalCount > 0 {
			instances = append(instances, resp.Servers...)
		}
	}

	return instances, nil
}

func (p *provider) createInstance(ctx context.Context, agent *woodpecker.Agent) (*instance.Server, error) {
	// TODO(raskyld): Implement a well-balanced zone anti-affinity to spread instance
	// 					evenly among zones for greater resilience.
	zones := make([]scw.Zone, 0, len(p.serverTypes))
	for z := range p.serverTypes {
		zones = append(zones, z)
	}
	random := rand.New(rand.NewSource(time.Now().Unix()))
	zone := zones[random.Intn(len(zones))]

	api := instance.NewAPI(p.client)

	req := instance.CreateServerRequest{
		Zone:              zone,
		Name:              agent.Name,
		DynamicIPRequired: scw.BoolPtr(true),
		CommercialType:    p.commercialType,
		Image:             scw.StringPtr(p.image),
		Volumes: map[string]*instance.VolumeServerTemplate{
			"0": {
				Boot:       scw.BoolPtr(true),
				Size:       scw.SizePtr(p.storage),
				VolumeType: p.storageType,
			},
		},
		EnableIPv6: &p.enableIPv6,
		Project:    p.projectID,
		Tags:       p.tags,
	}

	res, err := api.CreateServer(&req, scw.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	return res.Server, nil
}

func (p *provider) setCloudInit(ctx context.Context, agent *woodpecker.Agent, inst *instance.Server) error {
	ud, err := cloudinit.RenderUserDataTemplate(p.config, agent, nil)
	if err != nil {
		return err
	}

	api := instance.NewAPI(p.client)

	req := instance.SetServerUserDataRequest{
		Zone:     inst.Zone,
		ServerID: inst.ID,
		Key:      "cloud-init",
		Content:  bytes.NewBufferString(ud),
	}

	err = api.SetServerUserData(&req, scw.WithContext(ctx))
	if err != nil {
		return err
	}

	return nil
}

func (p *provider) deleteInstance(ctx context.Context, inst *instance.Server) error {
	err := p.haltInstance(ctx, inst)
	if err != nil {
		return err
	}

	api := instance.NewAPI(p.client)
	blockAPI := block.NewAPI(p.client)

	// Capture volumes before deletion (server deletion detaches them)
	volumes := inst.Volumes

	// Delete server first
	err = api.DeleteServer(&instance.DeleteServerRequest{
		Zone:     inst.Zone,
		ServerID: inst.ID,
	}, scw.WithContext(ctx))
	if err != nil {
		return err
	}

	// Delete volumes - collect all errors
	var errs []error
	for _, volume := range volumes {
		switch volume.VolumeType {
		case instance.VolumeServerVolumeTypeLSSD:
			// Local SSD - delete via instance API
			if err := api.DeleteVolume(&instance.DeleteVolumeRequest{
				VolumeID: volume.ID,
				Zone:     inst.Zone,
			}, scw.WithContext(ctx)); err != nil {
				errs = append(errs, fmt.Errorf("delete LSSD volume %s: %w", volume.ID, err))
			}

		case instance.VolumeServerVolumeTypeSbsVolume:
			// Block storage - wait for available status, then delete via block API
			terminalStatus := block.VolumeStatusAvailable
			if _, err := blockAPI.WaitForVolume(&block.WaitForVolumeRequest{
				VolumeID:       volume.ID,
				Zone:           inst.Zone,
				TerminalStatus: &terminalStatus,
			}, scw.WithContext(ctx)); err != nil {
				errs = append(errs, fmt.Errorf("wait for SBS volume %s: %w", volume.ID, err))
				continue
			}

			if err := blockAPI.DeleteVolume(&block.DeleteVolumeRequest{
				VolumeID: volume.ID,
				Zone:     inst.Zone,
			}, scw.WithContext(ctx)); err != nil {
				errs = append(errs, fmt.Errorf("delete SBS volume %s: %w", volume.ID, err))
			}

		case instance.VolumeServerVolumeTypeScratch:
			// Scratch volumes are automatically deleted with the server
		}
	}

	return errors.Join(errs...)
}

func (p *provider) bootInstance(ctx context.Context, inst *instance.Server) (*instance.ServerActionResponse, error) {
	api := instance.NewAPI(p.client)

	return api.ServerAction(&instance.ServerActionRequest{
		Zone:     inst.Zone,
		ServerID: inst.ID,
		Action:   instance.ServerActionPoweron,
	}, scw.WithContext(ctx))
}

func (p *provider) haltInstance(ctx context.Context, inst *instance.Server) error {
	api := instance.NewAPI(p.client)

	return api.ServerActionAndWait(&instance.ServerActionAndWaitRequest{
		Zone:     inst.Zone,
		ServerID: inst.ID,
		Action:   instance.ServerActionPoweroff,
	}, scw.WithContext(ctx))
}

func (p *provider) resolveZones() error {
	if p.region != nil {
		if !p.region.Exists() {
			return fmt.Errorf("%w: %s", ErrInvalidRegion, p.region.String())
		}

		p.zones = p.region.GetZones()

		return nil
	}

	if len(p.zones) == 0 {
		return ErrRegionOrZoneNotSet
	}

	for _, zone := range p.zones {
		if !zone.Exists() {
			return fmt.Errorf("%w: %s", ErrInvalidZone, zone.String())
		}
	}

	return nil
}
