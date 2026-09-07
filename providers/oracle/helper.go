package oracle

import (
	"context"
	"fmt"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/autoscaler/utils"
)

// OCI freeform tag keys must not contain periods, so the engine labels
// (wp.autoscaler/...) are mapped to wp-autoscaler/... on this provider.
var (
	tagPrefix = strings.ReplaceAll(engine.LabelPrefix, ".", "-")
	tagPool   = strings.ReplaceAll(engine.LabelPool, ".", "-")
	tagImage  = strings.ReplaceAll(engine.LabelImage, ".", "-")
)

// newConfigurationProvider prefers API key credentials passed via flags and
// falls back to the OCI SDK config file otherwise.
func newConfigurationProvider(c *cli.Command) (common.ConfigurationProvider, error) {
	tenancyID := c.String("oracle-tenancy-id")
	userID := c.String("oracle-user-id")
	fingerprint := c.String("oracle-fingerprint")
	privateKey := c.String("oracle-private-key")
	region := c.String("oracle-region")

	if tenancyID != "" || userID != "" || fingerprint != "" || privateKey != "" {
		if tenancyID == "" || userID == "" || fingerprint == "" || privateKey == "" {
			return nil, ErrIncompleteCredentials
		}
		if region == "" {
			return nil, ErrRegionRequired
		}
		return common.NewRawConfigurationProvider(tenancyID, userID, region, fingerprint, privateKey, nil), nil
	}

	return common.CustomProfileConfigProvider(c.String("oracle-config-file"), c.String("oracle-profile")), nil
}

// parseFreeformTags parses key=value pairs and rejects keys that OCI does not
// accept or that collide with the tags managed by the autoscaler.
func parseFreeformTags(raw []string) (map[string]string, error) {
	tags, err := utils.SliceToMap(raw, "=")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidTag, err)
	}

	for key := range tags {
		if strings.ContainsAny(key, ". ") {
			return nil, fmt.Errorf("%w: key %q must not contain periods or spaces", ErrInvalidTag, key)
		}
		if strings.HasPrefix(key, tagPrefix) {
			return nil, fmt.Errorf("%w: %s", ErrReservedTagPrefix, tagPrefix)
		}
	}

	return tags, nil
}

// resolveImage returns the OCID of the newest available platform image that
// matches the operating system, version and shape.
func (p *provider) resolveImage(ctx context.Context, operatingSystem, version string) (string, error) {
	req := core.ListImagesRequest{
		CompartmentId:          &p.compartmentID,
		OperatingSystem:        &operatingSystem,
		OperatingSystemVersion: &version,
		Shape:                  &p.shape,
		LifecycleState:         core.ImageLifecycleStateAvailable,
		SortBy:                 core.ListImagesSortByTimecreated,
		SortOrder:              core.ListImagesSortOrderDesc,
		Limit:                  utils.ToPtr(1),
	}

	resp, err := p.client.ListImages(ctx, req)
	if err != nil {
		return "", fmt.Errorf("ListImages: %w", err)
	}
	if len(resp.Items) == 0 || resp.Items[0].Id == nil {
		return "", fmt.Errorf("%w: %s %s for shape %s", ErrImageNotFound, operatingSystem, version, p.shape)
	}

	return *resp.Items[0].Id, nil
}

// listPoolInstances returns all non-terminated instances of the pool,
// optionally filtered by display name.
func (p *provider) listPoolInstances(ctx context.Context, displayName string) ([]core.Instance, error) {
	var (
		instances []core.Instance
		page      *string
	)

	for {
		req := core.ListInstancesRequest{
			CompartmentId:      &p.compartmentID,
			AvailabilityDomain: &p.availabilityDomain,
			Limit:              utils.ToPtr(listPageSize),
			Page:               page,
		}
		if displayName != "" {
			req.DisplayName = &displayName
		}

		resp, err := p.client.ListInstances(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("ListInstances: %w", err)
		}

		for _, instance := range resp.Items {
			if instance.FreeformTags[tagPool] == p.config.PoolID && !isTerminated(instance.LifecycleState) {
				instances = append(instances, instance)
			}
		}

		if resp.OpcNextPage == nil || *resp.OpcNextPage == "" {
			return instances, nil
		}
		page = resp.OpcNextPage
	}
}

// isFlexShape reports whether the shape accepts a shape configuration
// (OCPUs and memory). Only flexible shapes do; they are suffixed with ".Flex".
func isFlexShape(shape string) bool {
	return strings.HasSuffix(strings.ToLower(shape), ".flex")
}

func isTerminated(state core.InstanceLifecycleStateEnum) bool {
	return state == core.InstanceLifecycleStateTerminating || state == core.InstanceLifecycleStateTerminated
}
