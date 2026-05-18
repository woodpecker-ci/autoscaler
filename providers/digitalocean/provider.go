package digitalocean

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/digitalocean/godo"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var (
	ErrParameterNotSet          = errors.New("required parameter not set")
	ErrImageNotFound            = errors.New("image not found")
	ErrImageUnavailableInRegion = errors.New("image unavailable in region")
	ErrInvalidTag               = errors.New("invalid tag")
	ErrRegionNotFound           = errors.New("region not found")
	ErrRegionUnavailable        = errors.New("region unavailable")
	ErrSSHKeyNotFound           = errors.New("SSH key not found")
	ErrSizeNotFound             = errors.New("size not found")
	ErrSizeUnavailable          = errors.New("size unavailable")
	ErrSizeUnavailableInRegion  = errors.New("size unavailable in region")
)

type dropletsService interface {
	Create(context.Context, *godo.DropletCreateRequest) (*godo.Droplet, *godo.Response, error)
	Delete(context.Context, int) (*godo.Response, error)
	ListByTag(context.Context, string, *godo.ListOptions) ([]godo.Droplet, *godo.Response, error)
}

type imagesService interface {
	GetBySlug(context.Context, string) (*godo.Image, *godo.Response, error)
}

type keysService interface {
	List(context.Context, *godo.ListOptions) ([]godo.Key, *godo.Response, error)
}

type regionsService interface {
	List(context.Context, *godo.ListOptions) ([]godo.Region, *godo.Response, error)
}

type sizesService interface {
	List(context.Context, *godo.ListOptions) ([]godo.Size, *godo.Response, error)
}

