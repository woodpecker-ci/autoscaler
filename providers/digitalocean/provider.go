package digitalocean

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/digitalocean/godo"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	"golang.org/x/crypto/ssh"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var (
	ErrAPITokenNotSet     = errors.New("no api token provided")
	ErrRegionNotFound     = errors.New("region not found")
	ErrSizeNotFound       = errors.New("size not found")
	ErrSizeNotAvailable   = errors.New("size not available")
	ErrImageNotFound      = errors.New("image not found")
	ErrSSHKeyNotFound     = errors.New("SSH key not found")
	ErrIPv6RequiresIPv4   = errors.New("public IPv6 can not be enabled without public IPv4")
	ErrNATGatewayRequired = errors.New("a NAT gateway is required when public IPv4 is disabled")
	ErrNATGatewayNotFound = errors.New("NAT gateway not found")
	ErrNATGatewayInvalid  = errors.New("NAT gateway not usable")
)

// autoSSHKeyName is the SSH key the provider creates and reuses when no key is
// configured. Its private key is generated in memory and discarded.
const autoSSHKeyName = "random-autoscaler-key"

var invalidTagPart = regexp.MustCompile(`[^a-z0-9:_-]+`)

// blackhole metadata services so running steps can not extract agent token from user-data
// https://docs.digitalocean.com/products/droplets/how-to/access-metadata/ (served over IPv4 169.254.169.254 only)
var blackholeMetadataAPI = []string{
	"ip -4 route add blackhole 169.254.169.254/32",
}

const perPage = 200

