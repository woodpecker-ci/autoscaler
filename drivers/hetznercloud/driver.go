package hetznercloud

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"text/template"

	"github.com/hetznercloud/hcloud-go/hcloud"
	"github.com/urfave/cli/v2"
	"golang.org/x/exp/maps"

	"github.com/woodpecker-ci/autoscaler/config"
	"github.com/woodpecker-ci/autoscaler/engine"
	"github.com/woodpecker-ci/woodpecker/woodpecker-go/woodpecker"

	xerrors "github.com/pkg/errors"
)

var (
	ErrIllegalLablePrefix = errors.New("illegal label prefix")
	ErrCreateServer       = errors.New("failed to create server")
)

var optionUserDataDefault = `
#cloud-config

apt_reboot_if_required: false
package_update: false
package_upgrade: false

groups:
  - docker

system_info:
  default_user:
    groups: [ docker ]

apt:
  sources:
    docker.list:
      source: deb [arch=amd64] https://download.docker.com/linux/ubuntu $RELEASE stable
      keyid: 0EBFCD88

packages:
  - docker-ce
  - docker-compose-plugin

write_files:
- path: /root/docker-compose.yml
  content: |
    # docker-compose.yml
    version: '3'
    services:
      woodpecker-agent:
        image: {{ .Image }}
        restart: always
        volumes:
          - /var/run/docker.sock:/var/run/docker.sock
        environment:
          {{- range $key, $value := .Environment }}
          - {{ $key }}={{ $value }}
          {{- end }}

runcmd:
  - sh -xc "cd /root; docker compose up -d"

final_message: "The system is finally up, after $UPTIME seconds"
`

type Driver struct {
	APIToken    string
	ServerType  string
	UserData    *template.Template
	Image       string
	SSHKeyID    int
	Labels      map[string]string
	LabelPrefix string
	Config      *config.Config
	Location    string
	Name        string
	client      *hcloud.Client
}

func New(c *cli.Context, config *config.Config) (engine.Provider, error) {
	d := &Driver{
		APIToken:    c.String("hetznercloud-api-token"),
		Location:    c.String("hetznercloud-location"),
		ServerType:  c.String("hetznercloud-server-type"),
		Image:       c.String("hetznercloud-image"),
		SSHKeyID:    c.Int("hetznercloud-ssh-key-id"),
		LabelPrefix: "wp.scaler/",
		Config:      config,
	}

	d.client = hcloud.NewClient(hcloud.WithToken(d.APIToken))

	if userdata := c.String("hetznercloud-user-data"); userdata != "" {
		optionUserDataDefault = userdata
	}

	userdata, err := template.New("user-data").Parse(optionUserDataDefault)
	if err != nil {
		return nil, xerrors.Wrap(err, "")
	}

	d.UserData = userdata

	labels := engine.SliceToMap(c.StringSlice("hetznercloud-labels"), "=")
	for _, key := range maps.Keys(labels) {
		if strings.HasPrefix(key, d.LabelPrefix) {
			return nil, fmt.Errorf("%w: %s", ErrIllegalLablePrefix, d.LabelPrefix)
		}
	}

	d.Labels = labels

	return d, nil
}

func (d *Driver) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	sshKeys := []*hcloud.SSHKey{}

	if d.SSHKeyID > 0 {
		sshKeys = append(sshKeys, &hcloud.SSHKey{
			ID: d.SSHKeyID,
		})
	}

	defaultLabels := make(map[string]string, 0)
	defaultLabels[fmt.Sprintf("%spool", d.LabelPrefix)] = d.Config.PoolID
	defaultLabels[fmt.Sprintf("%simage", d.LabelPrefix)] = d.Image
	labels := engine.MergeMaps(defaultLabels, d.Labels)

	userdataString, err := engine.RenderUserDataTemplate(d.Config, agent, d.UserData)
	if err != nil {
		return xerrors.Wrap(err, "")
	}

	image, _, err := d.client.Image.GetByName(ctx, d.Image)
	if err != nil {
		return xerrors.Wrap(err, "")
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
		Labels:  labels,
	})

	return xerrors.Wrap(err, "")
}

func (d *Driver) getAgent(ctx context.Context, agent *woodpecker.Agent) (*hcloud.Server, error) {
	server, _, err := d.client.Server.GetByName(ctx, agent.Name)
	return server, xerrors.Wrap(err, "")
}

func (d *Driver) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	server, err := d.getAgent(ctx, agent)
	if err != nil {
		return xerrors.Wrap(err, "")
	}

	if server == nil {
		return nil
	}

	_, _, err = d.client.Server.DeleteWithResult(ctx, server)
	return xerrors.Wrap(err, "")
}

func (d *Driver) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	servers, err := d.client.Server.All(ctx)
	if err != nil {
		return nil, xerrors.Wrap(err, "")
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