type Provider struct {
	name       string
	region     string
	size       string
	image      string
	sshKeys    []godo.DropletCreateSSHKey
	tags       []string
	poolTag    string
	enableIPv6 bool
	vpcUUID    string
	config     *config.Config
	droplets   dropletsService
	images     imagesService
	keys       keysService
	regions    regionsService
	sizes      sizesService
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (types.Provider, error) {
	token := strings.TrimSpace(c.String("digitalocean-api-token"))
	if token == "" {
		return nil, fmt.Errorf("%w: digitalocean-api-token", ErrParameterNotSet)
	}

	client := godo.NewFromToken(token)
	p := &Provider{
		name:       "digitalocean",
		region:     c.String("digitalocean-region"),
		size:       c.String("digitalocean-size"),
		image:      c.String("digitalocean-image"),
		enableIPv6: c.Bool("digitalocean-ipv6"),
		vpcUUID:    c.String("digitalocean-vpc-uuid"),
		config:     config,
		droplets:   client.Droplets,
		images:     client.Images,
		keys:       client.Keys,
		regions:    client.Regions,
		sizes:      client.Sizes,
	}

	p.poolTag = buildInternalTag("pool", config.PoolID)
	p.tags = []string{
		p.poolTag,
		buildInternalTag("image", p.image),
	}

	for _, tag := range c.StringSlice("digitalocean-tags") {
		tag = strings.TrimSpace(tag)
		if !validTag(tag) {
			return nil, fmt.Errorf("%s: %w: %s", p.name, ErrInvalidTag, tag)
		}
		p.tags = append(p.tags, tag)
	}

	if err := p.resolveConfig(ctx, c.StringSlice("digitalocean-ssh-keys")); err != nil {
		return nil, err
	}

	return p, nil
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, nil)
	if err != nil {
		return fmt.Errorf("%s: cloudinit.RenderUserDataTemplate: %w", p.name, err)
	}

	createReq := &godo.DropletCreateRequest{
		Name:     agent.Name,
		Region:   p.region,
		Size:     p.size,
		UserData: userData,
		SSHKeys:  p.sshKeys,
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

	if _, _, err = p.droplets.Create(ctx, createReq); err != nil {
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

	if _, err = p.droplets.Delete(ctx, droplet.ID); err != nil {
		return fmt.Errorf("%s: Droplets.Delete: %w", p.name, err)
	}

	return nil
}

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	droplets, err := p.listDropletsByPool(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: Droplets.ListByTag: %w", p.name, err)
	}

	names := make([]string, 0, len(droplets))
	for _, droplet := range droplets {
		names = append(names, droplet.Name)
	}

	return names, nil
}

func (p *Provider) findDropletByName(ctx context.Context, name string) (*godo.Droplet, error) {
	droplets, err := p.listDropletsByPool(ctx)
	if err != nil {
		return nil, err
	}

	var found *godo.Droplet
	for i := range droplets {
		if droplets[i].Name != name {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("found multiple droplets with name %s", name)
		}
		found = &droplets[i]
	}

	return found, nil
}

func (p *Provider) listDropletsByPool(ctx context.Context) ([]godo.Droplet, error) {
	var droplets []godo.Droplet

	page := 1
	for {
		list, resp, err := p.droplets.ListByTag(ctx, p.poolTag, &godo.ListOptions{
			Page:    page,
			PerPage: 200, //nolint:mnd
		})
		if err != nil {
			return nil, err
		}

		droplets = append(droplets, list...)
		if !hasNextPage(resp) {
			return droplets, nil
		}
		page++
	}
}

func (p *Provider) resolveConfig(ctx context.Context, rawSSHKeys []string) error {
	if strings.TrimSpace(p.region) == "" {
		return fmt.Errorf("%w: digitalocean-region", ErrParameterNotSet)
	}
	if strings.TrimSpace(p.size) == "" {
		return fmt.Errorf("%w: digitalocean-size", ErrParameterNotSet)
	}
	if strings.TrimSpace(p.image) == "" {
		return fmt.Errorf("%w: digitalocean-image", ErrParameterNotSet)
	}

	region, err := p.resolveRegion(ctx)
	if err != nil {
		return err
	}

	size, err := p.resolveSize(ctx)
	if err != nil {
		return err
	}

	if !contains(region.Sizes, p.size) || !contains(size.Regions, p.region) {
		return fmt.Errorf("%s: %w: %s in %s", p.name, ErrSizeUnavailableInRegion, p.size, p.region)
	}

	image, _, err := p.images.GetBySlug(ctx, p.image)
	if err != nil {
		return fmt.Errorf("%s: Images.GetBySlug: %w", p.name, err)
	}
	if image == nil {
		return fmt.Errorf("%s: %w: %s", p.name, ErrImageNotFound, p.image)
	}
	if len(image.Regions) > 0 && !contains(image.Regions, p.region) {
		return fmt.Errorf("%s: %w: %s in %s", p.name, ErrImageUnavailableInRegion, p.image, p.region)
	}

	sshKeys, err := p.resolveSSHKeys(ctx, rawSSHKeys)
	if err != nil {
		return err
	}
	p.sshKeys = sshKeys

	return nil
}

func (p *Provider) resolveRegion(ctx context.Context) (*godo.Region, error) {
	page := 1
	for {
		regions, resp, err := p.regions.List(ctx, &godo.ListOptions{Page: page, PerPage: 200}) //nolint:mnd
		if err != nil {
			return nil, fmt.Errorf("%s: Regions.List: %w", p.name, err)
		}

		for i := range regions {
			if regions[i].Slug != p.region {
				continue
			}
			if !regions[i].Available {
				return nil, fmt.Errorf("%s: %w: %s", p.name, ErrRegionUnavailable, p.region)
			}
			return &regions[i], nil
		}

		if !hasNextPage(resp) {
			return nil, fmt.Errorf("%s: %w: %s", p.name, ErrRegionNotFound, p.region)
		}
		page++
	}
}

func (p *Provider) resolveSize(ctx context.Context) (*godo.Size, error) {
	page := 1
	for {
		sizes, resp, err := p.sizes.List(ctx, &godo.ListOptions{Page: page, PerPage: 200}) //nolint:mnd
		if err != nil {
			return nil, fmt.Errorf("%s: Sizes.List: %w", p.name, err)
		}

		for i := range sizes {
			if sizes[i].Slug != p.size {
				continue
			}
			if !sizes[i].Available {
				return nil, fmt.Errorf("%s: %w: %s", p.name, ErrSizeUnavailable, p.size)
			}
			return &sizes[i], nil
		}

		if !hasNextPage(resp) {
			return nil, fmt.Errorf("%s: %w: %s", p.name, ErrSizeNotFound, p.size)
		}
		page++
	}
}

func (p *Provider) resolveSSHKeys(ctx context.Context, rawKeys []string) ([]godo.DropletCreateSSHKey, error) {
	keys, err := p.listSSHKeys(ctx)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("%s: %w", p.name, ErrSSHKeyNotFound)
	}

	byName := make(map[string]godo.Key, len(keys))
	byFingerprint := make(map[string]godo.Key, len(keys))
	byID := make(map[string]godo.Key, len(keys))
	for _, key := range keys {
		byName[key.Name] = key
		byFingerprint[key.Fingerprint] = key
		byID[strconv.Itoa(key.ID)] = key
	}

	if len(rawKeys) == 0 {
		for _, name := range []string{"woodpecker", "id_rsa_woodpecker"} {
			key, ok := byName[name]
			if ok {
				return []godo.DropletCreateSSHKey{dropletSSHKey(key)}, nil
			}
		}
		return []godo.DropletCreateSSHKey{dropletSSHKey(keys[0])}, nil
	}

	resolved := make([]godo.DropletCreateSSHKey, 0, len(rawKeys))
	for _, raw := range rawKeys {
		raw = strings.TrimSpace(raw)
		key, ok := byFingerprint[raw]
		if !ok {
			key, ok = byID[raw]
		}
		if !ok {
			return nil, fmt.Errorf("%s: %w: %s", p.name, ErrSSHKeyNotFound, raw)
		}
		resolved = append(resolved, dropletSSHKey(key))
	}

	return resolved, nil
}

