package digitalocean

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var (
	ErrAPITokenNotSet = errors.New("no api token provided")
	ErrRegionNotFound = errors.New("region not found")
	ErrSizeNotFound   = errors.New("size not found")
	ErrImageNotFound  = errors.New("image not found")
	ErrSSHKeyNotFound = errors.New("SSH key not found")
)

var invalidTagPart = regexp.MustCompile(`[^a-z0-9:_-]+`)

type provider struct {
	name       string
	config     *config.Config
	client     *client
	region     region
	size       size
	image      image
	sshKeys    []string
	tags       []string
	enableIPv6 bool
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (types.Provider, error) {
	apiToken := c.String("digitalocean-api-token")
	if apiToken == "" {
		return nil, ErrAPITokenNotSet
	}

	return newProviderWithClient(ctx, c, config, newClient(http.DefaultClient, defaultBaseURL, apiToken))
}

func newProviderWithClient(ctx context.Context, c *cli.Command, config *config.Config, client *client) (types.Provider, error) {
	p := &provider{
		name:       "digitalocean",
		config:     config,
		enableIPv6: c.Bool("digitalocean-public-ipv6-enable"),
		client:     client,
	}

	if err := p.resolveRegion(ctx, c.String("digitalocean-region")); err != nil {
		return nil, err
	}
	if err := p.resolveSize(ctx, c.String("digitalocean-size")); err != nil {
		return nil, err
	}
	if err := p.resolveImage(ctx, c.String("digitalocean-image")); err != nil {
		return nil, err
	}
	if err := p.setupKeyPair(ctx, c.StringSlice("digitalocean-ssh-keys")); err != nil {
		return nil, fmt.Errorf("%s: setupKeyPair: %w", p.name, err)
	}

	p.tags = slices.Clone(c.StringSlice("digitalocean-tags"))
	p.tags = append(p.tags, poolTag(config.PoolID))
	p.tags = append(p.tags, imageTag(p.image.Slug))

	return p, nil
}

func (p *provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, cloudinit.RenderOption{})
	if err != nil {
		return fmt.Errorf("%s: cloudinit.RenderUserDataTemplate: %w", p.name, err)
	}

	req := dropletCreateRequest{
		Name:     agent.Name,
		Region:   p.region.Slug,
		Size:     p.size.Slug,
		Image:    createImageRef(p.image),
		SSHKeys:  p.sshKeys,
		UserData: userData,
		IPv6:     p.enableIPv6,
		Tags:     slices.Clone(p.tags),
		Backups:  false,
	}

	if _, err := p.client.createDroplet(ctx, req); err != nil {
		return fmt.Errorf("%s: createDroplet: %w", p.name, err)
	}

	return nil
}

func (p *provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	droplet, err := p.getAgent(ctx, agent.Name)
	if err != nil {
		return fmt.Errorf("%s: getAgent: %w", p.name, err)
	}
	if droplet == nil {
		return nil
	}

	if err := p.client.deleteDroplet(ctx, droplet.ID); err != nil {
		return fmt.Errorf("%s: deleteDroplet: %w", p.name, err)
	}

	return nil
}

func (p *provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	droplets, err := p.client.listDropletsByTag(ctx, poolTag(p.config.PoolID))
	if err != nil {
		return nil, fmt.Errorf("%s: listDropletsByTag: %w", p.name, err)
	}

	names := make([]string, 0, len(droplets))
	for _, droplet := range droplets {
		if droplet.Status != "new" && droplet.Status != "active" {
			continue
		}

		names = append(names, droplet.Name)
	}

	return names, nil
}

func (p *provider) BillingModel() types.BillingModel {
	return types.BillingHourlyRoundUp
}

func (p *provider) resolveRegion(ctx context.Context, regionSlug string) error {
	regions, err := p.client.listRegions(ctx)
	if err != nil {
		return fmt.Errorf("%s: listRegions: %w", p.name, err)
	}

	for _, region := range regions {
		if region.Slug == regionSlug && region.Available {
			p.region = region
			return nil
		}
	}

	return ErrRegionNotFound
}

