package provider

import (
	"context"

	"github.com/hetznercloud/hcloud-go/hcloud"

	"github.com/woodpecker-ci/woodpecker/server/model"
)

type Hetzner struct {
	ApiToken string
	client   *hcloud.Client
}

var template = `
static USER_DATA_TEMPLATE: &str = r#"#cloud-config
write_files:
- content: |
    # docker-compose.yml
    version: '3'
    services:
      woodpecker-agent:
        image: {{ image }}
        command: agent
        restart: always
        volumes:
          - /var/run/docker.sock:/var/run/docker.sock
        environment:
          {{#each params}}
          - {{ this.0 }}={{ this.1 }}
          {{/each}}
  path: /root/docker-compose.yml
runcmd:
- [ sh, -xc, "cd /root; docker run --rm --privileged multiarch/qemu-user-static --reset -p yes; docker compose up -d" ]
`

func (p *Hetzner) Init() error {
	p.client = hcloud.NewClient(hcloud.WithToken(p.ApiToken))
	return nil
}

func (p *Hetzner) DeployAgent(ctx context.Context, agent *model.Agent) error {
	_, _, err := p.client.Server.Create(ctx, hcloud.ServerCreateOpts{
		Name:  agent.Name,
		Image: &hcloud.Image{Name: "ubuntu-20.04"},
		Location: &hcloud.Location{
			Name: "nbg1",
		},
		ServerType: &hcloud.ServerType{
			Name: "cx11",
		},
		UserData: template,
	})

	return err
}

func (p *Hetzner) RemoveAgent(ctx context.Context, agent *model.Agent) error {
	_, err := p.client.Server.Delete(ctx, &hcloud.Server{Name: agent.Name})
	return err
}
