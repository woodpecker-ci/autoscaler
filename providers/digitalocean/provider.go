package digitalocean

import (
	"context"
	b64 "encoding/base64"
	"fmt"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/digitalocean/godo"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	"golang.org/x/oauth2"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type Provider struct {
	name             string
	config           *config.Config
	instanceType     string
	image            string
	tags             []string
	region           string
	sshKeys          []string
	client           *godo.Client
	lock             sync.Mutex
	userDataTemplate *template.Template
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (engine.Provider, error) {
	token := c.String("digitalocean-token")
	if token == "" {
		return nil, fmt.Errorf("digitalocean-token must be set")
	}

	if c.String("digitalocean-region") == "" {
		return nil, fmt.Errorf("digitalocean-region must be set")
	}

	p := &Provider{
		name:         "digitalocean",
		config:       config,
		instanceType: c.String("digitalocean-instance-type"),
		image:        c.String("digitalocean-image"),
		tags:         c.StringSlice("digitalocean-tags"),
		region:       c.String("digitalocean-region"),
		sshKeys:      c.StringSlice("digitalocean-ssh-keys"),
	}

	// Setup DigitalOcean client
	oauthClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	))
	p.client = godo.NewClient(oauthClient)

	// User data template
	if u := c.String("provider-user-data"); u != "" {
		userDataTmpl, err := template.New("user-data").Parse(u)
		if err != nil {
			return nil, fmt.Errorf("%s: template.New.Parse %w", p.name, err)
		}
		p.userDataTemplate = userDataTmpl
	}

	return p, nil
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := engine.RenderUserDataTemplate(p.config, agent, p.userDataTemplate)
	if err != nil {
		return fmt.Errorf("%s: engine.RenderUserDataTemplate: %w", p.name, err)
	}

	// Generate tags
	tags := []string{
		agent.Name,
		engine.LabelPool,
		p.config.PoolID,
	}

	// Append user specified tags
	for _, tag := range p.tags {
		tags = append(tags, strings.TrimSpace(tag))
	}

	// Create droplet request
	createRequest := &godo.DropletCreateRequest{
		Name:   agent.Name,
		Region: p.region,
		Size:   p.instanceType,
		Image: godo.DropletCreateImage{
			Slug: p.image,
		},
		Tags:       tags,
		UserData:   b64.StdEncoding.EncodeToString([]byte(userData)),
		SSHKeys:    p.sshKeys,
		Monitoring: true,
	}

	droplet, _, err := p.client.Droplets.Create(ctx, createRequest)
	if err != nil {
		return fmt.Errorf("%s: Droplets.Create: %w", p.name, err)
	}

	log.Debug().Msgf("created droplet %d (%s)", droplet.ID, droplet.Name)

	// Wait until droplet is active
	log.Debug().Msgf("waiting for droplet %d to become active", droplet.ID)
	for range 30 { // Max 30 seconds
		d, _, err := p.client.Droplets.Get(ctx, droplet.ID)
		if err != nil {
			return fmt.Errorf("%s: Droplets.Get: %w", p.name, err)
		}

		if d.Status == "active" {
			return nil
		}

		log.Debug().Msgf("droplet status: %s", d.Status)
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("droplet %d did not become active in time", droplet.ID)
}

func (p *Provider) getDropletByName(ctx context.Context, name string) (*godo.Droplet, error) {
	// List all droplets and find by name
	droplets, _, err := p.client.Droplets.List(ctx, &godo.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, d := range droplets {
		if d.Name == name {
			return &d, nil
		}
	}

	return nil, fmt.Errorf("droplet with name %s not found", name)
}

func (p *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	droplet, err := p.getDropletByName(ctx, agent.Name)
	if err != nil {
		return err
	}

	_, err = p.client.Droplets.Delete(ctx, droplet.ID)
	if err != nil {
		return fmt.Errorf("%s: Droplets.Delete: %w", p.name, err)
	}

	log.Debug().Msgf("deleted droplet %d (%s)", droplet.ID, agent.Name)
	return nil
}

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	log.Debug().Msgf("list deployed agent names")

	var names []string

	droplets, _, err := p.client.Droplets.List(ctx, &godo.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("%s: Droplets.List: %w", p.name, err)
	}

	for _, droplet := range droplets {
		// Check if droplet has the pool tag
		for _, tag := range droplet.Tags {
			if tag == p.config.PoolID {
				// Check if droplet is active or provisioning
				if droplet.Status == "active" || droplet.Status == "new" || droplet.Status == "provisioning" {
					log.Debug().Msgf("found agent %s (status: %s)", droplet.Name, droplet.Status)
					names = append(names, droplet.Name)
				}
				break
			}
		}
	}

	return names, nil
}
