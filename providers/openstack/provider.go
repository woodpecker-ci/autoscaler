package openstack

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/keypairs"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/v2/openstack/image/v2/images"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

const (
	labelPool = "wp.autoscaler-pool" // Override because OpenStack does not allow "/"
)

type provider struct {
	name           string
	region         string
	flavorRef      string
	flavorName     string
	imageRef       string
	imageName      string
	volumeSize     int
	network        string
	securityGroups []string
	keypair        string
	metadata       map[string]string
	config         *config.Config
	computeClient  *gophercloud.ServiceClient
	imageClient    *gophercloud.ServiceClient
	image          *images.Image
	capability     *types.Capability
}

func New(ctx context.Context, c *cli.Command, cfg *config.Config) (types.Provider, error) {
	numericVolumeSize := 0
	if c.String("openstack-volume-size") != "" {
		parsedVolumeSize, err := strconv.Atoi(c.String("openstack-volume-size"))
		if err != nil {
			return nil, fmt.Errorf("openstack: Unable to interprete Volume Size as an integer value: %w", err)
		}
		numericVolumeSize = parsedVolumeSize
	}

	p := &provider{
		name:           "openstack",
		region:         c.String("openstack-region"),
		flavorRef:      c.String("openstack-flavor-ref"),
		flavorName:     c.String("openstack-flavor-name"),
		imageRef:       c.String("openstack-image-ref"),
		imageName:      c.String("openstack-image-name"),
		volumeSize:     numericVolumeSize,
		network:        c.String("openstack-network"),
		securityGroups: c.StringSlice("openstack-security-groups"),
		keypair:        c.String("openstack-keypair"),
		config:         cfg,
	}

	if (p.flavorName == "") == (p.flavorRef == "") {
		return nil, fmt.Errorf("you must set exactly one of Flavor Name or Flavor Ref")
	}
	if (p.imageName == "") == (p.imageRef == "") {
		return nil, fmt.Errorf("you must set exactly one of Image Name or Image Ref")
	}

	// Prepare metadata
	p.metadata = map[string]string{
		labelPool: cfg.PoolID,
	}

	// Authenticate with OpenStack
	opts := gophercloud.AuthOptions{
		IdentityEndpoint:            c.String("openstack-auth-url"),
		Username:                    c.String("openstack-username"),
		Password:                    c.String("openstack-password"),
		ApplicationCredentialID:     c.String("openstack-application-credential-id"),
		ApplicationCredentialName:   c.String("openstack-application-credential-name"),
		ApplicationCredentialSecret: c.String("openstack-application-credential-secret"),
		TenantName:                  c.String("openstack-project-name"),
		DomainName:                  c.String("openstack-domain-name"),
	}

	provider, err := openstack.AuthenticatedClient(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("%s: openstack.AuthenticatedClient: %w", p.name, err)
	}

	computeClient, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
		Region: p.region,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: openstack.NewComputeV2: %w", p.name, err)
	}
	p.computeClient = computeClient

	imageClient, err := openstack.NewImageV2(provider, gophercloud.EndpointOpts{
		Region: p.region,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: openstack.NewImageV2: %w", p.name, err)
	}
	p.imageClient = imageClient

	if err := p.resolveFlavor(ctx); err != nil {
		return nil, err
	}
	if err := p.resolveCapability(ctx); err != nil {
		return nil, err
	}

	return p, nil
}

