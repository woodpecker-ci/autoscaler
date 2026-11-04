package openstack

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/v2/openstack/image/v2/images"
	"github.com/rs/zerolog/log"

	"go.woodpecker-ci.org/autoscaler/engine/types"
)

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
		// The property is optional and commonly unset on single-architecture
		// clouds, so the platform is left open: agents count for whatever
		// they report, but workflows that ask for a platform do not scale.
		log.Warn().Str("image", image.Name).Msg("OpenStack image has no hw_architecture property, workflows that set a platform label will not trigger scaling")
		return types.Capability{Backend: types.BackendDocker}, nil
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
	case "mipsel":
		return "mipsle"
	case "mips64el":
		return "mips64le"
	case "x86_64":
		return "amd64"
	default: // ppc64le, s390x, mips, ...
		return architecture
	}
}
