package hetznercloud

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/hetznercloud/hcloud-go/hcloud"
	"github.com/urfave/cli/v2"
	"golang.org/x/exp/maps"

	"github.com/woodpecker-ci/autoscaler/config"
	"github.com/woodpecker-ci/autoscaler/engine"
	"github.com/woodpecker-ci/woodpecker/woodpecker-go/woodpecker"
)

var (
	ErrIllegalLablePrefix = errors.New("illegal label prefix")
	ErrImageNotFound      = errors.New("image not found")
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
	APIToken      string
	ServerType    string
	UserData      *template.Template
	Image         string
	SSHKeyID      int
	LabelPrefix   string
	LabelPool     string
	LabelImage    string
	DefaultLabels map[string]string
	Labels        map[string]string
	Config        *config.Config
	Location      string
	Name          string
	client        *hcloud.Client
}

func New(c *cli.Context, config *config.Config, name string) (engine.Provider, error) {
	d := &Driver{
		Name:        name,
		APIToken:    c.String("hetznercloud-api-token"),
		Location:    c.String("hetznercloud-location"),
		ServerType:  c.String("hetznercloud-server-type"),
		Image:       c.String("hetznercloud-image"),
		SSHKeyID:    c.Int("hetznercloud-ssh-key-id"),
		LabelPrefix: "wp.scaler/",
		Config:      config,
	}

	d.LabelPool = fmt.Sprintf("%spool", d.LabelPrefix)
	d.LabelImage = fmt.Sprintf("%simage", d.LabelPrefix)

	d.DefaultLabels = make(map[string]string, 0)
	d.DefaultLabels[d.LabelPool] = d.Config.PoolID
	d.DefaultLabels[d.LabelImage] = d.Image

	d.client = hcloud.NewClient(hcloud.WithToken(d.APIToken))

	if userdata := c.String("hetznercloud-user-data"); userdata != "" {
		optionUserDataDefault = userdata
	}

	userdata, err := template.New("user-data").Parse(optionUserDataDefault)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.Name, err)
	}

	d.UserData = userdata

	labels := engine.SliceToMap(c.StringSlice("hetznercloud-labels"), "=")
	for _, key := range maps.Keys(labels) {
		if strings.HasPrefix(key, d.LabelPrefix) {
			return nil, fmt.Errorf("%s: %w: %s", d.Name, ErrIllegalLablePrefix, d.LabelPrefix)
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

	labels := engine.MergeMaps(d.DefaultLabels, d.Labels)

	userdataString, err := engine.RenderUserDataTemplate(d.Config, agent, d.UserData)
	if err != nil {
		return fmt.Errorf("%s: %w", d.Name, err)
	}

	image, _, err := d.client.Image.GetByName(ctx, d.Image)
	if err != nil {
		return fmt.Errorf("%s: %w", d.Image, err)
	}
	if image == nil {
		return fmt.Errorf("%s: %w: %s", d.Name, ErrImageNotFound, d.Image)
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
	if err != nil {
		return fmt.Errorf("%s: %w", d.Name, err)
	}

	return nil
}

func (d *Driver) getAgent(ctx context.Context, agent *woodpecker.Agent) (*hcloud.Server, error) {
	server, _, err := d.client.Server.GetByName(ctx, agent.Name)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.Name, err)
	}

	return server, nil
}

func (d *Driver) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	server, err := d.getAgent(ctx, agent)
	if err != nil {
		return fmt.Errorf("%s: %w", d.Name, err)
	}

	if server == nil {
		return nil
	}

	_, _, err = d.client.Server.DeleteWithResult(ctx, server)
	if err != nil {
		return fmt.Errorf("%s: %w", d.Name, err)
	}

	return nil
}

func (d *Driver) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	var names []string

	servers, err := d.client.Server.AllWithOpts(ctx,
		hcloud.ServerListOpts{
			ListOpts: hcloud.ListOpts{LabelSelector: fmt.Sprintf("%s==%s", d.LabelPool, d.Config.PoolID)},
		})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.Name, err)
	}

	for _, server := range servers {
		names = append(names, server.Name)
	}

	return names, nil
}
