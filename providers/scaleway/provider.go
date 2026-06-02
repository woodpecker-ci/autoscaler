package scaleway

import (
	"bytes"
	"context"
	"errors"
	"fmt"

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

type provider struct {
	projectID   *string
	prefix      string
	tags        []string
	images      []string
	enableIPv6  bool
	storage     scw.Size
	storageType instance.VolumeVolumeType
	config      *config.Config
	client      *scw.Client
	// candidates holds all resolved (serverType, image) pairs, in config order.
	candidates []deployCandidate
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (types.Provider, error) {
	// make sure required configs are set
	if !c.IsSet("scaleway-access-key") {
		return nil, fmt.Errorf("WOODPECKER_SCALEWAY_ACCESS_KEY is missing")
	}
	if !c.IsSet("scaleway-secret-key") {
		return nil, fmt.Errorf("WOODPECKER_SCALEWAY_SECRET_KEY is missing")
	}
	if !c.IsSet("scaleway-server-types") {
		return nil, fmt.Errorf("WOODPECKER_SCALEWAY_SERVER_TYPES is missing")
	}
	if !c.IsSet("scaleway-project") {
		return nil, fmt.Errorf("WOODPECKER_SCALEWAY_PROJECT is missing")
	}
	if !c.IsSet("scaleway-tags") {
		log.Warn().Msg("\"WOODPECKER_SCALEWAY_TAGS\" is not set, all scaleway instances are managed by autoscaler!")
	}

	defaultProjectID := c.String("scaleway-project")

	// load config
	p := &provider{
		projectID:   scw.StringPtr(defaultProjectID),
		prefix:      c.String("scaleway-prefix"),
		tags:        c.StringSlice("scaleway-tags"),
		images:      c.StringSlice("scaleway-images"),
		enableIPv6:  c.Bool("scaleway-enable-ipv6"),
		storage:     scw.Size(c.Uint64("scaleway-storage-size") * units.GB),
		storageType: instance.VolumeVolumeType(c.String("scaleway-storage-type")),
		config:      config,
	}

	var err error
	p.client, err = scw.NewClient(
		scw.WithDefaultProjectID(defaultProjectID),
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

func (p *provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent, cb types.Capability) error {
	if cb.Backend != types.BackendDocker {
		fmt.Errorf("scaleway only support docker backend")
	}

	if len(p.candidates) == 0 {
		return fmt.Errorf("scaleway: no candidates to deploy from")
	}

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
		if i < len(p.candidates)-1 {
			log.Warn().Str("type", c.rawType).Str("zone", c.zone.String()).
				Msgf("scaleway: create failed (resource unavailable), trying next candidate: %s", err)
			continue
		}
		return fmt.Errorf("scaleway: all candidates exhausted: %w", err)
	}

	return nil
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

func (p *provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	inst, err := p.getInstance(ctx, agent.Name)
	if err != nil {
		if errors.Is(err, ErrInstanceNotFound) {
			log.Warn().Str("agent", agent.Name).Msg("scaleway: instance not found, nothing to remove")
			return nil
		}
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
	ud, err := cloudinit.RenderUserDataTemplate(p.config, agent, cloudinit.RenderOption{})
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

func (p *provider) bootInstance(ctx context.Context, inst *instance.Server) (*instance.ServerActionResponse, error) {
	api := instance.NewAPI(p.client)
	return api.ServerAction(&instance.ServerActionRequest{
		Zone:     inst.Zone,
		ServerID: inst.ID,
		Action:   instance.ServerActionPoweron,
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

func (p *provider) haltInstance(ctx context.Context, inst *instance.Server) error {
	api := instance.NewAPI(p.client)
	return api.ServerActionAndWait(&instance.ServerActionAndWaitRequest{
		Zone:     inst.Zone,
		ServerID: inst.ID,
		Action:   instance.ServerActionPoweroff,
	}, scw.WithContext(ctx))
}
