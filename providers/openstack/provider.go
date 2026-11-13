package openstack

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/keypairs"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/v2/openstack/image/v2/images"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// blackhole metadata services so running steps can not extract agent token from user-data
// https://docs.openstack.org/nova/latest/user/metadata.html
// IPv6 metadata (fe80::a9fe:a9fe) is deployment-specific and requires an interface
// scope, so it can not be blackholed generically here.
var blackholeMetadataAPI = []string{
	"ip -4 route add blackhole 169.254.169.254/32",
}

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
	networkRef     string
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
	p := &provider{
		name:           "openstack",
		region:         c.String("openstack-region"),
		flavorRef:      c.String("openstack-flavor-ref"),
		flavorName:     c.String("openstack-flavor-name"),
		imageRef:       c.String("openstack-image-ref"),
		imageName:      c.String("openstack-image-name"),
		volumeSize:     c.Int("openstack-volume-size"),
		networkRef:     c.String("openstack-network-ref"),
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
		AllowReauth:                 true,
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

	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, cloudinit.RenderOption{
		PreExec: blackholeMetadataAPI,
	})
	if err != nil {
		return fmt.Errorf("%s: cloudinit.RenderUserDataTemplate: %w", p.name, err)
	}

	createOpts := servers.CreateOpts{
		Name:           agent.Name,
		FlavorRef:      p.flavorRef,
		SecurityGroups: p.securityGroups,
		Metadata:       p.metadata,
		UserData:       []byte(userData),
	}

	if p.networkRef != "" {
		createOpts.Networks = []servers.Network{{UUID: p.networkRef}}
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
	// Names are not unique in OpenStack, remove every server carrying it
	serverIDs, err := p.findServerIDsByName(ctx, agent.Name)
	if err != nil {
		return fmt.Errorf("%s: findServerIDsByName: %w", p.name, err)
	}

	for _, serverID := range serverIDs {
		if err := servers.Delete(ctx, p.computeClient, serverID).ExtractErr(); err != nil {
			return fmt.Errorf("%s: servers.Delete: %w", p.name, err)
		}
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

func (p *provider) Capabilities(_ context.Context) ([]types.Capability, error) {
	return []types.Capability{*p.capability}, nil
}

func (p *provider) BillingModel() types.BillingModel {
	return types.BillingPerSecond
}
