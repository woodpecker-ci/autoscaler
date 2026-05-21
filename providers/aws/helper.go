package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2_types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

func (p *Provider) resolveInstanceType(ctx context.Context, instanceType string) error {
	out, err := p.client.DescribeInstanceTypes(ctx, &ec2.DescribeInstanceTypesInput{
		InstanceTypes: []ec2_types.InstanceType{ec2_types.InstanceType(instanceType)},
	})
	if err != nil {
		return fmt.Errorf("%s: DescribeInstanceTypes %q: %w", p.name, instanceType, err)
	}
	if len(out.InstanceTypes) == 0 {
		return fmt.Errorf("%s: %w: %s", p.name, ErrInstanceTypeNotFound, instanceType)
	}
	p.instanceType = out.InstanceTypes[0]
	return nil
}

func (p *Provider) resolveImage(ctx context.Context, amiID string) error {
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

func (p *Provider) getAgent(ctx context.Context, agent *woodpecker.Agent) (*ec2_types.Instance, error) {
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
