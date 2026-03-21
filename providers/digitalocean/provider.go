package digitalocean

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"text/template"

	"github.com/digitalocean/godo"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var (
	ErrSSHKeyNotFound = errors.New("SSH key not found")
)

type Provider struct {
	name             string
	region           string
	size             string
	image            string
	enableIPv6       bool
	sshKeys          []godo.DropletCreateSSHKey
	tags             []string
	userDataTemplate *template.Template
	config           *config.Config
	client           *godo.Client
}

func New(_ context.Context, c *cli.Command, config *config.Config) (engine.Provider, error) {
	p := &Provider{
		name:       "digitalocean",
		region:     c.String("digitalocean-region"),
		size:       c.String("digitalocean-size"),
		image:      c.String("digitalocean-image"),
		enableIPv6: c.Bool("digitalocean-ipv6"),
		config:     config,
	}

	p.client = godo.NewFromToken(c.String("digitalocean-api-token"))

	// parse SSH keys (can be IDs or fingerprints)
	for _, key := range c.StringSlice("digitalocean-ssh-keys") {
		if id, err := strconv.Atoi(key); err == nil {
			p.sshKeys = append(p.sshKeys, godo.DropletCreateSSHKey{ID: id})
		} else {
			p.sshKeys = append(p.sshKeys, godo.DropletCreateSSHKey{Fingerprint: key})
		}
	}

	// # TODO: Deprecated remove in v2.0
	if u := c.String("digitalocean-user-data"); u != "" {
		log.Warn().Msg("digitalocean-user-data is deprecated, please use provider-user-data instead")
		userDataTmpl, err := template.New("user-data").Parse(u)
		if err != nil {
			return nil, fmt.Errorf("%s: template.New.Parse %w", p.name, err)
		}
		p.userDataTemplate = userDataTmpl
	}

	// pool tag is always added to identify managed droplets
	p.tags = append([]string{engine.LabelPool + "=" + config.PoolID}, c.StringSlice("digitalocean-tags")...)

	return p, nil
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := engine.RenderUserDataTemplate(p.config, agent, p.userDataTemplate)
	if err != nil {
		return fmt.Errorf("%s: engine.RenderUserDataTemplate: %w", p.name, err)
	}

	createReq := &godo.DropletCreateRequest{
		Name:   agent.Name,
		Region: p.region,
		Size:   p.size,
		Image: godo.DropletCreateImage{
			Slug: p.image,
		},
		SSHKeys:  p.sshKeys,
		UserData: userData,
		Tags:     p.tags,
		IPv6:     p.enableIPv6,
	}

	_, _, err = p.client.Droplets.Create(ctx, createReq)
	if err != nil {
		return fmt.Errorf("%s: Droplets.Create: %w", p.name, err)
	}

	return nil
}

func (p *Provider) getAgent(ctx context.Context, agent *woodpecker.Agent) (*godo.Droplet, error) {
	// list droplets filtered by the pool tag
	droplets, err := p.listAllDroplets(ctx)
	if err != nil {
		return nil, err
	}

	for _, d := range droplets {
		if d.Name == agent.Name {
			return &d, nil
		}
	}

	return nil, nil
}

func (p *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	droplet, err := p.getAgent(ctx, agent)
	if err != nil {
		return fmt.Errorf("%s: getAgent: %w", p.name, err)
	}

	if droplet == nil {
		return nil
	}

	_, err = p.client.Droplets.Delete(ctx, droplet.ID)
	if err != nil {
		return fmt.Errorf("%s: Droplets.Delete: %w", p.name, err)
	}

	return nil
}

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	droplets, err := p.listAllDroplets(ctx)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(droplets))
	for _, d := range droplets {
		names = append(names, d.Name)
	}

	return names, nil
}

func (p *Provider) listAllDroplets(ctx context.Context) ([]godo.Droplet, error) {
	var allDroplets []godo.Droplet

	poolTag := engine.LabelPool + "=" + p.config.PoolID

	opt := &godo.ListOptions{
		Page:    1,
		PerPage: 200, //nolint:mnd
	}

	for {
		droplets, resp, err := p.client.Droplets.ListByTag(ctx, poolTag, opt)
		if err != nil {
			return nil, fmt.Errorf("%s: Droplets.ListByTag: %w", p.name, err)
		}

		allDroplets = append(allDroplets, droplets...)

		if resp.Links == nil || resp.Links.IsLastPage() {
			break
		}

		page, err := resp.Links.CurrentPage()
		if err != nil {
			return nil, fmt.Errorf("%s: CurrentPage: %w", p.name, err)
		}
		opt.Page = page + 1
	}

	return allDroplets, nil
}
