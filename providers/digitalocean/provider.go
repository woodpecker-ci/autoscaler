package digitalocean

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/digitalocean/godo"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/autoscaler/version"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

const (
	autoscalerTag = "woodpecker-autoscaler"
	maxTagLength  = 255
)

var ErrAPITokenNotSet = errors.New("no api token provided")

// blackhole metadata service so running steps can not extract agent token from user-data
var blackholeMetadataAPI = []string{
	"ip -4 route add blackhole 169.254.169.254/32",
}

type provider struct {
	name              string
	config            *config.Config
	client            *godo.Client
	region            string
	size              string
	image             string
	sshKeys           []godo.DropletCreateSSHKey
	tags              []string
	poolTag           string
	enableIPv6        bool
	privateNetworking bool
	monitoring        bool
	backups           bool
}

func New(_ context.Context, c *cli.Command, config *config.Config) (types.Provider, error) {
	apiToken := c.String("digitalocean-api-token")
	if apiToken == "" {
		return nil, ErrAPITokenNotSet
	}

	p := &provider{
		name:              "digitalocean",
		config:            config,
		client:            newClient(apiToken),
		region:            c.String("digitalocean-region"),
		size:              c.String("digitalocean-size"),
		image:             c.String("digitalocean-image"),
		sshKeys:           parseSSHKeys(c.StringSlice("digitalocean-ssh-keys")),
		poolTag:           poolTag(config.PoolID),
		enableIPv6:        c.Bool("digitalocean-public-ipv6-enable"),
		privateNetworking: c.Bool("digitalocean-private-networking-enable"),
		monitoring:        c.Bool("digitalocean-monitoring-enable"),
		backups:           c.Bool("digitalocean-backups-enable"),
	}

	p.tags = append([]string{autoscalerTag, p.poolTag}, c.StringSlice("digitalocean-tags")...)

	return p, nil
}

func (p *provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, nil, cloudinit.RenderOption{
		PreExec: blackholeMetadataAPI,
	})
	if err != nil {
		return fmt.Errorf("%s: cloudinit.RenderUserDataTemplate: %w", p.name, err)
	}

	_, _, err = p.client.Droplets.Create(ctx, &godo.DropletCreateRequest{
		Name:              agent.Name,
		Region:            p.region,
		Size:              p.size,
		Image:             godo.DropletCreateImage{Slug: p.image},
		SSHKeys:           p.sshKeys,
		Backups:           p.backups,
		IPv6:              p.enableIPv6,
		PrivateNetworking: p.privateNetworking,
		Monitoring:        p.monitoring,
		UserData:          userData,
		Tags:              p.tags,
	})
	if err != nil {
		return fmt.Errorf("%s: Droplets.Create: %w", p.name, err)
	}

	return nil
}

func (p *provider) getAgent(ctx context.Context, agent *woodpecker.Agent) (*godo.Droplet, error) {
	droplets, err := p.listDroplets(ctx)
	if err != nil {
		return nil, err
	}

	var found *godo.Droplet
	for i := range droplets {
		if droplets[i].Name != agent.Name {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("%s: getAgent: found multiple droplets with name %s", p.name, agent.Name)
		}
		found = &droplets[i]
	}

	return found, nil
}

func (p *provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	droplet, err := p.getAgent(ctx, agent)
	if err != nil {
		return fmt.Errorf("%s: getAgent %w", p.name, err)
	}

	if droplet == nil {
		return nil
	}

	_, err = p.client.Droplets.Delete(ctx, droplet.ID)
	if err != nil {
		return fmt.Errorf("%s: Droplets.Delete %w", p.name, err)
	}

	return nil
}

func (p *provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	var names []string

	droplets, err := p.listDroplets(ctx)
	if err != nil {
		return nil, err
	}

	for _, droplet := range droplets {
		names = append(names, droplet.Name)
	}

	return names, nil
}

func (p *provider) listDroplets(ctx context.Context) ([]godo.Droplet, error) {
	var droplets []godo.Droplet

	opts := &godo.ListOptions{
		PerPage: 200, //nolint:mnd
	}

	for {
		newDroplets, resp, err := p.client.Droplets.ListByTag(ctx, p.poolTag, opts)
		if err != nil {
			return nil, fmt.Errorf("%s: Droplets.ListByTag %w", p.name, err)
		}

		droplets = append(droplets, newDroplets...)

		if resp == nil || resp.Links == nil || resp.Links.IsLastPage() {
			break
		}

		currentPage, err := resp.Links.CurrentPage()
		if err != nil {
			return nil, fmt.Errorf("%s: Links.CurrentPage %w", p.name, err)
		}
		opts.Page = currentPage + 1
	}

	return droplets, nil
}

func parseSSHKeys(keys []string) []godo.DropletCreateSSHKey {
	sshKeys := make([]godo.DropletCreateSSHKey, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		id, err := strconv.Atoi(key)
		if err == nil {
			sshKeys = append(sshKeys, godo.DropletCreateSSHKey{ID: id})
			continue
		}
		sshKeys = append(sshKeys, godo.DropletCreateSSHKey{Fingerprint: key})
	}

	return sshKeys
}

func poolTag(poolID string) string {
	return trimTag(fmt.Sprintf("woodpecker-pool-%s", sanitizeTag(poolID)))
}

func sanitizeTag(value string) string {
	value = strings.ToLower(value)
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == ':', r == '_':
			builder.WriteRune(r)
			lastDash = false
		case unicode.IsSpace(r), r == '-', r == '/', r == '.':
			if !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		default:
			if !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}

	tag := strings.Trim(builder.String(), "-")
	if tag == "" {
		return "default"
	}

	return trimTag(tag)
}

func trimTag(tag string) string {
	if len(tag) <= maxTagLength {
		return tag
	}

	return strings.TrimRight(tag[:maxTagLength], "-")
}

func newClient(apiToken string) *godo.Client {
	client := godo.NewFromToken(apiToken)
	client.UserAgent = "woodpecker-autoscaler/" + version.String() + " " + client.UserAgent

	return client
}
