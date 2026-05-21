package scaleway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// resolveCandidates resolves each "type:zone" entry from --scaleway-server-types.
// For each entry it fetches the ServerType (to get arch, ncpus, ram) and then
// finds the first image from --scaleway-images that resolves for that arch in
// that zone via ListImages. Entries that don't resolve are skipped with a
// warning. Hard error if no candidates survive.
func (p *provider) resolveCandidates(ctx context.Context, rawEntries []string) error {
	api := instance.NewAPI(p.client)

	for _, raw := range rawEntries {
		rawType, rawZone, _ := strings.Cut(raw, ":")
		if rawZone == "" {
			log.Error().Str("entry", raw).Msg("scaleway: server type entry missing zone (expected type:zone), skipping")
			continue
		}

		zone := scw.Zone(rawZone)
		if !zone.Exists() {
			return fmt.Errorf("%w: %s (from entry %q)", ErrInvalidZone, rawZone, raw)
		}

		resp, err := api.ListServersTypes(&instance.ListServersTypesRequest{
			Zone: zone,
		}, scw.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("scaleway: ListServersTypes zone=%s: %w", zone, err)
		}

		st, ok := resp.Servers[rawType]
		if !ok {
			log.Warn().Str("type", rawType).Str("zone", rawZone).Msg("scaleway: server type not found in zone, skipping")
			continue
		}
		if st.EndOfService {
			log.Warn().Str("type", rawType).Str("zone", rawZone).Msg("scaleway: server type is end-of-service, skipping")
			continue
		}

		archStr := string(st.Arch)
		imageID, imageName, err := p.resolveImage(ctx, api, zone, archStr)
		if err != nil {
			log.Warn().Str("type", rawType).Str("zone", rawZone).Str("arch", archStr).
				Msgf("scaleway: no image resolved (%s), skipping entry", err)
			continue
		}

		log.Info().
			Str("type", rawType).
			Str("zone", rawZone).
			Str("arch", archStr).
			Str("image", imageName).
			Str("image_id", imageID).
			Uint32("ncpus", st.Ncpus).
			Uint64("ram_bytes", st.RAM).
			Msg("scaleway: resolved deploy candidate")

		p.candidates = append(p.candidates, deployCandidate{
			rawType:    rawType,
			zone:       zone,
			serverType: st,
			imageID:    imageID,
			imageName:  imageName,
		})
	}

	if len(p.candidates) == 0 {
		return fmt.Errorf("scaleway: no valid deploy candidates after resolving --scaleway-server-types")
	}

	return nil
}

// resolveImage tries each name in p.images in order and returns the first one
// that resolves via ListImages for the given arch in the given zone.
func (p *provider) resolveImage(ctx context.Context, api *instance.API, zone scw.Zone, arch string) (imageID, imageName string, err error) {
	for _, name := range p.images {
		resp, err := api.ListImages(&instance.ListImagesRequest{
			Zone:   zone,
			Name:   scw.StringPtr(name),
			Arch:   scw.StringPtr(arch),
			Public: scw.BoolPtr(true),
		}, scw.WithContext(ctx))
		if err != nil {
			return "", "", fmt.Errorf("ListImages name=%s arch=%s zone=%s: %w", name, arch, zone, err)
		}
		if len(resp.Images) > 0 {
			img := resp.Images[0]
			return img.ID, img.Name, nil
		}
	}
	return "", "", fmt.Errorf("%w: tried %v for arch=%s zone=%s", ErrImageNotFound, p.images, arch, zone)
}

// getInstance looks up a single managed instance by name across all Scaleway
// zones. Returns ErrInstanceNotFound if no matching instance exists.
func (p *provider) getInstance(ctx context.Context, name string) (*instance.Server, error) {
	api := instance.NewAPI(p.client)
	for _, zone := range scw.AllZones {
		resp, err := api.ListServers(&instance.ListServersRequest{
			Zone:    zone,
			Project: p.projectID,
			Name:    scw.StringPtr(name),
			Tags:    p.tags,
		}, scw.WithContext(ctx))
		if err != nil {
			return nil, err
		}
		if resp.TotalCount > 0 {
			if resp.TotalCount > 1 {
				log.Warn().Msg("scaleway: found multiple instances with the same name, this may indicate orphaned resources")
			}
			return resp.Servers[0], nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrInstanceNotFound, name)
}

// getAllInstances returns every managed instance across all Scaleway zones.
func (p *provider) getAllInstances(ctx context.Context) ([]*instance.Server, error) {
	api := instance.NewAPI(p.client)
	var instances []*instance.Server
	for _, zone := range scw.AllZones {
		resp, err := api.ListServers(&instance.ListServersRequest{
			Zone:    zone,
			Project: p.projectID,
			Tags:    p.tags,
			PerPage: scw.Uint32Ptr(100), //nolint:mnd
		}, scw.WithContext(ctx), scw.WithAllPages())
		if err != nil {
			return nil, err
		}
		if resp.TotalCount > 0 {
			instances = append(instances, resp.Servers...)
		}
	}
	return instances, nil
}

// isResourceUnavailable reports whether the error indicates the requested
// server type has no capacity in the zone (soft error, try next candidate).
func isResourceUnavailable(err error) bool {
	var scwErr *scw.ResponseError
	if errors.As(err, &scwErr) {
		return scwErr.Message == "server_type_unavailable" || scwErr.StatusCode == http.StatusServiceUnavailable
	}
	return false
}
