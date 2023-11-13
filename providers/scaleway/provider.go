package scaleway

import (
	"bytes"
	"context"
	"errors"
	"math/rand"
	"text/template"
	"time"

	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/woodpecker-go/woodpecker"
)

type Provider struct {
	scwCfg    Config
	engineCfg *config.Config
	client    *scw.Client
}

func New(scwCfg Config, engineCfg *config.Config) (engine.Provider, error) {
	client, err := scw.NewClient(scw.WithDefaultProjectID(scwCfg.DefaultProjectID), scw.WithAuth(scwCfg.AccessKey, scwCfg.SecretKey))
	if err != nil {
		return nil, err
	}

	return &Provider{
		scwCfg:    scwCfg,
		client:    client,
		engineCfg: engineCfg,
	}, nil
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	_, err := p.getInstance(ctx, agent.Name)
	if err != nil {
		var doesNotExists *InstanceDoesNotExists
		if !errors.As(err, &doesNotExists) {
			return err
		}
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
	pool := p.scwCfg.InstancePool[DefaultPool]
	zones, err := pool.Locality.ResolveZones()
	if err != nil {
		return nil, err
	}

	api := instance.NewAPI(p.client)
	project := pool.ProjectID

	if project == nil {
		project = &p.scwCfg.DefaultProjectID
	}

	for _, zone := range zones {
		req := instance.ListServersRequest{
			Zone:    zone,
			Project: project,
			Name:    scw.StringPtr(name),
			Tags:    pool.Tags,
		}

		resp, err := api.ListServers(&req, scw.WithContext(ctx))
		if err != nil {
			return nil, err
		}

		if resp.TotalCount > 0 {
			// TODO(raskyld): add a warning if there are more than 1 found, it means there are orphan resources
			return resp.Servers[0], nil
		}
	}

	return nil, &InstanceDoesNotExists{
		InstanceName: name,
		Project:      *project,
		Zones:        zones,
	}
}

func (p *Provider) getAllInstances(ctx context.Context) ([]*instance.Server, error) {
	pool := p.scwCfg.InstancePool[DefaultPool]
	zones, err := pool.Locality.ResolveZones()
	if err != nil {
		return nil, err
	}

	api := instance.NewAPI(p.client)
	instances := make([]*instance.Server, 0, 150)

	for _, zone := range zones {
		// TODO(raskyld): handle pagination for cases with more than 50 agents running per region
		req := instance.ListServersRequest{
			Zone:    zone,
			Project: pool.ProjectID,
			Tags:    pool.Tags,
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
	pool := p.scwCfg.InstancePool[DefaultPool]
	zones, err := pool.Locality.ResolveZones()
	if err != nil {
		return nil, err
	}

	// TODO(raskyld): Implement a well-balanced zone anti-affinity to spread instance
	// 								evenly among zones for greater resilience.
	random := rand.New(rand.NewSource(time.Now().Unix()))
	zone := zones[random.Intn(len(zones))]

	api := instance.NewAPI(p.client)

	req := instance.CreateServerRequest{
		Zone:              zone,
		Name:              agent.Name,
		DynamicIPRequired: scw.BoolPtr(true),
		CommercialType:    pool.CommercialType,
		Image:             pool.Image,
		Volumes: map[string]*instance.VolumeServerTemplate{
			"0": {
				Boot:       scw.BoolPtr(true),
				Size:       scw.SizePtr(pool.Storage),
				VolumeType: instance.VolumeVolumeTypeBSSD,
			},
		},
		EnableIPv6: pool.EnableIPv6,
		Project:    pool.ProjectID,
		Tags:       pool.Tags,
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

	ud, err := engine.RenderUserDataTemplate(p.engineCfg, agent, tpl)
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
