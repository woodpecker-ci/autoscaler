package hetznercloud

import (
	"context"
	"fmt"
	"strings"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/rs/zerolog/log"

	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

func (p *provider) resolveServerConfigs(ctx context.Context, serverType []string, rawImage string) error {
	for _, raw := range serverType {
		rawType, rawLocation, _ := strings.Cut(raw, ":")

		serverType, _, err := p.client.ServerType().GetByName(ctx, rawType)
		if err != nil {
			return fmt.Errorf("%s: ServerType.GetByName: %w", rawType, err)
		}
		if serverType == nil {
			return fmt.Errorf("%s: %w: %s", p.name, ErrServerTypeNotFound, rawType)
		}
		if serverType.IsDeprecated() {
			log.Error().Msgf("server type %q is deprecated", serverType.Name)
		}

		location, err := resolveLocation(serverType, rawLocation)
		if err != nil {
			return err
		}

		image, _, err := p.client.Image().GetByNameAndArchitecture(ctx, rawImage, serverType.Architecture)
		if err != nil {
			return fmt.Errorf("%s: Image.GetByNameAndArchitecture: %w", p.name, err)
		}
		if image == nil {
			return fmt.Errorf("%s: %w: %s", p.name, ErrImageNotFound, rawImage)
		}

		p.deployCandidate = append(p.deployCandidate, deployCandidate{
			location:   location,
			serverType: serverType,
			image:      image,
		})
	}

	if len(p.deployCandidate) == 0 {
		return fmt.Errorf("no deploy candidates resolved")
	}

	return nil
}

func (p *provider) getAgent(ctx context.Context, agent *woodpecker.Agent) (*hcloud.Server, error) {
	server, _, err := p.client.Server().GetByName(ctx, agent.Name)
	if err != nil {
		return nil, fmt.Errorf("%s: Server.GetByName %w", p.name, err)
	}

	return server, nil
}

func resolveLocation(st *hcloud.ServerType, location string) (*hcloud.Location, error) {
	if location == "" {
		return nil, nil
	}

	for _, l := range st.Locations {
		if l.Location == nil || l.Location.Name != location {
			continue
		}
		// Filter out locations where this server type is being phased out.
		if l.IsDeprecated() {
			return nil, fmt.Errorf("%w: %s is deprecated", ErrImageNotSupported, location)
		}
		return l.Location, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrImageNotFound, location)
}
