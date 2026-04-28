package hetznercloud

import (
	"context"
	"fmt"
	"strings"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/rs/zerolog/log"

	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

func (p *Provider) lookupServerType(ctx context.Context, name string) (*hcloud.ServerType, error) {
	serverType, _, err := p.client.ServerType().GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("%s: ServerType.GetByName: %w", name, err)
	}
	if serverType == nil {
		return nil, fmt.Errorf("%s: %w: %s", p.name, ErrServerTypeNotFound, name)
	}

	return serverType, nil
}

func (p *Provider) getAgent(ctx context.Context, agent *woodpecker.Agent) (*hcloud.Server, error) {
	server, _, err := p.client.Server().GetByName(ctx, agent.Name)
	if err != nil {
		return nil, fmt.Errorf("%s: Server.GetByName %w", p.name, err)
	}

	return server, nil
}

// parseServerTypeEntry splits a "<type>:<location>" entry, falling back to the
// deprecated p.location when no location is given.
//
// TODO: Deprecated location-fallback should be removed in v2.0.
func (p *Provider) parseServerTypeEntry(raw string) (rawType, location string) {
	rawType, location, _ = strings.Cut(raw, ":")
	if location == "" && p.location != "" {
		log.Error().Msg("hetznercloud-location is deprecated, please use hetznercloud-server-type instead")
		location = p.location
	}
	return rawType, location
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
