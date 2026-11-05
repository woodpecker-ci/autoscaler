package oracle

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/engine"
)

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func sanitizeTagKey(key string) string {
	return strings.ReplaceAll(key, ".", "-")
}

func tagKeyPrefix() string {
	return sanitizeTagKey(engine.LabelPrefix)
}

func poolTagKey() string {
	return sanitizeTagKey(engine.LabelPool)
}

func parseTags(raw []string) ([]string, map[string]string, error) {
	sanitized := make([]string, 0, len(raw))
	tags := make(map[string]string, len(raw))
	for _, entry := range raw {
		key, value, found := strings.Cut(entry, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return nil, nil, fmt.Errorf("%w: %q is not in \"key=value\" format", ErrInvalidTag, entry)
		}
		key = sanitizeTagKey(key)
		// Oracle Cloud only accepts printable ASCII without spaces in tag keys.
		if strings.ContainsFunc(key, func(r rune) bool { return r <= ' ' || r > '~' }) {
			return nil, nil, fmt.Errorf("%w: key %q may only hold printable ASCII without spaces", ErrInvalidTag, key)
		}
		sanitized = append(sanitized, key+"="+value)
		tags[key] = value
	}
	return sanitized, tags, nil
}

func newConfigurationProvider(c *cli.Command) (common.ConfigurationProvider, error) {
	if c.Bool("oracle-use-instance-principal") {
		if region := c.String("oracle-region"); region != "" {
			return auth.InstancePrincipalConfigurationProviderForRegion(common.StringToRegion(region))
		}
		return auth.InstancePrincipalConfigurationProvider()
	}

	tenancy := c.String("oracle-tenancy-ocid")
	user := c.String("oracle-user-ocid")
	region := c.String("oracle-region")
	fingerprint := c.String("oracle-fingerprint")
	privateKey := c.String("oracle-private-key")

	required := []struct {
		name  string
		value string
	}{
		{"WOODPECKER_ORACLE_TENANCY_OCID", tenancy},
		{"WOODPECKER_ORACLE_USER_OCID", user},
		{"WOODPECKER_ORACLE_REGION", region},
		{"WOODPECKER_ORACLE_FINGERPRINT", fingerprint},
		{"WOODPECKER_ORACLE_PRIVATE_KEY", privateKey},
	}

	missing := make([]string, 0, len(required))
	for _, entry := range required {
		if entry.value == "" {
			missing = append(missing, entry.name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%w: %s is missing", ErrIncompleteAPIKeyAuth, strings.Join(missing, ", "))
	}

	var passphrase *string
	if p := c.String("oracle-private-key-passphrase"); p != "" {
		passphrase = common.String(p)
	}

	return common.NewRawConfigurationProvider(tenancy, user, region, fingerprint, privateKey, passphrase), nil
}

func (p *provider) resolveAvailabilityDomains(ctx context.Context, configured []string) error {
	if len(configured) > 0 {
		p.availabilityDomains = configured
		return nil
	}

	res, err := p.identityClient.ListAvailabilityDomains(ctx, identity.ListAvailabilityDomainsRequest{
		CompartmentId: common.String(p.compartmentID),
	})
	if err != nil {
		return fmt.Errorf("%s: ListAvailabilityDomains: %w", p.name, err)
	}

	for _, ad := range res.Items {
		if ad.Name != nil {
			p.availabilityDomains = append(p.availabilityDomains, *ad.Name)
		}
	}
	if len(p.availabilityDomains) == 0 {
		return fmt.Errorf("%s: %w: %s", p.name, ErrNoAvailabilityDomains, p.compartmentID)
	}

	return nil
}

func (p *provider) resolveSubnet(ctx context.Context) error {
	res, err := p.networkClient.GetSubnet(ctx, core.GetSubnetRequest{SubnetId: common.String(p.subnetID)})
	if err != nil {
		return fmt.Errorf("%s: GetSubnet: %w", p.name, err)
	}

	if p.assignPublicIP && res.ProhibitPublicIpOnVnic != nil && *res.ProhibitPublicIpOnVnic {
		return fmt.Errorf("%s: %w: %s is private, set WOODPECKER_ORACLE_ASSIGN_PUBLIC_IP=false",
			p.name, ErrPublicIPProhibited, p.subnetID)
	}

	// An AD-specific subnet only launches in its own availability domain.
	if ad := stringValue(res.AvailabilityDomain); ad != "" {
		if !slices.Contains(p.availabilityDomains, ad) {
			return fmt.Errorf("%s: %w: %s is bound to %s", p.name, ErrSubnetAvailabilityDomain, p.subnetID, ad)
		}
		p.availabilityDomains = []string{ad}
	}

	return nil
}

func (p *provider) resolveShapes(ctx context.Context, names []string) error {
	if len(names) == 0 {
		return fmt.Errorf("%s: %w", p.name, ErrNoShapes)
	}

	known := map[string]shapeCandidate{}
	request := core.ListShapesRequest{
		CompartmentId: common.String(p.compartmentID),
		Limit:         common.Int(listPageLimit),
	}
	for {
		res, err := p.computeClient.ListShapes(ctx, request)
		if err != nil {
			return fmt.Errorf("%s: ListShapes: %w", p.name, err)
		}
		for _, shape := range res.Items {
			name := stringValue(shape.Shape)
			known[name] = shapeCandidate{
				name:     name,
				flexible: shape.IsFlexible != nil && *shape.IsFlexible,
				platform: "linux/" + shapeArch(stringValue(shape.ProcessorDescription)),
			}
		}
		if res.OpcNextPage == nil {
			break
		}
		request.Page = res.OpcNextPage
	}

	p.shapes = make([]shapeCandidate, 0, len(names))
	for _, name := range names {
		shape, found := known[name]
		if !found {
			return fmt.Errorf("%s: %w: %s", p.name, ErrShapeNotFound, name)
		}
		p.shapes = append(p.shapes, shape)
	}

	return nil
}

// shapeArch maps the processor description of an OCI shape to a Go architecture:
// Ampere (Altra, AmpereOne) and NVIDIA Grace are arm64, the rest x86.
func shapeArch(processor string) string {
	if strings.Contains(processor, "Ampere") || strings.Contains(processor, "Grace") {
		return "arm64"
	}
	return "amd64"
}

func (p *provider) resolveImages(ctx context.Context, imageID, operatingSystem, operatingSystemVersion string) error {
	for i := range p.shapes {
		shape := &p.shapes[i]
		if imageID != "" {
			shape.imageID = imageID
			continue
		}

		request := core.ListImagesRequest{
			CompartmentId:          common.String(p.compartmentID),
			OperatingSystem:        common.String(operatingSystem),
			OperatingSystemVersion: common.String(operatingSystemVersion),
			Shape:                  common.String(shape.name),
			LifecycleState:         core.ImageLifecycleStateAvailable,
			SortBy:                 core.ListImagesSortByTimecreated,
			SortOrder:              core.ListImagesSortOrderDesc,
			Limit:                  common.Int(1),
		}
		var image *core.Image
		for {
			res, err := p.computeClient.ListImages(ctx, request)
			if err != nil {
				return fmt.Errorf("%s: ListImages: %w", p.name, err)
			}
			// OCI may return an empty page while opc-next-page says more remain.
			if len(res.Items) > 0 {
				image = &res.Items[0]
				break
			}
			if res.OpcNextPage == nil {
				break
			}
			request.Page = res.OpcNextPage
		}
		if image == nil || image.Id == nil {
			return fmt.Errorf("%s: %w: %s %s for shape %s",
				p.name, ErrImageNotFound, operatingSystem, operatingSystemVersion, shape.name)
		}

		shape.imageID = *image.Id
		log.Info().
			Str("image", stringValue(image.DisplayName)).
			Str("image_id", shape.imageID).
			Str("shape", shape.name).
			Msgf("%s: resolved image", p.name)
	}

	return nil
}

func (p *provider) listInstances(ctx context.Context, displayName string) ([]core.Instance, error) {
	var instances []core.Instance
	request := core.ListInstancesRequest{
		CompartmentId: common.String(p.compartmentID),
		Limit:         common.Int(listPageLimit),
	}
	if displayName != "" {
		request.DisplayName = common.String(displayName)
	}

	for {
		res, err := p.computeClient.ListInstances(ctx, request)
		if err != nil {
			return nil, fmt.Errorf("%s: ListInstances: %w", p.name, err)
		}
		for _, inst := range res.Items {
			if !isAlive(inst) || !p.ownsInstance(inst) {
				continue
			}
			instances = append(instances, inst)
		}
		if res.OpcNextPage == nil {
			return instances, nil
		}
		request.Page = res.OpcNextPage
	}
}

func (p *provider) getInstance(ctx context.Context, name string) (*core.Instance, error) {
	instances, err := p.listInstances(ctx, name)
	if err != nil {
		return nil, err
	}
	if len(instances) == 0 {
		return nil, fmt.Errorf("%s: %w: %s", p.name, ErrInstanceNotFound, name)
	}
	if len(instances) > 1 {
		log.Warn().Str("agent", name).
			Msgf("%s: found multiple instances with the same name, this may indicate orphaned resources", p.name)
	}
	return &instances[0], nil
}

func (p *provider) ownsInstance(inst core.Instance) bool {
	return inst.FreeformTags[poolTagKey()] == p.config.PoolID
}

func isAlive(inst core.Instance) bool {
	switch inst.LifecycleState {
	case core.InstanceLifecycleStateTerminating, core.InstanceLifecycleStateTerminated:
		return false
	default:
		return inst.DisplayName != nil
	}
}

func isCapacityError(err error) bool {
	var svcErr common.ServiceError
	if !errors.As(err, &svcErr) {
		return false
	}
	for _, code := range capacityErrorCodes {
		if svcErr.GetCode() == code {
			return true
		}
	}
	return strings.Contains(strings.ToLower(svcErr.GetMessage()), "out of host capacity")
}
