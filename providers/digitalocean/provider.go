package digitalocean

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/digitalocean/godo"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var (
	ErrIllegalLabelPrefix = errors.New("illegal label prefix")
)

// DropletsService abstracts the godo.DropletsService for testability.
type DropletsService interface {
	Create(ctx context.Context, createRequest *godo.DropletCreateRequest) (*godo.Droplet, *godo.Response, error)
	Delete(ctx context.Context, dropletID int) (*godo.Response, error)
	List(ctx context.Context, opt *godo.ListOptions) ([]godo.Droplet, *godo.Response, error)
}

type Provider struct {
	name             string
	region           string
	size             string
	image            string
	sshKeys          []string
	tags             []string
	enableIPv6       bool
	vpcUUID          string
	userDataTemplate *template.Template
	config           *config.Config
	droplets         DropletsService
}

func New(_ context.Context, c *cli.Command, cfg *config.Config) (engine.Provider, error) {
	token := c.String("digitalocean-api-token")
	client := godo.NewFromToken(token)

	p := &Provider{
		name:       "digitalocean",
		region:     c.String("digitalocean-region"),
		size:       c.String("digitalocean-size"),
		image:      c.String("digitalocean-image"),
		sshKeys:    c.StringSlice("digitalocean-ssh-keys"),
		enableIPv6: c.Bool("digitalocean-ipv6"),
		vpcUUID:    c.String("digitalocean-vpc-uuid"),
		config:     cfg,
		droplets:   client.Droplets,
	}

	// TODO: Deprecated remove in v2.0
	if u := c.String("digitalocean-user-data"); u != "" {
		log.Warn().Msg("digitalocean-user-data is deprecated, please use provider-user-data instead")
		userDataTmpl, err := template.New("user-data").Parse(u)
		if err != nil {
			return nil, fmt.Errorf("%s: template.New.Parse %w", p.name, err)
		}
		p.userDataTemplate = userDataTmpl
	}

	// Build default tags (used for listing deployed agents)
	defaultTags := []string{
		fmt.Sprintf("%s=%s", engine.LabelPool, cfg.PoolID),
		fmt.Sprintf("%s=%s", engine.LabelImage, p.image),
	}

	userTags := c.StringSlice("digitalocean-tags")
	for _, tag := range userTags {
		key, _, _ := strings.Cut(tag, "=")
		if strings.HasPrefix(key, engine.LabelPrefix) {
			return nil, fmt.Errorf("%s: %w: %s", p.name, ErrIllegalLabelPrefix, engine.LabelPrefix)
		}
	}
	p.tags = append(defaultTags, userTags...)

	return p, nil
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := engine.RenderUserDataTemplate(p.config, agent, p.userDataTemplate)
	if err != nil {
		return fmt.Errorf("%s: engine.RenderUserDataTemplate: %w", p.name, err)
	}

	sshKeys := make([]godo.DropletCreateSSHKey, 0, len(p.sshKeys))
	for _, key := range p.sshKeys {
		sshKeys = append(sshKeys, godo.DropletCreateSSHKey{Fingerprint: key})
	}

	createReq := &godo.DropletCreateRequest{
		Name:     agent.Name,
		Region:   p.region,
		Size:     p.size,
		UserData: userData,
		SSHKeys:  sshKeys,
		Tags:     p.tags,
		IPv6:     p.enableIPv6,
		Image: godo.DropletCreateImage{
			Slug: p.image,
		},
	}

	if p.vpcUUID != "" {
		createReq.VPCUUID = p.vpcUUID
	}

	log.Info().Msgf("create agent: region = %s size = %s", p.region, p.size)

	_, _, err = p.droplets.Create(ctx, createReq)
	if err != nil {
		return fmt.Errorf("%s: Droplets.Create: %w", p.name, err)
	}

	return nil
}

func (p *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	droplet, err := p.findDropletByName(ctx, agent.Name)
	if err != nil {
		return fmt.Errorf("%s: findDropletByName: %w", p.name, err)
	}

	if droplet == nil {
		return nil
	}

	_, err = p.droplets.Delete(ctx, droplet.ID)
	if err != nil {
		return fmt.Errorf("%s: Droplets.Delete: %w", p.name, err)
	}

	return nil
}

func (p *Provider) findDropletByName(ctx context.Context, name string) (*godo.Droplet, error) {
	page := 1
	for {
		droplets, resp, err := p.droplets.List(ctx, &godo.ListOptions{
			Page:    page,
			PerPage: 200, //nolint:mnd
		})
		if err != nil {
			return nil, err
		}

		for i := range droplets {
			if droplets[i].Name == name {
				return &droplets[i], nil
			}
		}

		if resp == nil || resp.Links == nil || resp.Links.Pages == nil || resp.Links.Pages.Next == "" {
			break
		}
		page++
	}

	return nil, nil
}

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	var names []string

	page := 1
	for {
		droplets, resp, err := p.droplets.List(ctx, &godo.ListOptions{
			Page:    page,
			PerPage: 200, //nolint:mnd
		})
		if err != nil {
			return nil, fmt.Errorf("%s: Droplets.List: %w", p.name, err)
		}

		poolTag := fmt.Sprintf("%s=%s", engine.LabelPool, p.config.PoolID)
		for _, d := range droplets {
			for _, tag := range d.Tags {
				if tag == poolTag {
					names = append(names, d.Name)
					break
				}
			}
		}

		if resp == nil || resp.Links == nil || resp.Links.Pages == nil || resp.Links.Pages.Next == "" {
			break
		}
		page++
	}

	return names, nil
}

// Ensure Provider implements engine.Provider at compile time.
var _ engine.Provider = (*Provider)(nil)
