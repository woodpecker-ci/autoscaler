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
	"golang.org/x/exp/maps"
	"golang.org/x/oauth2"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var (
	ErrIllegalLabelPrefix = errors.New("illegal label prefix")
	ErrSSHKeyNotFound     = errors.New("SSH key not found")
)

type Provider struct {
	name             string
	region           string
	size             string
	image            string
	sshKeys          []godo.DropletCreateSSHKey
	tags             []string
	labels           map[string]string
	enableIPv6       bool
	enableMonitoring bool
	vpcUUID          string
	userDataTemplate *template.Template
	config           *config.Config
	client           *godo.Client
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (engine.Provider, error) {
	p := &Provider{
		name:             "digitalocean",
		region:           c.String("digitalocean-region"),
		size:             c.String("digitalocean-size"),
		image:            c.String("digitalocean-image"),
		enableIPv6:       c.Bool("digitalocean-ipv6"),
		enableMonitoring: c.Bool("digitalocean-monitoring"),
		vpcUUID:          c.String("digitalocean-vpc-uuid"),
		config:           config,
	}

	// Setup OAuth2 client
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: c.String("digitalocean-api-token")})
	oauthClient := oauth2.NewClient(ctx, tokenSource)
	p.client = godo.NewClient(oauthClient)

	// Setup SSH keys
	if err := p.setupSSHKeys(ctx, c.StringSlice("digitalocean-ssh-keys")); err != nil {
		return nil, fmt.Errorf("%s: setupSSHKeys: %w", p.name, err)
	}

	// Setup default labels/tags
	defaultLabels := make(map[string]string)
	defaultLabels[engine.LabelPool] = p.config.PoolID
	defaultLabels[engine.LabelImage] = p.image

	// Parse user-provided labels
	labels, err := engine.SliceToMap(c.StringSlice("digitalocean-tags"), "=")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}

	for _, key := range maps.Keys(labels) {
		if strings.HasPrefix(key, engine.LabelPrefix) {
			return nil, fmt.Errorf("%s: %w: %s", p.name, ErrIllegalLabelPrefix, engine.LabelPrefix)
		}
	}
	p.labels = engine.MergeMaps(defaultLabels, labels)

	// Convert labels to DigitalOcean tags (they don't support key=value, so we encode it)
	p.tags = make([]string, 0, len(p.labels))
	for key, value := range p.labels {
		// DigitalOcean tags have restrictions: lowercase, alphanumeric, hyphens, underscores, colons
		// We encode as key:value format
		tag := sanitizeTag(fmt.Sprintf("%s:%s", key, value))
		p.tags = append(p.tags, tag)
	}

	return p, nil
}

// sanitizeTag converts a string to a valid DigitalOcean tag
func sanitizeTag(s string) string {
	// Replace / with - and = with :
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "=", ":")
	// Lowercase
	s = strings.ToLower(s)
	return s
}

func (p *Provider) setupSSHKeys(ctx context.Context, keyNames []string) error {
	keys, _, err := p.client.Keys.List(ctx, &godo.ListOptions{PerPage: 200}) //nolint:mnd
	if err != nil {
		return err
	}

	// If specific keys requested, find them
	if len(keyNames) > 0 {
		for _, name := range keyNames {
			found := false
			for _, key := range keys {
				if key.Name == name || key.Fingerprint == name {
					p.sshKeys = append(p.sshKeys, godo.DropletCreateSSHKey{ID: key.ID})
					found = true
					break
				}
			}
			if !found {
				log.Warn().Msgf("SSH key not found: %s", name)
			}
		}
		if len(p.sshKeys) > 0 {
			return nil
		}
	}

	// Try to find keys by naming convention
	index := make(map[string]int)
	for _, key := range keys {
		index[key.Name] = key.ID
	}

	for _, name := range []string{"woodpecker", "id_rsa_woodpecker"} {
		if id, ok := index[name]; ok {
			p.sshKeys = append(p.sshKeys, godo.DropletCreateSSHKey{ID: id})
			return nil
		}
	}

	// Use first available key if any
	if len(keys) > 0 {
		p.sshKeys = append(p.sshKeys, godo.DropletCreateSSHKey{ID: keys[0].ID})
		return nil
	}

	return ErrSSHKeyNotFound
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := engine.RenderUserDataTemplate(p.config, agent, p.userDataTemplate)
	if err != nil {
		return fmt.Errorf("%s: engine.RenderUserDataTemplate: %w", p.name, err)
	}

	createRequest := &godo.DropletCreateRequest{
		Name:       agent.Name,
		Region:     p.region,
		Size:       p.size,
		Image:      godo.DropletCreateImage{Slug: p.image},
		SSHKeys:    p.sshKeys,
		IPv6:       p.enableIPv6,
		Monitoring: p.enableMonitoring,
		UserData:   userData,
		Tags:       p.tags,
	}

	if p.vpcUUID != "" {
		createRequest.VPCUUID = p.vpcUUID
	}

	_, _, err = p.client.Droplets.Create(ctx, createRequest)
	if err != nil {
		return fmt.Errorf("%s: Droplets.Create: %w", p.name, err)
	}

	return nil
}

func (p *Provider) getAgent(ctx context.Context, agent *woodpecker.Agent) (*godo.Droplet, error) {
	// List droplets and find by name
	droplets, _, err := p.client.Droplets.ListByName(ctx, agent.Name, &godo.ListOptions{PerPage: 200}) //nolint:mnd
	if err != nil {
		return nil, fmt.Errorf("%s: Droplets.ListByName: %w", p.name, err)
	}

	if len(droplets) == 0 {
		return nil, nil
	}

	return &droplets[0], nil
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
	var names []string
	var droplets []godo.Droplet

	// Create tag for filtering by pool
	poolTag := sanitizeTag(fmt.Sprintf("%s:%s", engine.LabelPool, p.config.PoolID))

	opt := &godo.ListOptions{PerPage: 200} //nolint:mnd
	for {
		page, resp, err := p.client.Droplets.ListByTag(ctx, poolTag, opt)
		if err != nil {
			return nil, fmt.Errorf("%s: Droplets.ListByTag: %w", p.name, err)
		}

		droplets = append(droplets, page...)

		if resp.Links == nil || resp.Links.IsLastPage() {
			break
		}

		nextPage, err := resp.Links.CurrentPage()
		if err != nil {
			return nil, fmt.Errorf("%s: parsing pagination: %w", p.name, err)
		}
		opt.Page = nextPage + 1
	}

	for _, droplet := range droplets {
		names = append(names, droplet.Name)
	}

	return names, nil
}
