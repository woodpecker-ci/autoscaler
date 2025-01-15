package scaleway

import (
	"bytes"
	"context"
	"errors"
	"math/rand"
	"text/template"
	"time"

	"github.com/docker/go-units"
	"github.com/rs/zerolog/log"
	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	"github.com/urfave/cli/v2"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v2/woodpecker-go/woodpecker"
)

type Provider struct {
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
	config           *config.Config
	client           *scw.Client
}

func New(c *cli.Context, config *config.Config) (engine.Provider, error) {
	if !c.IsSet("scw-instance-type") {
		return nil, errors.New("scw-instance-type must be set")
	}

	if !c.IsSet("scw-tags") {
		return nil, errors.New("scw-tags must be set")
	}

	if !c.IsSet("scw-project") {
		return nil, errors.New("scw-project must be set")
	}

	if !c.IsSet("scw-secret-key") {
		return nil, errors.New("scw-secret-key must be set")
	}

	if !c.IsSet("scw-access-key") {
		return nil, errors.New("scw-access-key must be set")
	}

	d := &Provider{
		secretKey:        c.String("scw-secret-key"),
		accessKey:        c.String("scw-access-key"),
		defaultProjectID: c.String("scw-project"),
		projectID:        scw.StringPtr(c.String("scw-project")),
		prefix:           c.String("scw-prefix"),
		tags:             c.StringSlice("scw-tags"),
		commercialType:   c.String("scw-instance-type"),
		image:            c.String("scw-image"),
		enableIPv6:       c.Bool("scw-enable-ipv6"),
		storage:          scw.Size(c.Uint64("swc-storage-size") * units.GB),
		config:           config,
	}

	zone := scw.Zone(c.String("scw-zone"))
	if !zone.Exists() {
		return nil, errors.New(zone.String() + " is not a valid zone")
	}
	d.zones = []scw.Zone{zone}

	var err error
	d.client, err = scw.NewClient(scw.WithDefaultProjectID(d.defaultProjectID), scw.WithAuth(d.accessKey, d.secretKey))

	return d, err
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
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

func (p *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	inst, err := p.getInstance(ctx, agent.Name)
	if err != nil {
		return err
	}

	return p.deleteInstance(ctx, inst)
}

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
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

func (p *Provider) getInstance(ctx context.Context, name string) (*instance.Server, error) {
	if err := p.resolveZones(); err != nil {
		return nil, err
	}

	api := instance.NewAPI(p.client)
	project := p.projectID

	if project == nil {
		project = &p.defaultProjectID
	}

	for _, zone := range p.zones {
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
				log.Warn().Msg("multiple instances with found with the same name, this may indicate orphaned resources")
			}
			return resp.Servers[0], nil
		}
	}

	return nil, nil
}

func (p *Provider) getAllInstances(ctx context.Context) ([]*instance.Server, error) {
	if err := p.resolveZones(); err != nil {
		return nil, err
	}

	api := instance.NewAPI(p.client)
	instances := make([]*instance.Server, 0)

	for _, zone := range p.zones {
		// TODO(raskyld): handle pagination for cases with more than 50 agents running per region
		req := instance.ListServersRequest{
			Zone:    zone,
			Project: p.projectID,
			Tags:    p.tags,
		}

		resp, err := api.ListServers(&req, scw.WithContext(ctx))
		if err != nil {
			return nil, err
		}

		if resp.TotalCount > 0 {
			instances = append(instances, resp.Servers...)
		}
	}

	return instances, nil
}

func (p *Provider) createInstance(ctx context.Context, agent *woodpecker.Agent) (*instance.Server, error) {
	if err := p.resolveZones(); err != nil {
		return nil, err
	}

	// TODO(raskyld): Implement a well-balanced zone anti-affinity to spread instance
	// 								evenly among zones for greater resilience.
	random := rand.New(rand.NewSource(time.Now().Unix()))
	zone := p.zones[random.Intn(len(p.zones))]

	api := instance.NewAPI(p.client)

	req := instance.CreateServerRequest{
		Zone:              zone,
		Name:              agent.Name,
		DynamicIPRequired: scw.BoolPtr(true),
		CommercialType:    p.commercialType,
		Image:             p.image,
		Volumes: map[string]*instance.VolumeServerTemplate{
			"0": {
				Boot:       scw.BoolPtr(true),
				Size:       scw.SizePtr(p.storage),
				VolumeType: instance.VolumeVolumeTypeBSSD,
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

func (p *Provider) setCloudInit(ctx context.Context, agent *woodpecker.Agent, inst *instance.Server) error {
	tpl, err := template.New("user-data").Parse(engine.CloudInitUserDataUbuntuDefault)
	if err != nil {
		return err
	}

	ud, err := engine.RenderUserDataTemplate(p.config, agent, tpl)
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

func (p *Provider) deleteInstance(ctx context.Context, inst *instance.Server) error {
	err := p.haltInstance(ctx, inst)
	if err != nil {
		return err
	}

	api := instance.NewAPI(p.client)

	return api.DeleteServer(&instance.DeleteServerRequest{
		Zone:     inst.Zone,
		ServerID: inst.ID,
	}, scw.WithContext(ctx))
}

func (p *Provider) bootInstance(ctx context.Context, inst *instance.Server) (*instance.ServerActionResponse, error) {
	api := instance.NewAPI(p.client)

	return api.ServerAction(&instance.ServerActionRequest{
		Zone:     inst.Zone,
		ServerID: inst.ID,
		Action:   instance.ServerActionPoweron,
	}, scw.WithContext(ctx))
}

func (p *Provider) haltInstance(ctx context.Context, inst *instance.Server) error {
	api := instance.NewAPI(p.client)

	return api.ServerActionAndWait(&instance.ServerActionAndWaitRequest{
		Zone:     inst.Zone,
		ServerID: inst.ID,
		Action:   instance.ServerActionPoweroff,
	}, scw.WithContext(ctx))
}

func (p *Provider) resolveZones() error {
	if p.region != nil {
		if !p.region.Exists() {
			return errors.New("you specified an invalid region: " + p.region.String())
		}

		p.zones = p.region.GetZones()

		return nil
	}

	if len(p.zones) == 0 {
		return errors.New("you need to specify a valid locality")
	}

	for _, zone := range p.zones {
		if !zone.Exists() {
			return errors.New("you specified a non-existing zone: " + zone.String())
		}
	}

	return nil
}
