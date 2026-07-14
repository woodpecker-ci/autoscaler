package aws

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2_types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"

	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// resolveDeployCandidates resolves the ordered instance type fallbacks. An
// instance type has the form value or value:region; unqualified values use
// aws-region. The AMI reference is resolved separately in each candidate's
// region, while subnets and security groups remain region-qualified resources.
func (p *provider) resolveDeployCandidates(ctx context.Context, instanceTypes []string, image string, subnets, securityGroups []string) error {
	if len(instanceTypes) == 0 {
		return fmt.Errorf("%s: %w", p.name, ErrNoDeployCandidates)
	}

	subnetsByRegion, err := p.valuesByRegion("aws-subnets", subnets)
	if err != nil {
		return err
	}
	securityGroupsByRegion, err := p.valuesByRegion("aws-security-groups", securityGroups)
	if err != nil {
		return err
	}

	type regionConfigKey struct {
		region       string
		architecture ec2_types.ArchitectureValues
	}
	configs := map[regionConfigKey]regionConfig{}
	imageValidated := false
	for _, raw := range instanceTypes {
		instanceType, region, err := p.valueRegion("aws-instance-type", raw)
		if err != nil {
			return err
		}

		it, err := p.resolveInstanceType(ctx, instanceType, region)
		if err != nil {
			return err
		}
		if !imageValidated {
			if err := validateImageReference(image); err != nil {
				return fmt.Errorf("%s: %w", p.name, err)
			}
			imageValidated = true
		}

		architecture, err := instanceTypeArchitecture(it)
		if err != nil {
			return fmt.Errorf("%s: %w", p.name, err)
		}
		key := regionConfigKey{region: region, architecture: architecture}
		config, ok := configs[key]
		if !ok {
			config, err = p.resolveRegionConfig(ctx, region, image, architecture, subnetsByRegion[region], securityGroupsByRegion[region])
			if err != nil {
				return err
			}
			configs[key] = config
		}

		if !instanceTypeSupportsArch(it, config.image.Architecture) {
			return fmt.Errorf("%s: %w: %s needs one of %v, AMI is %s",
				p.name, ErrArchMismatch, it.InstanceType,
				it.ProcessorInfo.SupportedArchitectures, config.image.Architecture)
		}

		p.deployCandidates = append(p.deployCandidates, deployCandidate{
			instanceType: it,
			regionConfig: config,
		})
		if !slices.Contains(p.regions, region) {
			p.regions = append(p.regions, region)
		}
	}

	return nil
}

// valuesByRegion groups a flag's value or value:region entries by their
// effective region.
func (p *provider) valuesByRegion(option string, values []string) (map[string][]string, error) {
	byRegion := make(map[string][]string, len(values))
	for _, raw := range values {
		value, region, err := p.valueRegion(option, raw)
		if err != nil {
			return nil, err
		}
		byRegion[region] = append(byRegion[region], value)
	}
	return byRegion, nil
}

func (p *provider) valueRegion(option, raw string) (string, string, error) {
	value, region, qualified := strings.Cut(raw, ":")
	if !qualified {
		region = p.region
	}
	if value == "" {
		return "", "", fmt.Errorf("%s: empty %s value", p.name, option)
	}
	if region == "" {
		return "", "", fmt.Errorf("%s: %w: %s", p.name, ErrRegionNotSet, option)
	}
	return value, region, nil
}

func (p *provider) resolveRegionConfig(ctx context.Context, region, image string, architecture ec2_types.ArchitectureValues, subnetIDs, securityGroupIDs []string) (regionConfig, error) {
	if len(subnetIDs) == 0 {
		return regionConfig{}, fmt.Errorf("%s: %w: region %q", p.name, ErrSubnetsNotSet, region)
	}

	resolvedImage, err := p.resolveImage(ctx, image, region, architecture)
	if err != nil {
		return regionConfig{}, err
	}

	resolvedSubnets, err := p.client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		SubnetIds: subnetIDs,
	}, regionOpt(region))
	if err != nil {
		return regionConfig{}, fmt.Errorf("%s: DescribeSubnets %v in %q: %w", p.name, subnetIDs, region, err)
	}
	if len(resolvedSubnets.Subnets) != len(subnetIDs) {
		return regionConfig{}, fmt.Errorf("%s: %w: got %d of %d subnets in %q",
			p.name, ErrSubnetNotFound, len(resolvedSubnets.Subnets), len(subnetIDs), region)
	}

	if len(securityGroupIDs) > 0 {
		groups, err := p.client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
			GroupIds: securityGroupIDs,
		}, regionOpt(region))
		if err != nil {
			return regionConfig{}, fmt.Errorf("%s: DescribeSecurityGroups %v in %q: %w",
				p.name, securityGroupIDs, region, err)
		}
		if len(groups.SecurityGroups) != len(securityGroupIDs) {
			return regionConfig{}, fmt.Errorf("%s: %w: got %d of %d security groups in %q",
				p.name, ErrSecurityGroupNotFound, len(groups.SecurityGroups), len(securityGroupIDs), region)
		}
	}

	return regionConfig{
		region:         region,
		image:          resolvedImage,
		subnets:        subnetIDs,
		securityGroups: securityGroupIDs,
	}, nil
}

