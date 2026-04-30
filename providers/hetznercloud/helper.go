package hetznercloud

import (
	"context"
	"fmt"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"

	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

func (p *provider) lookupServerType(ctx context.Context, name string) (*hcloud.ServerType, error) {
	serverType, _, err := p.client.ServerType().GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("%s: ServerType.GetByName: %w", name, err)
	}
	if serverType == nil {
		return nil, fmt.Errorf("%s: %w: %s", p.name, ErrServerTypeNotFound, name)
	}

	return serverType, nil
}

func (p *provider) getAgent(ctx context.Context, agent *woodpecker.Agent) (*hcloud.Server, error) {
	server, _, err := p.client.Server().GetByName(ctx, agent.Name)
	if err != nil {
		return nil, fmt.Errorf("%s: Server.GetByName %w", p.name, err)
	}

	return server, nil
}

// serverTypeSupportsLocation reports whether the given server type is
// available in the given location and not deprecated there.
func serverTypeSupportsLocation(st *hcloud.ServerType, location string) bool {
	if location == "" {
		return true
	}

	for _, l := range st.Locations {
		if l.Location == nil || l.Location.Name != location {
			continue
		}
		// Filter out locations where this server type is being phased out.
		if l.IsDeprecated() {
			return false
		}
		return true
	}
	return false
}

// hcloudArchToGoArch maps hcloud architecture names to Go GOARCH strings.
func hcloudArchToGoArch(a hcloud.Architecture) string {
	switch a {
	case hcloud.ArchitectureARM:
		return "arm64"
	case hcloud.ArchitectureX86:
		return "amd64"
	default:
		return string(a)
	}
}
