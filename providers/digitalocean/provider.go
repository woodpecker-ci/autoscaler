package digitalocean

import (
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/digitalocean/godo"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	"golang.org/x/oauth2"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

const (
	poolTagPrefix = "wp-pool-"
)

// Provider implements engine.Provider for DigitalOcean Droplets.
type Provider struct {
	name             string
	client           *godo.Client
	config           *config.Config
	region           string
	size             string
	image            string
	ipv6             bool
	sshKeys          []godo.DropletCreateSSHKey
	tags             []string
	userDataTemplate *template.Template
}

// New creates a new DigitalOcean provider from CLI flags.
func New(ctx context.Context, c *cli.Command, cfg *config.Config) (engine.Provider, error) {
	token := c.String("digitalocean-token")
	if token == "" {
		return nil, fmt.Errorf("digitalocean: API token is required (set --digitalocean-token or WOODPECKER_AUTOSCALER_DIGITALOCEAN_TOKEN)")
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	oauthClient := oauth2.NewClient(ctx, ts)
	client := godo.NewClient(oauthClient)

	p := &Provider{
		name:   "digitalocean",
		client: client,
		config: cfg,
		region: c.String("digitalocean-region"),
		size:   c.String("digitalocean-size"),
		image:  c.String("digitalocean-image"),
		ipv6:   c.Bool("digitalocean-ipv6"),
	}

	// Resolve SSH keys supplied as fingerprints or numeric IDs.
	for _, key := range c.StringSlice("digitalocean-ssh-keys") {
		p.sshKeys = append(p.sshKeys, godo.DropletCreateSSHKey{Fingerprint: key})
	}

	// Build the canonical pool tag that marks all Droplets belonging to this pool.
	poolTag := poolTagPrefix + cfg.PoolID
	p.tags = append(p.tags, poolTag)

	for _, tag := range c.StringSlice("digitalocean-tags") {
		p.tags = append(p.tags, tag)
	}

	return p, nil
}

// DeployAgent creates a new Droplet to run a Woodpecker agent.
func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := engine.RenderUserDataTemplate(p.config, agent, p.userDataTemplate)
	if err != nil {
		return fmt.Errorf("%s: engine.RenderUserDataTemplate: %w", p.name, err)
	}

	createReq := &godo.DropletCreateRequest{
		Name:     agent.Name,
		Region:   p.region,
		Size:     p.size,
		Image:    godo.DropletCreateImage{Slug: p.image},
		SSHKeys:  p.sshKeys,
		IPv6:     p.ipv6,
		Tags:     p.tags,
		UserData: userData,
	}

	droplet, _, err := p.client.Droplets.Create(ctx, createReq)
	if err != nil {
		return fmt.Errorf("%s: Droplets.Create: %w", p.name, err)
	}

	log.Debug().Msgf("%s: created droplet %d (%s)", p.name, droplet.ID, agent.Name)

	return nil
}

// RemoveAgent deletes the Droplet that corresponds to the given agent.
func (p *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	droplet, err := p.findDropletByName(ctx, agent.Name)
	if err != nil {
		return fmt.Errorf("%s: findDropletByName: %w", p.name, err)
	}

	if droplet == nil {
		log.Warn().Msgf("%s: RemoveAgent: no droplet found for agent %s", p.name, agent.Name)
		return nil
	}

	_, err = p.client.Droplets.Delete(ctx, droplet.ID)
	if err != nil {
		return fmt.Errorf("%s: Droplets.Delete: %w", p.name, err)
	}

	log.Debug().Msgf("%s: deleted droplet %d (%s)", p.name, droplet.ID, agent.Name)

	return nil
}

// ListDeployedAgentNames returns the names of all Droplets that belong to
// this pool (identified by the pool tag).
func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	poolTag := poolTagPrefix + p.config.PoolID

	var names []string

	opt := &godo.ListOptions{PerPage: 200} //nolint:mnd
	for {
		droplets, resp, err := p.client.Droplets.ListByTag(ctx, poolTag, opt)
		if err != nil {
			return nil, fmt.Errorf("%s: Droplets.ListByTag: %w", p.name, err)
		}

		for _, d := range droplets {
			names = append(names, d.Name)
		}

		if resp.Links == nil || resp.Links.IsLastPage() {
			break
		}

		page, err := resp.Links.CurrentPage()
		if err != nil {
			return nil, fmt.Errorf("%s: pagination: %w", p.name, err)
		}
		opt.Page = page + 1
	}

	return names, nil
}

// findDropletByName returns the first Droplet in this pool whose name matches,
// or nil if none is found.
func (p *Provider) findDropletByName(ctx context.Context, name string) (*godo.Droplet, error) {
	poolTag := poolTagPrefix + p.config.PoolID

	opt := &godo.ListOptions{PerPage: 200} //nolint:mnd
	for {
		droplets, resp, err := p.client.Droplets.ListByTag(ctx, poolTag, opt)
		if err != nil {
			return nil, fmt.Errorf("Droplets.ListByTag: %w", err)
		}

		for i := range droplets {
			if strings.EqualFold(droplets[i].Name, name) {
				return &droplets[i], nil
			}
		}

		if resp.Links == nil || resp.Links.IsLastPage() {
			break
		}

		page, err := resp.Links.CurrentPage()
		if err != nil {
			return nil, fmt.Errorf("pagination: %w", err)
		}
		opt.Page = page + 1
	}

	return nil, nil
}
