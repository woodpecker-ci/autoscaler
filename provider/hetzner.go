package provider

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hetznercloud/hcloud-go/hcloud"

	"github.com/woodpecker-ci/autoscaler/config"
	"github.com/woodpecker-ci/woodpecker/woodpecker-go/woodpecker"
)

type Hetzner struct {
	ApiToken   string
	ServerType string
	Config     *config.Config
	Location   string
	client     *hcloud.Client
}

func (p *Hetzner) Setup() error {
	p.client = hcloud.NewClient(hcloud.WithToken(p.ApiToken))
	return nil
}

func (p *Hetzner) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := getUserDataTemplate(p.Config, agent)
	if err != nil {
		return err
	}

	_, _, err = p.client.Server.Create(ctx, hcloud.ServerCreateOpts{
		Name:     agent.Name,
		UserData: userData,
		Image:    &hcloud.Image{Name: "ubuntu-22.04"},
		Location: &hcloud.Location{
			Name: p.Location,
		},
		ServerType: &hcloud.ServerType{
			Name: p.ServerType,
		},
	})

	return err
}

func (p *Hetzner) getAgent(ctx context.Context, agent *woodpecker.Agent) (*hcloud.Server, error) {
	server, _, err := p.client.Server.GetByName(ctx, agent.Name)
	return server, err
}

func (p *Hetzner) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	server, err := p.getAgent(ctx, agent)
	if err != nil {
		return err
	}

	if server == nil {
		return nil
	}

	_, _, err = p.client.Server.DeleteWithResult(ctx, server)
	return err
}

func (p *Hetzner) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	servers, err := p.client.Server.All(ctx)
	if err != nil {
		return nil, err
	}

	var names []string

	poolID := 1 // TODO

	r, _ := regexp.Compile(fmt.Sprintf("pool-%d-agent-.*?", poolID))

	for _, server := range servers {
		if r.MatchString(server.Name) {
			names = append(names, server.Name)
		}
	}

	return names, nil
}
