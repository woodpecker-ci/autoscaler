package digitalocean

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/digitalocean/godo"
	"github.com/urfave/cli/v2"
	"golang.org/x/exp/maps"

	"github.com/woodpecker-ci/autoscaler/config"
	"github.com/woodpecker-ci/autoscaler/engine"
	"github.com/woodpecker-ci/woodpecker/woodpecker-go/woodpecker"
)

var (
	ErrIllegalLablePrefix = errors.New("illegal label prefix")
	ErrImageNotFound      = errors.New("image not found")
	ErrSSHKeyNotFound     = errors.New("SSH key not found")
	ErrNetworkNotFound    = errors.New("network not found")
	ErrFirewallNotFound   = errors.New("firewall not found")
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
      source: deb https://download.docker.com/linux/ubuntu $RELEASE stable
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
	DropletSize   string
	UserData      *template.Template
	Image         string
	SSHKeys       []string
	LabelPrefix   string
	LabelPool     string
	LabelImage    string
	DefaultLabels map[string]string
	Labels        map[string]string
	Config        *config.Config
	Region        string
	Networks      []string
	Firewalls     []string
	EnableIPv6    bool
	Name          string
	client        *godo.Client
}

func New(c *cli.Context, config *config.Config, name string) (engine.Provider, error) {
	d := &Driver{
		Name:        name,
		APIToken:    c.String("hetznercloud-api-token"),
		Region:      c.String("hetznercloud-location"),
		DropletSize: c.String("hetznercloud-server-type"),
		Image:       c.String("hetznercloud-image"),
		SSHKeys:     c.StringSlice("hetznercloud-ssh-keys"),
		Firewalls:   c.StringSlice("hetznercloud-firewalls"),
		Networks:    c.StringSlice("hetznercloud-networks"),
		EnableIPv6:  c.Bool("hetznercloud-public-ipv6-enable"),
		LabelPrefix: "wp.autoscaler/",
		Config:      config,
	}

	d.LabelPool = fmt.Sprintf("%spool", d.LabelPrefix)
	d.LabelImage = fmt.Sprintf("%simage", d.LabelPrefix)

	d.DefaultLabels = make(map[string]string, 0)
	d.DefaultLabels[d.LabelPool] = d.Config.PoolID
	d.DefaultLabels[d.LabelImage] = d.Image

	d.client = godo.NewFromToken(d.APIToken)

	if userdata := c.String("hetznercloud-user-data"); userdata != "" {
		optionUserDataDefault = userdata
	}

	userdata, err := template.New("user-data").Parse(optionUserDataDefault)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.Name, err)
	}

	d.UserData = userdata

	labels, err := engine.SliceToMap(c.StringSlice("hetznercloud-labels"), "=")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.Name, err)
	}
	for _, key := range maps.Keys(labels) {
		if strings.HasPrefix(key, d.LabelPrefix) {
			return nil, fmt.Errorf("%s: %w: %s", d.Name, ErrIllegalLablePrefix, d.LabelPrefix)
		}
	}

	d.Labels = labels

	return d, nil
}

func (d *Driver) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	_labels := engine.MergeMaps(d.DefaultLabels, d.Labels)
	labels := make([]string, 0)
	for key, value := range _labels {
		labels = append(labels, fmt.Sprintf("%s=%s", key, value))
	}

	userdataString, err := engine.RenderUserDataTemplate(d.Config, agent, d.UserData)
	if err != nil {
		return fmt.Errorf("%s: RenderUserDataTemplate: %w", d.Name, err)
	}

	droplet, _, err := d.client.Droplets.Create(ctx, &godo.DropletCreateRequest{
		Name:     agent.Name,
		UserData: userdataString,
		Image: godo.DropletCreateImage{
			Slug: d.Image,
		},
		Region:  d.Region,
		Size:    d.DropletSize,
		IPv6:    d.EnableIPv6,
		Backups: false,
		Tags:    labels,
		SSHKeys: []godo.DropletCreateSSHKey{
			{Fingerprint: d.SSHKeys[0]}, // TODO: support multiple SSH keys
		},
	})
	if err != nil {
		return fmt.Errorf("%s: Droplets.Create: %w", d.Name, err)
	}

	if d.Firewall != "" {
		_, err := d.client.Firewalls.AddDroplets(ctx, d.firewall, droplet.ID)
		if err != nil {
			return fmt.Errorf("%s: Firewalls.AddDroplets: %w", d.Name, err)
		}
	}

	return nil
}

func (d *Driver) getDroplet(ctx context.Context, agent *woodpecker.Agent) (*godo.Droplet, error) {
	droplet, _, err := d.client.Droplets.ListByName(ctx, agent.Name, &godo.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.Name, err)
	}

	if len(droplet) == 0 {
		return nil, nil
	}

	if len(droplet) > 1 {
		return nil, fmt.Errorf("%s: getDroplet: multiple droplets found for %s", d.Name, agent.Name)
	}

	return &droplet[0], nil
}

func (d *Driver) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	droplet, err := d.getDroplet(ctx, agent)
	if err != nil {
		return fmt.Errorf("%s: %w", d.Name, err)
	}

	if droplet == nil {
		return nil
	}

	_, err = d.client.Droplets.Delete(ctx, droplet.ID)
	if err != nil {
		return fmt.Errorf("%s: %w", d.Name, err)
	}

	return nil
}

func (d *Driver) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	var names []string

	droplets, _, err := d.client.Droplets.ListByTag(ctx,
		fmt.Sprintf("%s=%s", d.LabelPool, d.Config.PoolID), &godo.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.Name, err)
	}

	for _, droplet := range droplets {
		names = append(names, droplet.Name)
	}

	return names, nil
}