func (p *provider) resolveSize(ctx context.Context, sizeSlug string) error {
	sizes, err := p.client.listSizes(ctx)
	if err != nil {
		return fmt.Errorf("%s: listSizes: %w", p.name, err)
	}

	for _, size := range sizes {
		if size.Slug == sizeSlug && slices.Contains(size.Regions, p.region.Slug) && size.Available {
			p.size = size
			return nil
		}
	}

	return ErrSizeNotFound
}

func (p *provider) resolveImage(ctx context.Context, selector string) error {
	images, err := p.client.listImages(ctx)
	if err != nil {
		return fmt.Errorf("%s: listImages: %w", p.name, err)
	}

	var matches []image
	want := normalizeSelector(selector)
	for _, image := range images {
		switch {
		case image.Slug != "" && strings.EqualFold(image.Slug, selector):
			p.image = image
			return nil
		case normalizeSelector(image.Name) == want:
			matches = append(matches, image)
		case normalizeSelector(strings.TrimSpace(image.Distribution+" "+image.Name)) == want:
			matches = append(matches, image)
		}
	}

	if len(matches) == 0 {
		return ErrImageNotFound
	}

	slices.SortFunc(matches, func(a, b image) int {
		return strings.Compare(a.Name, b.Name)
	})
	p.image = matches[0]
	if len(matches) > 1 {
		log.Info().Msgf("digitalocean image selector had %d matches, chose %q", len(matches), matches[0].Name)
	}

	return nil
}

func (p *provider) setupKeyPair(ctx context.Context, configuredKeys []string) error {
	keys, err := p.client.listSSHKeys(ctx)
	if err != nil {
		return err
	}

	if len(keys) == 0 {
		return ErrSSHKeyNotFound
	}

	byName := make(map[string]string, len(keys))
	byFingerprint := make(map[string]string, len(keys))
	for _, key := range keys {
		byName[key.Name] = key.Fingerprint
		byFingerprint[key.Fingerprint] = key.Fingerprint
	}

	if len(configuredKeys) > 0 {
		p.sshKeys = make([]string, 0, len(configuredKeys))
		for _, configuredKey := range configuredKeys {
			if fingerprint, ok := byName[configuredKey]; ok {
				p.sshKeys = append(p.sshKeys, fingerprint)
				continue
			}
			if fingerprint, ok := byFingerprint[configuredKey]; ok {
				p.sshKeys = append(p.sshKeys, fingerprint)
				continue
			}
			return fmt.Errorf("%w: %s", ErrSSHKeyNotFound, configuredKey)
		}

		return nil
	}

	for _, name := range []string{"woodpecker", "id_rsa_woodpecker"} {
		if fingerprint, ok := byName[name]; ok {
			p.sshKeys = []string{fingerprint}
			return nil
		}
	}

	p.sshKeys = []string{keys[0].Fingerprint}
	return nil
}

func (p *provider) getAgent(ctx context.Context, name string) (*droplet, error) {
	droplets, err := p.client.listDropletsByTag(ctx, poolTag(p.config.PoolID))
	if err != nil {
		return nil, err
	}

	var matches []droplet
	for _, droplet := range droplets {
		if droplet.Name == name {
			matches = append(matches, droplet)
		}
	}

	switch len(matches) {
	case 0:
		return nil, nil
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("found multiple droplets named %s", name)
	}
}

func poolTag(poolID string) string {
	return "wp-autoscaler-pool-" + sanitizeTagPart(poolID)
}

func imageTag(image string) string {
	return "wp-autoscaler-image-" + sanitizeTagPart(image)
}

func sanitizeTagPart(value string) string {
	value = strings.ToLower(value)
	value = invalidTagPart.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "default"
	}

	return value
}

func normalizeSelector(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "")
	return value
}

func createImageRef(image image) dropletCreateImage {
	if image.Slug != "" {
		return dropletCreateImage{Slug: image.Slug}
	}

	return dropletCreateImage{ID: image.ID}
}
