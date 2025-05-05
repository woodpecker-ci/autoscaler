package hetznercloud

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	"golang.org/x/exp/maps"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/autoscaler/providers/hetznercloud/hcapi"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var (
	ErrIllegalLablePrefix = errors.New("illegal label prefix")
	ErrImageNotFound      = errors.New("image not found")
	ErrSSHKeyNotFound     = errors.New("SSH key not found")
	ErrNetworkNotFound    = errors.New("network not found")
	ErrFirewallNotFound   = errors.New("firewall not found")
	ErrServerTypeNotFound = errors.New("server type not found")
)

type Provider struct {
	name       string
	serverType []string
	// TODO: Deprecated remove in v2.0
	location         string
	userDataTemplate *template.Template
	image            string
	sshKeys          []string
	labels           map[string]string
	config           *config.Config
	networks         []string
	firewalls        []string
	enableIPv4       bool
	enableIPv6       bool
	client           hcapi.Client
}

func New(_ context.Context, c *cli.Command, config *config.Config) (engine.Provider, error) {
	p := &Provider{
		name:       "hetznercloud",
		serverType: c.StringSlice("hetznercloud-server-type"),
		// TODO: Deprecated remove in v2.0
		location:   c.String("hetznercloud-location"),
		image:      c.String("hetznercloud-image"),
		sshKeys:    c.StringSlice("hetznercloud-ssh-keys"),
		firewalls:  c.StringSlice("hetznercloud-firewalls"),
		networks:   c.StringSlice("hetznercloud-networks"),
		enableIPv4: c.Bool("hetznercloud-public-ipv4-enable"),
		enableIPv6: c.Bool("hetznercloud-public-ipv6-enable"),
		config:     config,
	}

	p.client = hcapi.NewClient(hcloud.WithToken(c.String("hetznercloud-api-token")))

	// # TODO: Deprecated remove in v2.0
	if u := c.String("hetznercloud-user-data"); u != "" {
		log.Warn().Msg("hetznercloud-user-data is deprecated, please use provider-user-data instead")
		userDataTmpl, err := template.New("user-data").Parse(u)
		if err != nil {
			return nil, fmt.Errorf("%s: template.New.Parse %w", p.name, err)
		}
		p.userDataTemplate = userDataTmpl
	}

	defaultLabels := make(map[string]string, 0)
	defaultLabels[engine.LabelPool] = p.config.PoolID
	defaultLabels[engine.LabelImage] = p.image

	labels, err := engine.SliceToMap(c.StringSlice("hetznercloud-labels"), "=")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}

	for _, key := range maps.Keys(labels) {
		if strings.HasPrefix(key, engine.LabelPrefix) {
			return nil, fmt.Errorf("%s: %w: %s", p.name, ErrIllegalLablePrefix, engine.LabelPrefix)
		}
	}
	p.labels = engine.MergeMaps(defaultLabels, p.labels)

	return p, nil
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := engine.RenderUserDataTemplate(p.config, agent, p.userDataTemplate)
	if err != nil {
		return fmt.Errorf("%s: engine.RenderUserDataTemplate: %w", p.name, err)
	}

	sshKeys := make([]*hcloud.SSHKey, 0)
	for _, item := range p.sshKeys {
		key, _, err := p.client.SSHKey().GetByName(ctx, item)
		if err != nil {
			return fmt.Errorf("%s: SSHKey.GetByName: %w", p.name, err)
		}
		if key == nil {
			return fmt.Errorf("%s: %w: %s", p.name, ErrSSHKeyNotFound, item)
		}
		sshKeys = append(sshKeys, key)
	}

	networks := make([]*hcloud.Network, 0)
	for _, item := range p.networks {
		network, _, err := p.client.Network().GetByName(ctx, item)
		if err != nil {
			return fmt.Errorf("%s: Network.GetByName: %w", p.name, err)
		}
		if network == nil {
			return fmt.Errorf("%s: %w: %s", p.name, ErrNetworkNotFound, item)
		}
		networks = append(networks, network)
	}

	firewalls := make([]*hcloud.ServerCreateFirewall, 0)
	for _, item := range p.firewalls {
		fw, _, err := p.client.Firewall().GetByName(ctx, item)
		if err != nil {
			return fmt.Errorf("%s: Firewall.GetByName: %w", p.name, err)
		}
		if fw == nil {
			return fmt.Errorf("%s: %w: %s", p.name, ErrFirewallNotFound, item)
		}
		firewalls = append(firewalls, &hcloud.ServerCreateFirewall{Firewall: hcloud.Firewall{ID: fw.ID}})
	}

	serverCreateOpts := hcloud.ServerCreateOpts{
		Name:      agent.Name,
		UserData:  userData,
		SSHKeys:   sshKeys,
		Networks:  networks,
		Firewalls: firewalls,
		Labels:    p.labels,
		PublicNet: &hcloud.ServerCreatePublicNet{
			EnableIPv4: p.enableIPv4,
			EnableIPv6: p.enableIPv6,
		},
	}

	for _, raw := range p.serverType {
		rawType, location, _ := strings.Cut(raw, ":")

		// TODO: Deprecated remove in v2.0
		if location == "" {
			log.Warn().Msg("hetznercloud-location is deprecated, please use hetznercloud-server-type instead")
			location = p.location
		}

		serverType, err := p.LookupServerType(ctx, rawType)
		if err != nil {
			return err
		}

		image, _, err := p.client.Image().GetByNameAndArchitecture(ctx, p.image, serverType.Architecture)
		if err != nil {
			return fmt.Errorf("%s: Image.GetByNameAndArchitecture: %w", p.name, err)
		}
		if image == nil {
			return fmt.Errorf("%s: %w: %s", p.name, ErrImageNotFound, p.image)
		}

		serverCreateOpts.Location = &hcloud.Location{Name: location}
		serverCreateOpts.ServerType = serverType
		serverCreateOpts.Image = image

		_, _, err = p.client.Server().Create(ctx, serverCreateOpts)
		if err == nil {
			return nil
		}

		// Continue to next fallback type if still unavailable
		if !hcloud.IsError(err, hcloud.ErrorCodeResourceUnavailable) {
			return fmt.Errorf("%s: Server.Create: %w", p.name, err)
		}
	}

	return nil
}