func (p *Provider) listSSHKeys(ctx context.Context) ([]godo.Key, error) {
	var keys []godo.Key

	page := 1
	for {
		list, resp, err := p.keys.List(ctx, &godo.ListOptions{Page: page, PerPage: 200}) //nolint:mnd
		if err != nil {
			return nil, fmt.Errorf("%s: Keys.List: %w", p.name, err)
		}

		keys = append(keys, list...)
		if !hasNextPage(resp) {
			return keys, nil
		}
		page++
	}
}

func dropletSSHKey(key godo.Key) godo.DropletCreateSSHKey {
	if key.ID != 0 {
		return godo.DropletCreateSSHKey{ID: key.ID}
	}

	return godo.DropletCreateSSHKey{Fingerprint: key.Fingerprint}
}

func buildInternalTag(kind, value string) string {
	value = sanitizeTagValue(value)
	if value == "" {
		value = "default"
	}

	tag := "wp-autoscaler-" + kind + "-" + value
	if len(tag) > 255 {
		return tag[:255]
	}

	return tag
}

func sanitizeTagValue(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == ':' || r == '-' || r == '_':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}

	return strings.Trim(b.String(), "-")
}

func validTag(tag string) bool {
	if tag == "" || len(tag) > 255 {
		return false
	}

	for _, r := range tag {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == ':' || r == '-' || r == '_':
		default:
			return false
		}
	}

	return true
}

func contains(list []string, item string) bool {
	for _, elem := range list {
		if elem == item {
			return true
		}
	}
	return false
}

func hasNextPage(resp *godo.Response) bool {
	return resp != nil && resp.Links != nil && resp.Links.Pages != nil && resp.Links.Pages.Next != ""
}

var _ types.Provider = (*Provider)(nil)
