package scaleway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

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
	ErrServerTypeNotFound   = errors.New("server type not found")
	ErrImageNotFound        = errors.New("no configured image resolves for server type arch")
	ErrNoMatchingServerType = errors.New("no configured server type matches requested capability")
)

type provider struct {
	defaultProjectID string
	projectID        *string
	prefix           string
	tags             []string
	images           []string
	enableIPv6       bool
	storage          scw.Size
	storageType      instance.VolumeVolumeType
	config           *config.Config
	client           *scw.Client
	// candidates holds all resolved (serverType, image) pairs, in config order.
	// Built once in New(); DeployAgent filters by requested capability at call time.
	candidates []deployCandidate
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (types.Provider, error) {
	p := &provider{
		defaultProjectID: c.String("scaleway-project"),
		projectID:        scw.StringPtr(c.String("scaleway-project")),
		prefix:           c.String("scaleway-prefix"),
		tags:             c.StringSlice("scaleway-tags"),
		images:           c.StringSlice("scaleway-images"),
		enableIPv6:       c.Bool("scaleway-enable-ipv6"),
		storage:          scw.Size(c.Uint64("scaleway-storage-size") * units.GB),
		storageType:      instance.VolumeVolumeType(c.String("scaleway-storage-type")),
		config:           config,
	}

	var err error
	p.client, err = scw.NewClient(
		scw.WithDefaultProjectID(p.defaultProjectID),
		scw.WithAuth(c.String("scaleway-access-key"), c.String("scaleway-secret-key")),
	)
	if err != nil {
		return nil, err
	}

	if err := p.resolveCandidates(ctx, c.StringSlice("scaleway-server-types")); err != nil {
		return nil, err
	}

	return p, nil
}

// resolveCandidates resolves each "type:zone" entry from --scaleway-server-types.
// For each entry it fetches the ServerType (to get arch, ncpus, ram) and then
// finds the first image from --scaleway-images that resolves for that arch in
// that zone via ListImages. Entries that don't resolve are skipped with a
// warning. Hard error if no candidates survive.
func (p *provider) resolveCandidates(ctx context.Context, rawEntries []string) error {
	api := instance.NewAPI(p.client)

	for _, raw := range rawEntries {
		rawType, rawZone, _ := strings.Cut(raw, ":")
		if rawZone == "" {
			log.Error().Str("entry", raw).Msg("scaleway: server type entry missing zone (expected type:zone), skipping")
			continue
		}

		zone := scw.Zone(rawZone)
		if !zone.Exists() {
			return fmt.Errorf("%w: %s (from entry %q)", ErrInvalidZone, rawZone, raw)
		}

		resp, err := api.ListServersTypes(&instance.ListServersTypesRequest{
			Zone: zone,
		}, scw.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("scaleway: ListServersTypes zone=%s: %w", zone, err)
		}

		st, ok := resp.Servers[rawType]
		if !ok {
			log.Warn().Str("type", rawType).Str("zone", rawZone).Msg("scaleway: server type not found in zone, skipping")
			continue
		}
		if st.EndOfService {
			log.Warn().Str("type", rawType).Str("zone", rawZone).Msg("scaleway: server type is end-of-service, skipping")
			continue
		}

		archStr := string(st.Arch)
		imageID, imageName, err := p.resolveImage(ctx, api, zone, archStr)
		if err != nil {
			log.Warn().Str("type", rawType).Str("zone", rawZone).Str("arch", archStr).
				Msgf("scaleway: no image resolved (%s), skipping entry", err)
			continue
		}

		log.Info().
			Str("type", rawType).
			Str("zone", rawZone).
			Str("arch", archStr).
			Str("image", imageName).
			Str("image_id", imageID).
			Uint32("ncpus", st.Ncpus).
			Uint64("ram_bytes", st.RAM).
			Msg("scaleway: resolved deploy candidate")

		p.candidates = append(p.candidates, deployCandidate{
			rawType:    rawType,
			zone:       zone,
			serverType: st,
			imageID:    imageID,
			imageName:  imageName,
		})
	}

	if len(p.candidates) == 0 {
		return fmt.Errorf("scaleway: no valid deploy candidates after resolving --scaleway-server-types")
	}

	return nil
}

// resolveImage tries each name in p.images in order and returns the first one
// that resolves via ListImages for the given arch in the given zone.
func (p *provider) resolveImage(ctx context.Context, api *instance.API, zone scw.Zone, arch string) (imageID, imageName string, err error) {
	for _, name := range p.images {
		resp, err := api.ListImages(&instance.ListImagesRequest{
			Zone:   zone,
			Name:   scw.StringPtr(name),
			Arch:   scw.StringPtr(arch),
			Public: scw.BoolPtr(true),
		}, scw.WithContext(ctx))
		if err != nil {
			return "", "", fmt.Errorf("ListImages name=%s arch=%s zone=%s: %w", name, arch, zone, err)
		}
		if len(resp.Images) > 0 {
			img := resp.Images[0]
			return img.ID, img.Name, nil
		}
	}
	return "", "", fmt.Errorf("%w: tried %v for arch=%s zone=%s", ErrImageNotFound, p.images, arch, zone)
}

func (p *provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent, cb types.Capability) error {
	var matched []deployCandidate
	for _, c := range p.candidates {
		if "linux/"+scwArchToGoArch(c.serverType.Arch) == cb.Platform {
			matched = append(matched, c)
		}
	}
	if len(matched) == 0 {
		return fmt.Errorf("scaleway: %w: %s", ErrNoMatchingServerType, cb.Platform)
	}

	for i, c := range matched {
		err := p.createAndBoot(ctx, agent, c)
		if err == nil {
			return nil
		}
		if !isResourceUnavailable(err) {
			return err
		}
		if i < len(matched)-1 {
			log.Warn().Str("type", c.rawType).Str("zone", c.zone.String()).
				Msgf("scaleway: create failed (resource unavailable), trying next candidate: %s", err)
			continue
		}
		return fmt.Errorf("scaleway: all candidates exhausted for %s: %w", cb.Platform, err)
	}

	return nil
}

func (p *provider) createAndBoot(ctx context.Context, agent *woodpecker.Agent, c deployCandidate) error {
	inst, err := p.createInstance(ctx, agent, c)
	if err != nil {
		return err
	}
	if err := p.setCloudInit(ctx, agent, inst); err != nil {
		return err
	}
	log.Info().Str("type", c.rawType).Str("zone", c.zone.String()).
		Str("image", c.imageName).Msgf("scaleway: create agent %s", agent.Name)
	_, err = p.bootInstance(ctx, inst)
	return err
}

func (p *provider) Capabilities(_ context.Context) ([]types.Capability, error) {
	seen := make(map[string]struct{})
	var caps []types.Capability
	for _, c := range p.candidates {
		goarch := scwArchToGoArch(c.serverType.Arch)
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

// scwArchToGoArch maps Scaleway Arch values to Go GOARCH strings.
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

// isResourceUnavailable reports whether the error indicates the requested
// server type has no capacity in the zone (soft error, try next candidate).
func isResourceUnavailable(err error) bool {
	var scwErr *scw.ResponseError
	if errors.As(err, &scwErr) {
		return scwErr.Message == "server_type_unavailable" || scwErr.StatusCode == http.StatusServiceUnavailable
	}
	return false
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
	for _, zone := range p.candidateZones() {
		resp, err := api.ListServers(&instance.ListServersRequest{
			Zone:    zone,
			Project: project,
			Name:    scw.StringPtr(name),
			Tags:    p.tags,
		}, scw.WithContext(ctx))
		if err != nil {
			return nil, err
		}
		if resp.TotalCount > 0 {
			if resp.TotalCount > 1 {
				log.Warn().Msg("scaleway: found multiple instances with the same name, this may indicate orphaned resources")
			}
			return resp.Servers[0], nil
		}
	}
	return nil, nil
}

func (p *provider) getAllInstances(ctx context.Context) ([]*instance.Server, error) {
	api := instance.NewAPI(p.client)
	var instances []*instance.Server
	for _, zone := range p.candidateZones() {
		resp, err := api.ListServers(&instance.ListServersRequest{
			Zone:    zone,
			Project: p.projectID,
			Tags:    p.tags,
			PerPage: scw.Uint32Ptr(100), //nolint:mnd
		}, scw.WithContext(ctx), scw.WithAllPages())
		if err != nil {
			return nil, err
		}
		if resp.TotalCount > 0 {
			instances = append(instances, resp.Servers...)
		}
	}
	return instances, nil
}

// candidateZones returns the deduplicated set of zones across all candidates,
// preserving config order.
func (p *provider) candidateZones() []scw.Zone {
	seen := make(map[scw.Zone]struct{})
	var zones []scw.Zone
	for _, c := range p.candidates {
		if _, ok := seen[c.zone]; !ok {
			seen[c.zone] = struct{}{}
			zones = append(zones, c.zone)
		}
	}
	return zones
}

func (p *provider) createInstance(ctx context.Context, agent *woodpecker.Agent, c deployCandidate) (*instance.Server, error) {
	api := instance.NewAPI(p.client)
	res, err := api.CreateServer(&instance.CreateServerRequest{
		Zone:              c.zone,
		Name:              agent.Name,
		DynamicIPRequired: scw.BoolPtr(true),
		CommercialType:    c.rawType,
		Image:             scw.StringPtr(c.imageID),
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
	}, scw.WithContext(ctx))
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
	return api.SetServerUserData(&instance.SetServerUserDataRequest{
		Zone:     inst.Zone,
		ServerID: inst.ID,
		Key:      "cloud-init",
		Content:  bytes.NewBufferString(ud),
	}, scw.WithContext(ctx))
}

func (p *provider) deleteInstance(ctx context.Context, inst *instance.Server) error {
	if err := p.haltInstance(ctx, inst); err != nil {
		return err
	}

	api := instance.NewAPI(p.client)
	blockAPI := block.NewAPI(p.client)
	volumes := inst.Volumes

	// Delete server first
	if err := api.DeleteServer(&instance.DeleteServerRequest{
		Zone:     inst.Zone,
		ServerID: inst.ID,
	}, scw.WithContext(ctx)); err != nil {
		return err
	}

	var errs []error
	for _, volume := range volumes {
		switch volume.VolumeType {
		case instance.VolumeServerVolumeTypeLSSD:
			if err := api.DeleteVolume(&instance.DeleteVolumeRequest{
				VolumeID: volume.ID,
				Zone:     inst.Zone,
			}, scw.WithContext(ctx)); err != nil {
				errs = append(errs, fmt.Errorf("delete LSSD volume %s: %w", volume.ID, err))
			}

		case instance.VolumeServerVolumeTypeSbsVolume:
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
			// deleted automatically with server
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