func (p *Provider) LookupServerType(ctx context.Context, name string) (*hcloud.ServerType, error) {
	serverType, _, err := p.client.ServerType().GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("%s: ServerType.GetByName: %w", name, err)
	}
	if serverType == nil {
		return nil, fmt.Errorf("%s: %w: %s", p.name, ErrServerTypeNotFound, name)
	}

	return serverType, nil
}

func (p *Provider) getAgent(ctx context.Context, agent *woodpecker.Agent) (*hcloud.Server, error) {
	server, _, err := p.client.Server().GetByName(ctx, agent.Name)
	if err != nil {
		return nil, fmt.Errorf("%s: Server.GetByName %w", p.name, err)
	}

	return server, nil
}

func (p *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	server, err := p.getAgent(ctx, agent)
	if err != nil {
		return fmt.Errorf("%s: getAgent %w", p.name, err)
	}

	if server == nil {
		return nil
	}

	_, _, err = p.client.Server().DeleteWithResult(ctx, server)
	if err != nil {
		return fmt.Errorf("%s: Server.DeleteWithResults %w", p.name, err)
	}

	return nil
}

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	var names []string

	servers, err := p.client.Server().AllWithOpts(ctx,
		hcloud.ServerListOpts{
			ListOpts: hcloud.ListOpts{LabelSelector: fmt.Sprintf("%s==%s", engine.LabelPool, p.config.PoolID)},
		})
	if err != nil {
		return nil, fmt.Errorf("%s: Server.AllWithOpts %w", p.name, err)
	}

	for _, server := range servers {
		names = append(names, server.Name)
	}

	return names, nil
}
