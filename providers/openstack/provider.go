package openstack

import (
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/keypairs"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type Provider struct {
	name             string
	region           string
	flavor           string
	image            string
	network          string
	securityGroups   []string
	keypair          string
	floatingIPPool   string
	metadata         map[string]string
	userDataTemplate *template.Template
	config           *config.Config
	computeClient    *gophercloud.ServiceClient
}

func New(ctx context.Context, c *cli.Command, cfg *config.Config) (engine.Provider, error) {
	p := &Provider{
		name:           "openstack",
		region:         c.String("openstack-region"),
		flavor:         c.String("openstack-flavor"),
		image:          c.String("openstack-image"),
		network:        c.String("openstack-network"),
		securityGroups: c.StringSlice("openstack-security-groups"),
		keypair:        c.String("openstack-keypair"),
		floatingIPPool: c.String("openstack-floating-ip-pool"),
		config:         cfg,
	}

	// Parse metadata
	p.metadata = map[string]string{
		engine.LabelPool: cfg.PoolID,
	}
	for _, m := range c.StringSlice("openstack-metadata") {
		key, value, ok := strings.Cut(m, "=")
		if ok {
			p.metadata[key] = value
		}
	}

	// # TODO: Deprecated remove in v2.0
	if u := c.String("openstack-user-data"); u != "" {
		log.Warn().Msg("openstack-user-data is deprecated, please use provider-user-data instead")
		userDataTmpl, err := template.New("user-data").Parse(u)
		if err != nil {
			return nil, fmt.Errorf("%s: template.New.Parse %w", p.name, err)
		}
		p.userDataTemplate = userDataTmpl
	}

	// Authenticate with OpenStack
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: c.String("openstack-auth-url"),
		Username:         c.String("openstack-username"),
		Password:         c.String("openstack-password"),
		TenantName:       c.String("openstack-tenant-name"),
		DomainName:       c.String("openstack-domain-name"),
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

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := engine.RenderUserDataTemplate(p.config, agent, p.userDataTemplate)
	if err != nil {
		return fmt.Errorf("%s: engine.RenderUserDataTemplate: %w", p.name, err)
	}

	var networks []servers.Network
	if p.network != "" {
		networks = []servers.Network{
			{UUID: p.network},
		}
	}

	createOpts := servers.CreateOpts{
		Name:           agent.Name,
		FlavorRef:      p.flavor,
		ImageRef:       p.image,
		Networks:       networks,
		SecurityGroups: p.securityGroups,
		Metadata:       p.metadata,
		UserData:       []byte(userData),
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

func (p *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
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

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
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
		poolValue, ok := s.Metadata[engine.LabelPool]
		if !ok || poolValue != p.config.PoolID {
			continue
		}
		names = append(names, s.Name)
	}

	return names, nil
}

func (p *Provider) findServerIDByName(ctx context.Context, name string) (string, error) {
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
