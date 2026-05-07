package digitalocean

import (
	"context"
	"errors"
	"fmt"
	"text/template"

	"github.com/digitalocean/godo"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

const perPage = 200 //nolint:mnd

var ErrSSHKeyNotFound = errors.New("SSH key not found")

type dropletsService interface {
	Create(ctx context.Context, createRequest *godo.DropletCreateRequest) (*godo.Droplet, *godo.Response, error)
	Delete(ctx context.Context, dropletID int) (*godo.Response, error)
	ListByTag(ctx context.Context, tag string, opts *godo.ListOptions) ([]godo.Droplet, *godo.Response, error)
}

type keysService interface {
	List(ctx context.Context, opts *godo.ListOptions) ([]godo.Key, *godo.Response, error)
}

type Provider struct {
	name             string
	region           string
	size             string
	image            string
	sshKeyID         int
	tags             []string
	poolTag          string
	config           *config.Config
	userDataTemplate *template.Template
	droplets         dropletsService
	keys             keysService
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (types.Provider, error) {
	client := godo.NewFromToken(c.String("digitalocean-api-token"))

	p := &Provider{
		name:     "digitalocean",
		region:   c.String("digitalocean-region"),
		size:     c.String("digitalocean-size"),
		image:    c.String("digitalocean-image"),
		config:   config,
		poolTag:  "woodpecker-pool:" + config.PoolID,
		droplets: client.Droplets,
		keys:     client.Keys,
	}

	if err := p.setupKeypair(ctx, c.String("digitalocean-ssh-key")); err != nil {
		return nil, fmt.Errorf("%s: setupKeypair: %w", p.name, err)
	}

	p.tags = append(c.StringSlice("digitalocean-tags"), p.poolTag)

	return p, nil
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, p.userDataTemplate)
	if err != nil {
		return fmt.Errorf("%s: cloudinit.RenderUserDataTemplate: %w", p.name, err)
	}

	log.Debug().Msgf("%s: creating droplet %s in %s with size %s", p.name, agent.Name, p.region, p.size)

	_, _, err = p.droplets.Create(ctx, &godo.DropletCreateRequest{
		Name:     agent.Name,
		Region:   p.region,
		Size:     p.size,
		Image:    godo.DropletCreateImage{Slug: p.image},
		SSHKeys:  []godo.DropletCreateSSHKey{{ID: p.sshKeyID}},
		UserData: userData,
		Tags:     p.tags,
	})
	if err != nil {
		return fmt.Errorf("%s: Droplets.Create: %w", p.name, err)
	}

	return nil
}

func (p *Provider) getAgent(ctx context.Context, agent *woodpecker.Agent) (*godo.Droplet, error) {
	opts := &godo.ListOptions{PerPage: perPage}
	for {
		droplets, resp, err := p.droplets.ListByTag(ctx, p.poolTag, opts)
		if err != nil {
			return nil, fmt.Errorf("%s: Droplets.ListByTag: %w", p.name, err)
		}
		for i := range droplets {
			if droplets[i].Name == agent.Name {
				return &droplets[i], nil
			}
		}
		if resp == nil || resp.Links == nil || resp.Links.IsLastPage() {
			break
		}
		page, err := resp.Links.CurrentPage()
		if err != nil {
			return nil, fmt.Errorf("%s: Links.CurrentPage: %w", p.name, err)
		}
		opts.Page = page + 1
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

	log.Debug().Msgf("%s: deleting droplet %s (id=%d)", p.name, agent.Name, droplet.ID)

	if _, err = p.droplets.Delete(ctx, droplet.ID); err != nil {
		return fmt.Errorf("%s: Droplets.Delete: %w", p.name, err)
	}

	return nil
}

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	var names []string
	opts := &godo.ListOptions{PerPage: perPage}
	for {
		droplets, resp, err := p.droplets.ListByTag(ctx, p.poolTag, opts)
		if err != nil {
			return nil, fmt.Errorf("%s: Droplets.ListByTag: %w", p.name, err)
		}
		for _, d := range droplets {
			names = append(names, d.Name)
		}
		if resp == nil || resp.Links == nil || resp.Links.IsLastPage() {
			break
		}
		page, err := resp.Links.CurrentPage()
		if err != nil {
			return nil, fmt.Errorf("%s: Links.CurrentPage: %w", p.name, err)
		}
		opts.Page = page + 1
	}
	return names, nil
}

func (p *Provider) setupKeypair(ctx context.Context, preferredName string) error {
	index := make(map[string]int)
	opts := &godo.ListOptions{PerPage: perPage}
	for {
		keys, resp, err := p.keys.List(ctx, opts)
		if err != nil {
			return fmt.Errorf("%s: Keys.List: %w", p.name, err)
		}
		for _, key := range keys {
			index[key.Name] = key.ID
		}
		if resp == nil || resp.Links == nil || resp.Links.IsLastPage() {
			break
		}
		page, err := resp.Links.CurrentPage()
		if err != nil {
			return fmt.Errorf("%s: Links.CurrentPage: %w", p.name, err)
		}
		opts.Page = page + 1
	}

	candidates := []string{"woodpecker", "id_rsa_woodpecker"}
	if preferredName != "" {
		candidates = append([]string{preferredName}, candidates...)
	}

	for _, name := range candidates {
		if id, ok := index[name]; ok {
			p.sshKeyID = id
			return nil
		}
	}

	if len(index) > 0 {
		for _, id := range index {
			p.sshKeyID = id
			return nil
		}
	}

	return ErrSSHKeyNotFound
}