func (p *provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent, capability types.Capability) error {
	if capability != *p.capability {
		return fmt.Errorf("%s: unsupported capability: platform=%s backend=%s", p.name, capability.Platform, capability.Backend)
	}

	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, cloudinit.RenderOption{})
	if err != nil {
		return fmt.Errorf("%s: cloudinit.RenderUserDataTemplate: %w", p.name, err)
	}

	var networks []servers.Network
	if p.network != "" {
		networks = []servers.Network{
			{UUID: p.network},
		}
	}

	createOpts := servers.CreateOpts{
		Name:           agent.Name,
		FlavorRef:      p.flavorRef,
		Networks:       networks,
		SecurityGroups: p.securityGroups,
		Metadata:       p.metadata,
		UserData:       []byte(userData),
	}

	if p.volumeSize != 0 {
		blockDevice := servers.BlockDevice{
			SourceType:          servers.SourceImage,
			UUID:                p.imageRef,
			DeleteOnTermination: true,
			DestinationType:     servers.DestinationVolume,
			VolumeSize:          p.volumeSize,
		}
		createOpts.BlockDevice = append(createOpts.BlockDevice, blockDevice)
	} else {
		// Implicitly use ephemeral storage - I was not able to make this work with DestinationLocal on my cloud
		createOpts.ImageRef = p.imageRef
	}

	// Wrap with keypair extension if a keypair is configured
	var serverCreateOpts servers.CreateOptsBuilder = createOpts
	if p.keypair != "" {
		serverCreateOpts = keypairs.CreateOptsExt{
			CreateOptsBuilder: createOpts,
			KeyName:           p.keypair,
		}
	}

	server, err := servers.Create(ctx, p.computeClient, serverCreateOpts, nil).Extract()
	if err != nil {
		return fmt.Errorf("%s: servers.Create: %w", p.name, err)
	}

	// Wait for server to become active
	if err := servers.WaitForStatus(ctx, p.computeClient, server.ID, "ACTIVE"); err != nil {
		if deleteErr := servers.Delete(context.WithoutCancel(ctx), p.computeClient, server.ID).ExtractErr(); deleteErr != nil {
			return fmt.Errorf("%s: servers.WaitForStatus: %w; servers.Delete: %w", p.name, err, deleteErr)
		}
		return fmt.Errorf("%s: servers.WaitForStatus: %w", p.name, err)
	}

	return nil
}

func (p *provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	// Find the server by name
	serverID, err := p.findServerIDByName(ctx, agent.Name)
	if err != nil {
		return fmt.Errorf("%s: findServerIDByName: %w", p.name, err)
	}

	if serverID == "" {
		// Server not found, nothing to do
		return nil
	}

	if err := servers.Delete(ctx, p.computeClient, serverID).ExtractErr(); err != nil {
		return fmt.Errorf("%s: servers.Delete: %w", p.name, err)
	}

	return nil
}

func (p *provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	var names []string

	allPages, err := servers.List(p.computeClient, servers.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: servers.List: %w", p.name, err)
	}

	allServers, err := servers.ExtractServers(allPages)
	if err != nil {
		return nil, fmt.Errorf("%s: servers.ExtractServers: %w", p.name, err)
	}

	for _, s := range allServers {
		poolValue, ok := s.Metadata[labelPool]
		if !ok || poolValue != p.config.PoolID {
			continue
		}
		names = append(names, s.Name)
	}

	return names, nil
}

func (p *provider) findServerIDByName(ctx context.Context, name string) (string, error) {
	allPages, err := servers.List(p.computeClient, servers.ListOpts{
		Name: fmt.Sprintf("^%s$", name),
	}).AllPages(ctx)
	if err != nil {
		return "", fmt.Errorf("servers.List: %w", err)
	}

	allServers, err := servers.ExtractServers(allPages)
	if err != nil {
		return "", fmt.Errorf("servers.ExtractServers: %w", err)
	}

	serverID := ""
	for _, s := range allServers {
		if s.Name != name || s.Metadata[labelPool] != p.config.PoolID {
			continue
		}
		if serverID != "" {
			return "", fmt.Errorf("multiple servers found with name %q in pool %q", name, p.config.PoolID)
		}
		serverID = s.ID
	}

	return serverID, nil
}

