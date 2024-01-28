package hetznercloud

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/urfave/cli/v2"
	"golang.org/x/exp/maps"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v2/woodpecker-go/woodpecker"
)

var (
	ErrIllegalLablePrefix = errors.New("illegal label prefix")
	ErrImageNotFound      = errors.New("image not found")
	ErrSSHKeyNotFound     = errors.New("SSH key not found")
	ErrNetworkNotFound    = errors.New("network not found")
	ErrFirewallNotFound   = errors.New("firewall not found")
)

type Provider struct {
	name       string
	serverType string
	userData   *template.Template
	image      string
	sshKeys    []string
	labels     map[string]string
	config     *config.Config
	location   string
	networks   []string
	firewalls  []string
	enableIPv4 bool
	enableIPv6 bool
	client     *hcloud.Client
}

func New(c *cli.Context, config *config.Config) (engine.Provider, error) {
	d := &Provider{
		name:       "hetznercloud",
		location:   c.String("hetznercloud-location"),
		serverType: c.String("hetznercloud-server-type"),
		image:      c.String("hetznercloud-image"),
		sshKeys:    c.StringSlice("hetznercloud-ssh-keys"),
		firewalls:  c.StringSlice("hetznercloud-firewalls"),
		networks:   c.StringSlice("hetznercloud-networks"),
		enableIPv4: c.Bool("hetznercloud-public-ipv4-enable"),
		enableIPv6: c.Bool("hetznercloud-public-ipv6-enable"),
		config:     config,
	}

	d.client = hcloud.NewClient(hcloud.WithToken(c.String("hetznercloud-api-token")))

	userDataStr := engine.CloudInitUserDataUbuntuDefault
	if _userDataStr := c.String("hetznercloud-user-data"); _userDataStr != "" {
		userDataStr = _userDataStr
	}
	userDataTmpl, err := template.New("user-data").Parse(userDataStr)
	if err != nil {
		return nil, fmt.Errorf("%s: template.New.Parse %w", d.name, err)
	}
	d.userData = userDataTmpl

	defaultLabels := make(map[string]string, 0)
	defaultLabels[engine.LabelPool] = d.config.PoolID
	defaultLabels[engine.LabelImage] = d.image

	labels, err := engine.SliceToMap(c.StringSlice("hetznercloud-labels"), "=")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.name, err)
	}

	for _, key := range maps.Keys(labels) {
		if strings.HasPrefix(key, engine.LabelPrefix) {
			return nil, fmt.Errorf("%s: %w: %s", d.name, ErrIllegalLablePrefix, engine.LabelPrefix)
		}
	}
	d.labels = engine.MergeMaps(defaultLabels, d.labels)

	return d, nil
}

func (d *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userdataString, err := engine.RenderUserDataTemplate(d.config, agent, d.userData)
	if err != nil {
		return fmt.Errorf("%s: RenderUserDataTemplate: %w", d.name, err)
	}

	serverType, _, err := d.client.ServerType.GetByName(ctx, d.serverType)
	if err != nil {
		return fmt.Errorf("%s: ServerType.GetByName: %w", d.name, err)
	}

	image, _, err := d.client.Image.GetByNameAndArchitecture(ctx, d.image, serverType.Architecture)
	if err != nil {
		return fmt.Errorf("%s: Image.GetByNameAndArchitecture: %w", d.name, err)
	}
	if image == nil {
		return fmt.Errorf("%s: %w: %s", d.name, ErrImageNotFound, d.image)
	}

	sshKeys := make([]*hcloud.SSHKey, 0)
	for _, item := range d.sshKeys {
		key, _, err := d.client.SSHKey.GetByName(ctx, item)
		if err != nil {
			return fmt.Errorf("%s: SSHKey.GetByName: %w", d.name, err)
		}
		if key == nil {
			return fmt.Errorf("%s: %w: %s", d.name, ErrSSHKeyNotFound, item)
		}
		sshKeys = append(sshKeys, key)
	}

	networks := make([]*hcloud.Network, 0)
	for _, item := range d.networks {
		network, _, err := d.client.Network.GetByName(ctx, item)
		if err != nil {
			return fmt.Errorf("%s: Network.GetByName: %w", d.name, err)
		}
		if network == nil {
			return fmt.Errorf("%s: %w: %s", d.name, ErrNetworkNotFound, item)
		}
		networks = append(networks, network)
	}

	firewalls := make([]*hcloud.ServerCreateFirewall, 0)
	for _, item := range d.firewalls {
		fw, _, err := d.client.Firewall.GetByName(ctx, item)
		if err != nil {
			return fmt.Errorf("%s: Firewall.GetByName: %w", d.name, err)
		}
		if fw == nil {
			return fmt.Errorf("%s: %w: %s", d.name, ErrFirewallNotFound, item)
		}
		firewalls = append(firewalls, &hcloud.ServerCreateFirewall{Firewall: hcloud.Firewall{ID: fw.ID}})
	}

	_, _, err = d.client.Server.Create(ctx, hcloud.ServerCreateOpts{
		Name:     agent.Name,
		UserData: userdataString,
		Image:    image,
		Location: &hcloud.Location{
			Name: d.location,
		},
		ServerType: serverType,
		SSHKeys:    sshKeys,
		Networks:   networks,
		Firewalls:  firewalls,
		Labels:     d.labels,
		PublicNet: &hcloud.ServerCreatePublicNet{
			EnableIPv4: d.enableIPv4,
			EnableIPv6: d.enableIPv6,
		},
	})
	if err != nil {
		return fmt.Errorf("%s: Server.Create: %w", d.name, err)
	}

	return nil
}

func (d *Provider) getAgent(ctx context.Context, agent *woodpecker.Agent) (*hcloud.Server, error) {
	server, _, err := d.client.Server.GetByName(ctx, agent.Name)
	if err != nil {
		return nil, fmt.Errorf("%s: Server.GetByName %w", d.name, err)
	}

	return server, nil
}

func (d *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	server, err := d.getAgent(ctx, agent)
	if err != nil {
		return fmt.Errorf("%s: getAgent %w", d.name, err)
	}

	if server == nil {
		return nil
	}

	_, _, err = d.client.Server.DeleteWithResult(ctx, server)
	if err != nil {
		return fmt.Errorf("%s: Server.DeleteWithResults %w", d.name, err)
	}

	return nil
}

func (d *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	var names []string

	servers, err := d.client.Server.AllWithOpts(ctx,
		hcloud.ServerListOpts{
			ListOpts: hcloud.ListOpts{LabelSelector: fmt.Sprintf("%s==%s", engine.LabelPool, d.config.PoolID)},
		})
	if err != nil {
		return nil, fmt.Errorf("%s: Server.AllWithOpts %w", d.name, err)
	}

	for _, server := range servers {
		names = append(names, server.Name)
	}

	return names, nil
}
