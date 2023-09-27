package digitalocean

import (
	"context"
	"fmt"
	"text/template"

	"github.com/digitalocean/godo"
	"github.com/urfave/cli/v2"

	"github.com/woodpecker-ci/autoscaler/config"
	"github.com/woodpecker-ci/autoscaler/engine"
	"github.com/woodpecker-ci/woodpecker/woodpecker-go/woodpecker"
)

var (
// ErrIllegalLablePrefix = errors.New("illegal label prefix")
// ErrImageNotFound      = errors.New("image not found")
// ErrSSHKeyNotFound     = errors.New("SSH key not found")
// ErrNetworkNotFound    = errors.New("network not found")
// ErrFirewallNotFound   = errors.New("firewall not found")
)

type Provider struct {
	name        string
	dropletSize string
	userData    *template.Template
	image       string
	sshKeys     []string
	labels      map[string]string
	config      *config.Config
	region      string
	firewall    string
	enableIPv6  bool
	client      *godo.Client
}

func New(c *cli.Context, config *config.Config) (engine.Provider, error) {
	d := &Provider{
		name:        "digitalocean",
		region:      c.String("digitalocean-region"),
		dropletSize: c.String("digitalocean-droplet-size"),
		image:       c.String("hetznercloud-image"),
		sshKeys:     c.StringSlice("hetznercloud-ssh-keys"),
		enableIPv6:  c.Bool("hetznercloud-public-ipv6-enable"),
		config:      config,
	}

	d.client = godo.NewFromToken(c.String("digitalocean-api-token"))

	return d, nil
}

func (d *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userdataString, err := engine.RenderUserDataTemplate(d.config, agent, d.userData)
	if err != nil {
		return fmt.Errorf("%s: RenderUserDataTemplate: %w", d.name, err)
	}

	droplet, _, err := d.client.Droplets.Create(ctx, &godo.DropletCreateRequest{
		Name:     agent.Name,
		UserData: userdataString,
		Image: godo.DropletCreateImage{
			Slug: d.image,
		},
		Region:  d.region,
		Size:    d.dropletSize,
		IPv6:    d.enableIPv6,
		Backups: false,
		Tags:    engine.MapToSlice(d.labels, "="),
		SSHKeys: []godo.DropletCreateSSHKey{
			{Fingerprint: d.sshKeys[0]}, // TODO: support multiple SSH keys
		},
	})
	if err != nil {
		return fmt.Errorf("%s: Droplets.Create: %w", d.name, err)
	}

	// TODO: support firewalls
	if d.firewall != "" {
		_, err := d.client.Firewalls.AddDroplets(ctx, d.firewall, droplet.ID)
		if err != nil {
			return fmt.Errorf("%s: Firewalls.AddDroplets: %w", d.name, err)
		}
	}

	return nil
}

func (d *Provider) getDroplet(ctx context.Context, agent *woodpecker.Agent) (*godo.Droplet, error) {
	droplet, _, err := d.client.Droplets.ListByName(ctx, agent.Name, &godo.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.name, err)
	}

	if len(droplet) == 0 {
		return nil, nil
	}

	if len(droplet) > 1 {
		return nil, fmt.Errorf("%s: getDroplet: multiple droplets found for %s", d.name, agent.Name)
	}

	return &droplet[0], nil
}

func (d *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	droplet, err := d.getDroplet(ctx, agent)
	if err != nil {
		return fmt.Errorf("%s: getDroplet %w", d.name, err)
	}

	if droplet == nil {
		return nil
	}

	_, err = d.client.Droplets.Delete(ctx, droplet.ID)
	if err != nil {
		return fmt.Errorf("%s: Droplets.Delete %w", d.name, err)
	}

	return nil
}

func (d *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	var names []string

	droplets, _, err := d.client.Droplets.ListByTag(ctx,
		fmt.Sprintf("%s=%s", engine.LabelPool, d.config.PoolID), &godo.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.name, err)
	}

	for _, droplet := range droplets {
		names = append(names, droplet.Name)
	}

	return names, nil
}