func (p *provider) resolveFlavor(ctx context.Context) error {
	if p.flavorRef != "" {
		if _, err := flavors.Get(ctx, p.computeClient, p.flavorRef).Extract(); err != nil {
			return fmt.Errorf("%s: flavors.Get: %w", p.name, err)
		}
		return nil
	}

	allPages, err := flavors.ListDetail(p.computeClient, nil).AllPages(ctx)
	if err != nil {
		return fmt.Errorf("%s: Error in flavors.ListDetail: %w", p.name, err)
	}

	allFlavors, err := flavors.ExtractFlavors(allPages)
	if err != nil {
		return fmt.Errorf("%s: Error in flavors.ExtractFlavors: %w", p.name, err)
	}

	for _, flavor := range allFlavors {
		if flavor.Name == p.flavorName {
			p.flavorRef = flavor.ID
			return nil
		}
	}

	return fmt.Errorf("%s: No flavor ID found for flavor name: %s", p.name, p.flavorName)
}

func (p *provider) resolveImage(ctx context.Context) (*images.Image, error) {
	if p.image != nil {
		return p.image, nil
	}

	if p.imageRef != "" {
		image, err := images.Get(ctx, p.imageClient, p.imageRef).Extract()
		if err != nil {
			return nil, fmt.Errorf("%s: images.Get: %w", p.name, err)
		}
		p.image = image
		return image, nil
	}

	listOpts := images.ListOpts{
		Name: p.imageName,
		Sort: "created_at:desc",
	}

	allPages, err := images.List(p.imageClient, listOpts).AllPages(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: Error in images.List: %w", p.name, err)
	}

	allImages, err := images.ExtractImages(allPages)
	if err != nil {
		return nil, fmt.Errorf("%s: Error in images.ExtractImages: %w", p.name, err)
	}
	if len(allImages) == 0 {
		return nil, fmt.Errorf("%s: No image ID found for image name: %s", p.name, p.imageName)
	}

	p.image = &allImages[0]
	p.imageRef = p.image.ID
	return p.image, nil
}

func (p *provider) resolveCapability(ctx context.Context) error {
	image, err := p.resolveImage(ctx)
	if err != nil {
		return err
	}
	capability, err := imageCapability(image)
	if err != nil {
		return fmt.Errorf("%s: %w", p.name, err)
	}
	p.capability = &capability
	return nil
}

func imageCapability(image *images.Image) (types.Capability, error) {
	if osType, ok := image.Properties["os_type"]; ok {
		value, ok := osType.(string)
		if !ok || value != "linux" {
			return types.Capability{}, fmt.Errorf("unsupported OpenStack image os_type %q", osType)
		}
	}

	architecture, ok := image.Properties["hw_architecture"]
	if !ok {
		architecture, ok = image.Properties["architecture"]
	}
	if !ok {
		return types.Capability{}, fmt.Errorf("OpenStack image has no hw_architecture property")
	}

	value, ok := architecture.(string)
	if !ok {
		return types.Capability{}, fmt.Errorf("OpenStack image architecture is not a string: %T", architecture)
	}
	goarch := openStackArchToGoArch(value)
	if goarch == "" {
		return types.Capability{}, fmt.Errorf("unsupported OpenStack image architecture %q", value)
	}

	return types.Capability{
		Platform: "linux/" + goarch,
		Backend:  types.BackendDocker,
	}, nil
}

func openStackArchToGoArch(architecture string) string {
	switch architecture {
	case "aarch64":
		return "arm64"
	case "arm", "armv6", "armv7b", "armv7l":
		return "arm"
	case "i686":
		return "386"
	case "mips":
		return "mips"
	case "mipsel":
		return "mipsle"
	case "mips64":
		return "mips64"
	case "mips64el":
		return "mips64le"
	case "ppc64":
		return "ppc64"
	case "ppc64le":
		return "ppc64le"
	case "s390x":
		return "s390x"
	case "x86_64":
		return "amd64"
	default:
		return ""
	}
}

func (p *provider) Capabilities(_ context.Context) ([]types.Capability, error) {
	return []types.Capability{*p.capability}, nil
}

func (p *provider) BillingModel() types.BillingModel {
	return types.BillingPerSecond
}
