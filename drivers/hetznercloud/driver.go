package hetznercloud

import (
	"context"
	"fmt"
	"regexp"
	"text/template"

	"github.com/hetznercloud/hcloud-go/hcloud"
	"github.com/urfave/cli/v2"

	"github.com/woodpecker-ci/autoscaler/config"
	"github.com/woodpecker-ci/autoscaler/engine"
	"github.com/woodpecker-ci/woodpecker/woodpecker-go/woodpecker"
)

type Driver struct {
	ApiToken   string
	ServerType string
	UserData   *template.Template
	Image      string
	SSHKeyID   int
	Config     *config.Config
	Location   string
	Name       string
	client     *hcloud.Client
}

func New(c *cli.Context, config *config.Config) (engine.Provider, error) {
	d := &Driver{
		ApiToken:   c.String("hetznercloud-api-token"),
		Location:   c.String("hetznercloud-location"),
		ServerType: c.String("hetznercloud-server-type"),
		Image:      c.String("hetznercloud-image"),
		SSHKeyID:   c.Int("hetznercloud-ssh-key-id"),
		Config:     config,
	}

	d.client = hcloud.NewClient(hcloud.WithToken(d.ApiToken))

	userdata, err := template.New("user-data").Parse(c.String("hetznercloud-user-data"))
	if err != nil {
		return nil, err
	}

	d.UserData = userdata

	return d, nil
}

func (d *Driver) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	sshKeys := []*hcloud.SSHKey{}

	if d.SSHKeyID > 0 {
		sshKeys = append(sshKeys, &hcloud.SSHKey{
			ID: d.SSHKeyID,
		})
	}

	userdataString, err := engine.RenderUserDataTemplate(d.Config, agent, d.UserData)
	if err != nil {
		return err
	}

	image, _, err := d.client.Image.GetByName(ctx, d.Image)
	if err != nil {
		return err
	}

	_, _, err = d.client.Server.Create(ctx, hcloud.ServerCreateOpts{
		Name:     agent.Name,
		UserData: userdataString,
		Image:    image,
		Location: &hcloud.Location{
			Name: d.Location,
		},
		ServerType: &hcloud.ServerType{
			Name: d.ServerType,
		},
		SSHKeys: sshKeys,
	})

	return err
}

func (d *Driver) getAgent(ctx context.Context, agent *woodpecker.Agent) (*hcloud.Server, error) {
	server, _, err := d.client.Server.GetByName(ctx, agent.Name)
	return server, err
}

func (d *Driver) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	server, err := d.getAgent(ctx, agent)
	if err != nil {
		return err
	}

	if server == nil {
		return nil
	}

	_, _, err = d.client.Server.DeleteWithResult(ctx, server)
	return err
}

func (d *Driver) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	servers, err := d.client.Server.All(ctx)
	if err != nil {
		return nil, err
	}

	var names []string

	r, _ := regexp.Compile(fmt.Sprintf("pool-%s-agent-.*?", d.Config.PoolID))

	for _, server := range servers {
		if r.MatchString(server.Name) {
			names = append(names, server.Name)
		}
	}

	return names, nil
}
