// Copyright 2024 Woodpecker Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package digitalocean

import (
	"context"
	"errors"
	"fmt"
	"strconv"
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
	ErrSSHKeyNotFound = errors.New("SSH key not found")
)

type Provider struct {
	name             string
	region           string
	size             string
	image            string
	sshKey           string
	tags             []string
	vpcUUID          string
	ipv6             bool
	monitoring       bool
	firewallID       string
	config           *config.Config
	userDataTemplate *template.Template
	client           *godo.Client
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (engine.Provider, error) {
	p := &Provider{
		name:       "digitalocean",
		region:     c.String("digitalocean-region"),
		size:       c.String("digitalocean-size"),
		image:      c.String("digitalocean-image"),
		sshKey:     c.String("digitalocean-ssh-key"),
		tags:       c.StringSlice("digitalocean-tags"),
		vpcUUID:    c.String("digitalocean-vpc-uuid"),
		ipv6:       c.Bool("digitalocean-ipv6"),
		monitoring: c.Bool("digitalocean-monitoring"),
		firewallID: c.String("digitalocean-firewall-id"),
		config:     config,
	}

	p.client = godo.NewFromToken(c.String("digitalocean-api-token"))

	err := p.setupKeypair(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: setupKeypair: %w", p.name, err)
	}

	return p, nil
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := engine.RenderUserDataTemplate(p.config, agent, p.userDataTemplate)
	if err != nil {
		return fmt.Errorf("%s: engine.RenderUserDataTemplate: %w", p.name, err)
	}

	// Prepare SSH keys
	sshKeys := []godo.DropletCreateSSHKey{}
	if p.sshKey != "" {
		// Try to parse as ID first
		if id, err := strconv.Atoi(p.sshKey); err == nil {
			sshKeys = append(sshKeys, godo.DropletCreateSSHKey{ID: id})
		} else {
			// Treat as fingerprint
			sshKeys = append(sshKeys, godo.DropletCreateSSHKey{Fingerprint: p.sshKey})
		}
	}

	// Build tags list, always include woodpecker-agent tag for identification
	tags := append([]string{"woodpecker-agent"}, p.tags...)

	createRequest := &godo.DropletCreateRequest{
		Name:       agent.Name,
		Region:     p.region,
		Size:       p.size,
		Image:      godo.DropletCreateImage{Slug: p.image},
		SSHKeys:    sshKeys,
		Tags:       tags,
		IPv6:       p.ipv6,
		Monitoring: p.monitoring,
		UserData:   userData,
	}

	// Set VPC if specified
	if p.vpcUUID != "" {
		createRequest.VPCUUID = p.vpcUUID
	}

	droplet, _, err := p.client.Droplets.Create(ctx, createRequest)
	if err != nil {
		return fmt.Errorf("%s: Droplets.Create: %w", p.name, err)
	}

	log.Info().Str("agent", agent.Name).Int("droplet_id", droplet.ID).Msg("droplet created")

	// Apply firewall if specified
	if p.firewallID != "" {
		_, err = p.client.Firewalls.AddDroplets(ctx, p.firewallID, droplet.ID)
		if err != nil {
			log.Warn().Err(err).Str("firewall_id", p.firewallID).Msg("failed to add droplet to firewall")
		}
	}

	return nil
}

func (p *Provider) getDroplet(ctx context.Context, agent *woodpecker.Agent) (*godo.Droplet, error) {
	// List droplets with woodpecker-agent tag
	opt := &godo.ListOptions{
		Page:    1,
		PerPage: 200,
	}

	droplets, _, err := p.client.Droplets.ListByTag(ctx, "woodpecker-agent", opt)
	if err != nil {
		return nil, fmt.Errorf("%s: Droplets.ListByTag: %w", p.name, err)
	}

	for _, droplet := range droplets {
		if droplet.Name == agent.Name {
			return &droplet, nil
		}
	}

	return nil, nil
}

func (p *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	droplet, err := p.getDroplet(ctx, agent)
	if err != nil {
		return fmt.Errorf("%s: getDroplet: %w", p.name, err)
	}

	if droplet == nil {
		log.Debug().Str("agent", agent.Name).Msg("droplet not found, nothing to remove")
		return nil
	}

	_, err = p.client.Droplets.Delete(ctx, droplet.ID)
	if err != nil {
		return fmt.Errorf("%s: Droplets.Delete: %w", p.name, err)
	}

	log.Info().Str("agent", agent.Name).Int("droplet_id", droplet.ID).Msg("droplet deleted")

	return nil
}

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	var names []string

	opt := &godo.ListOptions{
		Page:    1,
		PerPage: 200,
	}

	// List all droplets with woodpecker-agent tag
	droplets, _, err := p.client.Droplets.ListByTag(ctx, "woodpecker-agent", opt)
	if err != nil {
		return nil, fmt.Errorf("%s: Droplets.ListByTag: %w", p.name, err)
	}

	for _, droplet := range droplets {
		// Only include droplets that match the agent naming pattern
		if strings.Contains(droplet.Name, "agent") {
			names = append(names, droplet.Name)
		}
	}

	return names, nil
}

func (p *Provider) setupKeypair(ctx context.Context) error {
	// If SSH key is already configured via flag, use it
	if p.sshKey != "" {
		return nil
	}

	// List all SSH keys
	opt := &godo.ListOptions{
		Page:    1,
		PerPage: 200,
	}

	keys, _, err := p.client.Keys.List(ctx, opt)
	if err != nil {
		return fmt.Errorf("Keys.List: %w", err)
	}

	// Try to find a key with common names
	for _, name := range []string{"woodpecker", "id_rsa_woodpecker", "id_ed25519"} {
		for _, key := range keys {
			if key.Name == name {
				p.sshKey = key.Fingerprint
				log.Info().Str("key", name).Msg("using SSH key")
				return nil
			}
		}
	}

	// Use the first available key
	if len(keys) > 0 {
		p.sshKey = keys[0].Fingerprint
		log.Info().Str("key", keys[0].Name).Msg("using first available SSH key")
		return nil
	}

	log.Warn().Msg("no SSH key found, droplets will be created without SSH access")
	return nil
}
