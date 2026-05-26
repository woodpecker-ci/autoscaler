package aws

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2_types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"

	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// resolveDeployCandidates parses the aws-instance-type entries (each in
// "type" or "type:region" form) into ordered deploy candidates. An entry
// without a region lets AWS pick one at deploy time.
func (p *provider) resolveDeployCandidates(ctx context.Context, instanceTypes []string) error {
	for _, raw := range instanceTypes {
		rawType, region, _ := strings.Cut(raw, ":")

		it, err := p.resolveInstanceType(ctx, rawType)
		if err != nil {
			return err
		}

		// The AMI has a single architecture; an instance type whose
		// architectures don't include it can never boot this image.
		if !instanceTypeSupportsArch(it, p.image.Architecture) {
			return fmt.Errorf("%s: %w: %s needs one of %v, AMI is %s",
				p.name, ErrArchMismatch, it.InstanceType,
				it.ProcessorInfo.SupportedArchitectures, p.image.Architecture)
		}

		if region != "" {
			if err := p.checkTypeOfferedInRegion(ctx, rawType, region); err != nil {
				return err
			}
		}

		p.deployCandidates = append(p.deployCandidates, deployCandidate{
			instanceType: it,
			region:       region,
		})
	}

	if len(p.deployCandidates) == 0 {
		return fmt.Errorf("%s: %w", p.name, ErrNoDeployCandidates)
	}

	return nil
}

func (p *provider) resolveInstanceType(ctx context.Context, instanceType string) (ec2_types.InstanceTypeInfo, error) {
	out, err := p.client.DescribeInstanceTypes(ctx, &ec2.DescribeInstanceTypesInput{
		InstanceTypes: []ec2_types.InstanceType{ec2_types.InstanceType(instanceType)},
	})
	if err != nil {
		return ec2_types.InstanceTypeInfo{}, fmt.Errorf("%s: DescribeInstanceTypes %q: %w", p.name, instanceType, err)
	}
	if len(out.InstanceTypes) == 0 {
		return ec2_types.InstanceTypeInfo{}, fmt.Errorf("%s: %w: %s", p.name, ErrInstanceTypeNotFound, instanceType)
	}
	return out.InstanceTypes[0], nil
}

// checkTypeOfferedInRegion verifies the instance type is actually offered in
// the requested region, querying that region directly.
func (p *provider) checkTypeOfferedInRegion(ctx context.Context, instanceType, region string) error {
	out, err := p.client.DescribeInstanceTypeOfferings(ctx, &ec2.DescribeInstanceTypeOfferingsInput{
		LocationType: ec2_types.LocationTypeRegion,
		Filters: []ec2_types.Filter{
			{Name: aws.String("instance-type"), Values: []string{instanceType}},
			{Name: aws.String("location"), Values: []string{region}},
		},
	}, func(o *ec2.Options) { o.Region = region })
	if err != nil {
		return fmt.Errorf("%s: DescribeInstanceTypeOfferings %q: %w", p.name, instanceType, err)
	}
	if len(out.InstanceTypeOfferings) == 0 {
		return fmt.Errorf("%s: %w: %s in %s", p.name, ErrTypeNotInRegion, instanceType, region)
	}
	return nil
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

func (p *provider) resolveImage(ctx context.Context, amiID string) error {
	out, err := p.client.DescribeImages(ctx, &ec2.DescribeImagesInput{
		ImageIds: []string{amiID},
	})
	if err != nil {
		return fmt.Errorf("%s: DescribeImages %q: %w", p.name, amiID, err)
	}
	if len(out.Images) == 0 {
		return fmt.Errorf("%s: %w: %s", p.name, ErrAMINotFound, amiID)
	}
	p.image = out.Images[0]
	return nil
}

func (p *provider) getAgent(ctx context.Context, agent *woodpecker.Agent) (*ec2_types.Instance, error) {
	instances, err := p.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []ec2_types.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []string{agent.Name},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(instances.Reservations) != 1 {
		return nil, fmt.Errorf("expected 1 reservation with tag:Name=%s, got %d", agent.Name, len(instances.Reservations))
	}
	if len(instances.Reservations[0].Instances) != 1 {
		return nil, fmt.Errorf("expected 1 instance with tag:Name=%s, got %d", agent.Name, len(instances.Reservations[0].Instances))
	}
	return &instances.Reservations[0].Instances[0], nil
}

// isInsufficientCapacity reports whether err is an AWS capacity error, for
// which deploying the next fallback candidate is worthwhile.
func isInsufficientCapacity(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "InsufficientInstanceCapacity"
	}
	return false
}
