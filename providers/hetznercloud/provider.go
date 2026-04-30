package hetznercloud

import (
	"context"
	"fmt"
	"strings"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	"golang.org/x/exp/maps"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/autoscaler/providers/hetznercloud/hcapi"
	"go.woodpecker-ci.org/autoscaler/utils"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type provider struct {
	name             string
	deployCandidates []*deployCandidate
	sshKeys          []string
	labels           map[string]string
	config           *config.Config
	networks         []string
	firewalls        []string
	enableIPv4       bool
	enableIPv6       bool
	client           hcapi.Client
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (types.Provider, error) {
	p := &provider{
		name: "hetznercloud",

		sshKeys:    c.StringSlice("hetznercloud-ssh-keys"),
		firewalls:  c.StringSlice("hetznercloud-firewalls"),
		networks:   c.StringSlice("hetznercloud-networks"),
		enableIPv4: c.Bool("hetznercloud-public-ipv4-enable"),
		enableIPv6: c.Bool("hetznercloud-public-ipv6-enable"),
		config:     config,
	}

	p.client = hcapi.NewClient(hcloud.WithToken(c.String("hetznercloud-api-token")))

	if err := p.resolveServerConfigs(ctx, c.StringSlice("hetznercloud-server-type"), c.String("hetznercloud-image")); err != nil {
		return nil, err
	}

	defaultLabels := make(map[string]string, 0)
	defaultLabels[engine.LabelPool] = p.config.PoolID
	defaultLabels[engine.LabelImage] = p.deployCandidates[0].image.Name

	labels, err := utils.SliceToMap(c.StringSlice("hetznercloud-labels"), "=")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}

	for _, key := range maps.Keys(labels) {
		if strings.HasPrefix(key, engine.LabelPrefix) {
			return nil, fmt.Errorf("%s: %w: %s", p.name, ErrIllegalLabelPrefix, engine.LabelPrefix)
		}
	}
	p.labels = utils.MergeMaps(defaultLabels, p.labels)

	return p, nil
}

func (p *provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent, cb types.Capability) error {
	if cb.Backend != types.BackendDocker {
		fmt.Errorf("hetzner only support docker backend")
	}

	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, nil)
	if err != nil {
		return fmt.Errorf("%s: cloudinit.RenderUserDataTemplate: %w", p.name, err)
	}

	sshKeys := make([]*hcloud.SSHKey, 0)
	for _, item := range p.sshKeys {
		key, _, err := p.client.SSHKey().GetByName(ctx, item)
		if err != nil {
			return fmt.Errorf("%s: SSHKey.GetByName: %w", p.name, err)
		}
		if key == nil {
			return fmt.Errorf("%s: %w: %s", p.name, ErrSSHKeyNotFound, item)
		}
		sshKeys = append(sshKeys, key)
	}

	networks := make([]*hcloud.Network, 0)
	for _, item := range p.networks {
		network, _, err := p.client.Network().GetByName(ctx, item)
		if err != nil {
			return fmt.Errorf("%s: Network.GetByName: %w", p.name, err)
		}
		if network == nil {
			return fmt.Errorf("%s: %w: %s", p.name, ErrNetworkNotFound, item)
		}
		networks = append(networks, network)
	}

	firewalls := make([]*hcloud.ServerCreateFirewall, 0)
	for _, item := range p.firewalls {
		fw, _, err := p.client.Firewall().GetByName(ctx, item)
		if err != nil {
			return fmt.Errorf("%s: Firewall.GetByName: %w", p.name, err)
		}
		if fw == nil {
			return fmt.Errorf("%s: %w: %s", p.name, ErrFirewallNotFound, item)
		}
		firewalls = append(firewalls, &hcloud.ServerCreateFirewall{Firewall: hcloud.Firewall{ID: fw.ID}})
	}

	serverCreateOpts := hcloud.ServerCreateOpts{
		Name:      agent.Name,
		UserData:  userData,
		SSHKeys:   sshKeys,
		Networks:  networks,
		Firewalls: firewalls,
		Labels:    p.labels,
		PublicNet: &hcloud.ServerCreatePublicNet{
			EnableIPv4: p.enableIPv4,
			EnableIPv6: p.enableIPv6,
		},
	}

	// First pass: resolve all configured entries and filter to those that
	// match the requested capability and whose location actually supports
	// the server type. We need this list up front so that we can correctly
	// distinguish "no last entry to error on" from "last viable entry
	// failed".
	candidates := make([]*deployCandidate, 0, len(p.deployCandidates))
	for _, c := range p.deployCandidates {
		// Filter by requested capability (arch). cap is always one of the
		// platforms we returned from Capabilities, so a mismatch here just
		// means this entry is for a different arch in the fallback chain.
		platform := "linux/" + hcloudArchToGoArch(c.serverType.Architecture)
		if platform == cb.Platform {
			candidates = append(candidates, c)
		}
	}

	if len(candidates) == 0 {
		return fmt.Errorf("%s: %w: %s", p.name, ErrNoMatchingServerType, cb.Platform)
	}

	for i, c := range candidates {
		serverCreateOpts.Location = c.location
		serverCreateOpts.ServerType = c.serverType
		serverCreateOpts.Image = c.image

		log.Info().Msgf("create agent: location = %s type = %s", c.location, c.serverType.Name)

		_, _, err = p.client.Server().Create(ctx, serverCreateOpts)
		if err == nil {
			return nil
		}

		// Continue to next fallback entry only if the resource is unavailable.
		if !hcloud.IsError(err, hcloud.ErrorCodeResourceUnavailable) {
			return fmt.Errorf("%s: Server.Create: %w", p.name, err)
		}

		// Only log and continue if there are more candidates left.
		if i < len(p.deployCandidates)-1 {
			log.Warn().Msgf(
				"create agent failed: location = %s type = %s: %s",
				c.location, c.serverType.Name, err,
			)
			continue
		}

		// Last candidate failed.
		return fmt.Errorf("%s: Server.Create: %w", p.name, err)
	}

	return nil
}

func (p *provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	server, err := p.getAgent(ctx, agent)
	if err != nil {
		return fmt.Errorf("%s: getAgent %w", p.name, err)
	}

	if server == nil {
		return nil
	}

	_, _, err = p.client.Server().DeleteWithResult(ctx, server)
	if err != nil {
		return fmt.Errorf("%s: Server.DeleteWithResults %w", p.name, err)
	}

	return nil
}

func (p *provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	var names []string

	servers, err := p.client.Server().AllWithOpts(ctx,
		hcloud.ServerListOpts{
			ListOpts: hcloud.ListOpts{LabelSelector: fmt.Sprintf("%s==%s", engine.LabelPool, p.config.PoolID)},
		})
	if err != nil {
		return nil, fmt.Errorf("%s: Server.AllWithOpts %w", p.name, err)
	}

	for _, server := range servers {
		names = append(names, server.Name)
	}

	return names, nil
}

func (p *provider) Capabilities(ctx context.Context) ([]types.Capability, error) {
	seen := map[string]bool{}
	var caps []types.Capability

	for _, c := range p.deployCandidates {
		platform := "linux/" + hcloudArchToGoArch(c.serverType.Architecture)
		if !seen[platform] {
			seen[platform] = true
			caps = append(caps, types.Capability{
				Platform: platform,
				Backend:  types.BackendDocker,
			})
		}
	}
	return caps, nil
}
