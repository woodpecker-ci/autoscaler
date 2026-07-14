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

	if p.flavorName != "" && p.flavorRef != "" {
		return nil, fmt.Errorf("you must set either Flavor Name or Flavor Ref")
	}
	if p.imageName != "" && p.imageRef != "" {
		return nil, fmt.Errorf("you must set either Image Name or Image Ref")
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

	return p, nil
}

func (p *provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
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

	if p.flavorRef == "" {
		allPages, err := flavors.ListDetail(p.computeClient, nil).AllPages(ctx)
		if err != nil {
			return fmt.Errorf("%s: Error in flavors.ListDetail: %w", p.name, err)
		}

		allFlavors, err := flavors.ExtractFlavors(allPages)
		if err != nil {
			return fmt.Errorf("%s: Error in flavors.ExtractFlavors: %w", p.name, err)
		}

		for _, f := range allFlavors {
			if f.Name == p.flavorName {
				p.flavorRef = f.ID
				break
			}
		}

		if p.flavorRef == "" {
			return fmt.Errorf("%s: No flavor ID found for flavor name: %s", p.name, p.flavorName)
		}
	}

	if p.imageRef == "" {
		listOpts := images.ListOpts{
			Name: p.imageName,
			Sort: "created_at:desc",
		}

		allPages, err := images.List(p.computeClient, listOpts).AllPages(ctx)
		if err != nil {
			return fmt.Errorf("%s: Error in images.List: %w", p.name, err)
		}

		allImages, err := images.ExtractImages(allPages)
		if err != nil {
			return fmt.Errorf("%s: Error in images.ExtractImages: %w", p.name, err)
		}

		if len(allImages) == 0 {
			return fmt.Errorf("%s: No image ID found for image name: %s", p.name, p.imageName)
		}

		p.imageRef = allImages[0].ID
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

	for _, s := range allServers {
		if s.Name == name {
			return s.ID, nil
		}
	}

	return "", nil
}

func (p *provider) BillingModel() types.BillingModel {
	return types.BillingPerSecond
}