type provider struct {
	name       string
	config     *config.Config
	client     *godo.Client
	region     godo.Region
	size       godo.Size
	image      godo.Image
	sshKeys    []godo.DropletCreateSSHKey
	tags       []string
	vpcUUID    string
	enableIPv4 bool
	enableIPv6 bool
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (types.Provider, error) {
	apiToken := c.String("digitalocean-api-token")
	if apiToken == "" {
		return nil, ErrAPITokenNotSet
	}

	return newProviderWithClient(ctx, c, config, godo.NewFromToken(apiToken))
}

func newProviderWithClient(ctx context.Context, c *cli.Command, config *config.Config, client *godo.Client) (types.Provider, error) {
	p := &provider{
		name:       "digitalocean",
		config:     config,
		enableIPv4: c.Bool("digitalocean-public-ipv4-enable"),
		enableIPv6: c.Bool("digitalocean-public-ipv6-enable"),
		client:     client,
	}

	// DigitalOcean has no IPv6-only droplets: public IPv6 is only served on the
	// public interface, which always carries IPv4 as well.
	if p.enableIPv6 && !p.enableIPv4 {
		return nil, fmt.Errorf("%s: %w", p.name, ErrIPv6RequiresIPv4)
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
	if err := p.resolveNATGateway(ctx, c.String("digitalocean-nat-gateway")); err != nil {
		return nil, err
	}

	p.tags = slices.Clone(c.StringSlice("digitalocean-tags"))
	p.tags = append(p.tags, poolTag(config.PoolID))
	p.tags = append(p.tags, imageTag(p.image.Slug))

	return p, nil
}

func (p *provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, cloudinit.RenderOption{
		PreExec: blackholeMetadataAPI,
	})
	if err != nil {
		return fmt.Errorf("%s: cloudinit.RenderUserDataTemplate: %w", p.name, err)
	}

	req := &godo.DropletCreateRequest{
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
	if !p.enableIPv4 {
		// creates the droplet without a public network interface
		req.PublicNetworking = godo.PtrTo(false)
	}
	if p.vpcUUID != "" {
		req.VPCUUID = p.vpcUUID
	}

	if _, _, err := p.client.Droplets.Create(ctx, req); err != nil {
		return fmt.Errorf("%s: Droplets.Create: %w", p.name, err)
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

	if _, err := p.client.Droplets.Delete(ctx, droplet.ID); err != nil {
		return fmt.Errorf("%s: Droplets.Delete: %w", p.name, err)
	}

	return nil
}

func (p *provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	droplets, err := p.listPoolDroplets(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: Droplets.ListByTag: %w", p.name, err)
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
	regions, err := listAll(ctx, p.client.Regions.List)
	if err != nil {
		return fmt.Errorf("%s: Regions.List: %w", p.name, err)
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
	sizes, err := listAll(ctx, p.client.Sizes.List)
	if err != nil {
		return fmt.Errorf("%s: Sizes.List: %w", p.name, err)
	}

	for _, size := range sizes {
		if size.Slug != sizeSlug {
			continue
		}
		if !slices.Contains(size.Regions, p.region.Slug) {
			return fmt.Errorf("%w: size %s is not offered in region %s", ErrSizeNotAvailable, sizeSlug, p.region.Slug)
		}
		if !size.Available {
			return fmt.Errorf("%w: size %s is currently not available", ErrSizeNotAvailable, sizeSlug)
		}
		p.size = size
		return nil
	}

	return ErrSizeNotFound
}

func (p *provider) resolveImage(ctx context.Context, selector string) error {
	images, err := listAll(ctx, p.client.Images.List)
	if err != nil {
		return fmt.Errorf("%s: Images.List: %w", p.name, err)
	}

	var matches []godo.Image
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

	slices.SortFunc(matches, func(a, b godo.Image) int {
		return strings.Compare(a.Name, b.Name)
	})
	p.image = matches[0]
	if len(matches) > 1 {
		log.Info().Msgf("digitalocean image selector had %d matches, chose %q", len(matches), matches[0].Name)
	}

	return nil
}

// resolveNATGateway looks up the configured VPC NAT gateway and pins agents to
// the VPC it serves as default gateway, so private droplets get egress. Private
// droplets without a gateway have no way to reach the server, so this is
// required when public IPv4 is disabled.
func (p *provider) resolveNATGateway(ctx context.Context, selector string) error {
	if selector == "" {
		if !p.enableIPv4 {
			return fmt.Errorf("%s: %w", p.name, ErrNATGatewayRequired)
		}
		return nil
	}

	gateways, err := listAll(ctx, func(ctx context.Context, opt *godo.ListOptions) ([]*godo.VPCNATGateway, *godo.Response, error) {
		return p.client.VPCNATGateways.List(ctx, &godo.VPCNATGatewaysListOptions{ListOptions: *opt})
	})
	if err != nil {
		return fmt.Errorf("%s: VPCNATGateways.List: %w", p.name, err)
	}

	for _, gateway := range gateways {
		// skip gateways matching neither the ID nor the name
		if gateway.ID != selector && gateway.Name != selector {
			continue
		}
		if gateway.Region != p.region.Slug {
			return fmt.Errorf("%s: %w: gateway %s is in region %s, not %s", p.name, ErrNATGatewayInvalid, selector, gateway.Region, p.region.Slug)
		}
		for _, vpc := range gateway.VPCs {
			if vpc.DefaultGateway {
				p.vpcUUID = vpc.VpcUUID
				return nil
			}
		}
		return fmt.Errorf("%s: %w: gateway %s is not the default gateway of any VPC", p.name, ErrNATGatewayInvalid, selector)
	}

	return fmt.Errorf("%s: %w: %s", p.name, ErrNATGatewayNotFound, selector)
}

func (p *provider) setupKeyPair(ctx context.Context, configuredKeys []string) error {
	keys, err := listAll(ctx, p.client.Keys.List)
	if err != nil {
		return err
	}

	byName := make(map[string]string, len(keys))
	byFingerprint := make(map[string]string, len(keys))
	for _, key := range keys {
		byName[key.Name] = key.Fingerprint
		byFingerprint[key.Fingerprint] = key.Fingerprint
	}

	if len(configuredKeys) > 0 {
		p.sshKeys = make([]godo.DropletCreateSSHKey, 0, len(configuredKeys))
		for _, configuredKey := range configuredKeys {
			if fingerprint, ok := byName[configuredKey]; ok {
				p.sshKeys = append(p.sshKeys, godo.DropletCreateSSHKey{Fingerprint: fingerprint})
				continue
			}
			if fingerprint, ok := byFingerprint[configuredKey]; ok {
				p.sshKeys = append(p.sshKeys, godo.DropletCreateSSHKey{Fingerprint: fingerprint})
				continue
			}
			return fmt.Errorf("%w: %s", ErrSSHKeyNotFound, configuredKey)
		}

		return nil
	}

	// No key configured. SSH access is not needed for the agent to work, but
	// creating a droplet without a key makes DigitalOcean email a root
	// password, so reuse or create a throwaway key instead.
	if fingerprint, ok := byName[autoSSHKeyName]; ok {
		log.Info().Msgf("%s: using existing SSH key %q", p.name, autoSSHKeyName)
		p.sshKeys = []godo.DropletCreateSSHKey{{Fingerprint: fingerprint}}
		return nil
	}

	publicKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		return err
	}
	sshPublicKey, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		return err
	}

	key, _, err := p.client.Keys.Create(ctx, &godo.KeyCreateRequest{
		Name:      autoSSHKeyName,
		PublicKey: string(ssh.MarshalAuthorizedKey(sshPublicKey)),
	})
	if err != nil {
		return fmt.Errorf("Keys.Create: %w", err)
	}

	log.Info().Msgf("%s: created SSH key %q, its private key was discarded", p.name, autoSSHKeyName)
	p.sshKeys = []godo.DropletCreateSSHKey{{Fingerprint: key.Fingerprint}}
	return nil
}

func (p *provider) listPoolDroplets(ctx context.Context) ([]godo.Droplet, error) {
	tag := poolTag(p.config.PoolID)
	return listAll(ctx, func(ctx context.Context, opt *godo.ListOptions) ([]godo.Droplet, *godo.Response, error) {
		return p.client.Droplets.ListByTag(ctx, tag, opt)
	})
}

func (p *provider) getAgent(ctx context.Context, name string) (*godo.Droplet, error) {
	droplets, err := p.listPoolDroplets(ctx)
	if err != nil {
		return nil, err
	}

	var matches []godo.Droplet
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

// listAll drains a paginated godo list endpoint.
func listAll[T any](ctx context.Context, list func(context.Context, *godo.ListOptions) ([]T, *godo.Response, error)) ([]T, error) {
	var all []T
	opt := &godo.ListOptions{Page: 1, PerPage: perPage}
	for {
		items, resp, err := list(ctx, opt)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)

		if resp == nil || resp.Links == nil || resp.Links.IsLastPage() {
			return all, nil
		}

		page, err := resp.Links.CurrentPage()
		if err != nil {
			return nil, err
		}
		opt.Page = page + 1
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

func createImageRef(image godo.Image) godo.DropletCreateImage {
	if image.Slug != "" {
		return godo.DropletCreateImage{Slug: image.Slug}
	}

	return godo.DropletCreateImage{ID: image.ID}
}