func regionOpt(region string) func(*ec2.Options) {
	return func(o *ec2.Options) {
		o.Region = region
	}
}

func (p *provider) resolveInstanceType(ctx context.Context, instanceType, region string) (ec2_types.InstanceTypeInfo, error) {
	out, err := p.client.DescribeInstanceTypes(ctx, &ec2.DescribeInstanceTypesInput{
		InstanceTypes: []ec2_types.InstanceType{ec2_types.InstanceType(instanceType)},
	}, regionOpt(region))
	if err != nil {
		// DescribeInstanceTypes only knows types offered in the queried
		// region and rejects others with InvalidInstanceType.
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "InvalidInstanceType" {
			return ec2_types.InstanceTypeInfo{}, fmt.Errorf("%s: %w: %s in %q", p.name, ErrTypeNotInRegion, instanceType, region)
		}
		return ec2_types.InstanceTypeInfo{}, fmt.Errorf("%s: DescribeInstanceTypes %q in %q: %w", p.name, instanceType, region, err)
	}
	if len(out.InstanceTypes) == 0 {
		return ec2_types.InstanceTypeInfo{}, fmt.Errorf("%s: %w: %s in %q", p.name, ErrInstanceTypeNotFound, instanceType, region)
	}
	resolved := out.InstanceTypes[0]
	var architectures []ec2_types.ArchitectureType
	if resolved.ProcessorInfo != nil {
		architectures = resolved.ProcessorInfo.SupportedArchitectures
	}
	if len(architectures) != 1 {
		return ec2_types.InstanceTypeInfo{}, fmt.Errorf(
			"%s: %w: %s in %q reports %v; only instance types with exactly one architecture are supported",
			p.name, ErrInstanceTypeArchitecture, instanceType, region, architectures,
		)
	}
	return resolved, nil
}

func instanceTypeArchitecture(instanceType ec2_types.InstanceTypeInfo) (ec2_types.ArchitectureValues, error) {
	if instanceType.ProcessorInfo == nil || len(instanceType.ProcessorInfo.SupportedArchitectures) != 1 {
		return "", fmt.Errorf("%w: %s", ErrInstanceTypeArchitecture, instanceType.InstanceType)
	}
	return ec2_types.ArchitectureValues(instanceType.ProcessorInfo.SupportedArchitectures[0]), nil
}

func instanceTypeSupportsArch(it ec2_types.InstanceTypeInfo, arch ec2_types.ArchitectureValues) bool {
	if it.ProcessorInfo == nil {
		return false
	}
	for _, a := range it.ProcessorInfo.SupportedArchitectures {
		if string(a) == string(arch) {
			return true
		}
	}
	return false
}

// instancesByTag returns all pending or running instances in the given region
// that carry the tag, with the reservation layer flattened away. Terminated
// instances keep their tags for a while and must not show up as agents.
func (p *provider) instancesByTag(ctx context.Context, region, tag, value string) ([]ec2_types.Instance, error) {
	out, err := p.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []ec2_types.Filter{
			{Name: aws.String("tag:" + tag), Values: []string{value}},
			{Name: aws.String("instance-state-name"), Values: []string{
				string(ec2_types.InstanceStateNamePending),
				string(ec2_types.InstanceStateNameRunning),
			}},
		},
	}, regionOpt(region))
	if err != nil {
		return nil, err
	}
	var instances []ec2_types.Instance
	for _, r := range out.Reservations {
		instances = append(instances, r.Instances...)
	}
	return instances, nil
}

// getAgent finds the agent's instance and the region it runs in.
func (p *provider) getAgent(ctx context.Context, agent *woodpecker.Agent) (*ec2_types.Instance, string, error) {
	for _, region := range p.regions {
		instances, err := p.instancesByTag(ctx, region, "Name", agent.Name)
		if err != nil {
			return nil, "", err
		}
		if len(instances) > 1 {
			return nil, "", fmt.Errorf("expected 1 instance with tag:Name=%s, got %d", agent.Name, len(instances))
		}
		if len(instances) == 1 {
			return &instances[0], region, nil
		}
	}
	return nil, "", fmt.Errorf("no instance with tag:Name=%s in any deploy region", agent.Name)
}

// capacityErrorCodes are the RunInstances error codes that mean the requested
// capacity is not available right now, for which deploying the next fallback
// candidate is worthwhile.
// See https://docs.aws.amazon.com/AWSEC2/latest/APIReference/errors-overview.html
var capacityErrorCodes = map[string]bool{
	"InsufficientInstanceCapacity":        true,
	"Server.InsufficientInstanceCapacity": true,
	// The instance type is not supported in the requested availability zone.
	"Unsupported": true,
	// Spot-specific unavailability.
	"MaxSpotInstanceCountExceeded": true,
	"SpotMaxPriceTooLow":           true,
	// Not enough spare capacity to fulfill the Spot request right now.
	"UnfulfillableCapacity": true,
}

// isCapacityError reports whether err is an AWS capacity error, for which
// deploying the next fallback candidate is worthwhile.
func isCapacityError(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return capacityErrorCodes[apiErr.ErrorCode()]
	}
	return false
}
