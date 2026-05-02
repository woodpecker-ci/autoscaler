package aws

import (
	"context"
	b64 "encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2_types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var ErrInstanceTypeNotFound = fmt.Errorf("instance type not found")

type provider struct {
	name                  string
	config                *config.Config
	amiID                 string
	tags                  []string
	region                string
	subnets               []string
	securityGroups        []string
	iamInstanceProfileArn string
	useSpotInstances      bool
	client                *ec2.Client
	lock                  sync.Mutex
	subnetRR              int
	sshKeyName            string
	// resolved config
	instanceType ec2_types.InstanceTypeInfo
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (types.Provider, error) {
	if len(c.StringSlice("aws-subnets")) == 0 {
		return nil, fmt.Errorf("aws-subnets must be set")
	}
	p := &provider{
		name:                  "aws",
		config:                config,
		amiID:                 c.String("aws-ami-id"),
		tags:                  c.StringSlice("aws-tags"),
		region:                c.String("aws-region"),
		subnets:               c.StringSlice("aws-subnets"),
		iamInstanceProfileArn: c.String("aws-iam-instance-profile-arn"),
		securityGroups:        c.StringSlice("aws-security-groups"),
		useSpotInstances:      c.Bool("aws-use-spot-instances"),
		sshKeyName:            c.String("aws-ssh-key-name"),
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(p.region))
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration, %w", err)
	}
	p.client = ec2.NewFromConfig(cfg)

	if err := p.resolveInstanceType(ctx, c.String("aws-instance-type")); err != nil {
		return nil, err
	}

	p.printResolvedConfig()

	return p, nil
}

func (p *provider) resolveInstanceType(ctx context.Context, instanceType string) error {
	out, err := p.client.DescribeInstanceTypes(ctx, &ec2.DescribeInstanceTypesInput{
		InstanceTypes: []ec2_types.InstanceType{ec2_types.InstanceType(instanceType)},
	})
	if err != nil {
		return fmt.Errorf("%s: DescribeInstanceTypes %q: %w", p.name, instanceType, err)
	}
	if len(out.InstanceTypes) == 0 {
		return fmt.Errorf("%w: %s", ErrInstanceTypeNotFound, instanceType)
	}
	p.instanceType = out.InstanceTypes[0]
	return nil
}

func (p *provider) printResolvedConfig() {
	archs := make([]string, 0, len(p.instanceType.ProcessorInfo.SupportedArchitectures))
	for _, a := range p.instanceType.ProcessorInfo.SupportedArchitectures {
		archs = append(archs, string(a))
	}
	log.Info().
		Str("type", string(p.instanceType.InstanceType)).
		Strs("architectures", archs).
		Bool("current_gen", aws.ToBool(p.instanceType.CurrentGeneration)).
		Msg("deploy with instance type")
}

func (p *provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent, cb types.Capability) error {
	if err := p.validateCapability(cb); err != nil {
		return err
	}

	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, nil)
	if err != nil {
		return fmt.Errorf("%s: cloudinit.RenderUserDataTemplate: %w", p.name, err)
	}

	// Generate base tags for instance
	tags := []ec2_types.Tag{{
		Key:   aws.String("Name"),
		Value: aws.String(agent.Name),
	}, {
		Key:   aws.String(engine.LabelPool),
		Value: aws.String(p.config.PoolID),
	}}

	// Append user specified tags
	tagKVParts := 2
	for _, tag := range p.tags {
		parts := strings.Split(tag, "=")
		var rt ec2_types.Tag
		if len(parts) >= tagKVParts {
			rt = ec2_types.Tag{
				Key:   aws.String(parts[0]),
				Value: aws.String(parts[1]),
			}
		} else {
			rt = ec2_types.Tag{
				Key: aws.String(parts[0]),
			}
		}
		tags = append(tags, rt)
	}

	runInstancesInput := ec2.RunInstancesInput{
		IamInstanceProfile: &ec2_types.IamInstanceProfileSpecification{
			Arn: aws.String(p.iamInstanceProfileArn),
		},
		ImageId:      aws.String(p.amiID),
		InstanceType: p.instanceType.InstanceType,
		MetadataOptions: &ec2_types.InstanceMetadataOptionsRequest{
			HttpEndpoint:            ec2_types.InstanceMetadataEndpointStateEnabled,
			HttpPutResponseHopLimit: aws.Int32(1),
			HttpTokens:              ec2_types.HttpTokensStateRequired,
		},
		SecurityGroupIds: p.securityGroups,
		MinCount:         aws.Int32(1),
		MaxCount:         aws.Int32(1),
		TagSpecifications: []ec2_types.TagSpecification{
			{
				ResourceType: "instance",
				Tags:         tags,
			},
			{
				ResourceType: "volume",
				Tags:         tags,
			},
		},
	}

	// When multiple subnets are given, assign agent to a subnet in a round-robin fashion.
	p.lock.Lock()
	runInstancesInput.SubnetId = aws.String(p.subnets[p.subnetRR])
	p.subnetRR = (p.subnetRR + 1) % len(p.subnets)
	p.lock.Unlock()

	if p.useSpotInstances {
		runInstancesInput.InstanceMarketOptions = &ec2_types.InstanceMarketOptionsRequest{
			MarketType: ec2_types.MarketTypeSpot,
		}
	}

	if p.sshKeyName != "" {
		runInstancesInput.KeyName = aws.String(p.sshKeyName)
	}

	runInstancesInput.UserData = aws.String(b64.StdEncoding.EncodeToString([]byte(userData)))
	result, err := p.client.RunInstances(ctx, &runInstancesInput)
	if err != nil {
		return fmt.Errorf("%s: RunInstances: %w", p.name, err)
	}

	// Wait until instance is available. Sometimes it can take a second or two for the tag based
	// filter to show the instance we just created in AWS
	log.Debug().Msgf("waiting for instance %s", *result.Instances[0].InstanceId)
	for range 5 {
		agents, err := p.ListDeployedAgentNames(ctx)
		if err != nil {
			return fmt.Errorf("failed to return list for agents")
		}

		for _, a := range agents {
			if a == agent.Name {
				return nil
			}
		}

		log.Debug().Msgf("created agent not found in list yet")
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("instance did not resolve in agent list: %s", *result.Instances[0].InstanceId)
}

// validateCapability checks the requested (platform, backend) against the
// architectures reported by DescribeInstanceTypes for the configured type.
func (p *provider) validateCapability(cb types.Capability) error {
	for _, c := range p.capabilities() {
		if c.Platform == cb.Platform && c.Backend == cb.Backend {
			return nil
		}
	}
	return fmt.Errorf("%s: instance type %s does not support requested capability platform=%s backend=%s",
		p.name, p.instanceType.InstanceType, cb.Platform, cb.Backend)
}

func (p *provider) Capabilities(_ context.Context) ([]types.Capability, error) {
	return p.capabilities(), nil
}

// capabilities derives the supported (platform, backend) pairs from the
// resolved InstanceTypeInfo.ProcessorInfo.SupportedArchitectures.
func (p *provider) capabilities() []types.Capability {
	var caps []types.Capability
	for _, arch := range p.instanceType.ProcessorInfo.SupportedArchitectures {
		goarch := ec2ArchToGoArch(arch)
		if goarch == "" {
			continue
		}
		caps = append(caps, types.Capability{
			Platform: "linux/" + goarch,
			Backend:  types.BackendDocker,
		})
	}
	return caps
}

// ec2ArchToGoArch maps EC2 ArchitectureType values to Go GOARCH strings.
func ec2ArchToGoArch(a ec2_types.ArchitectureType) string {
	switch a {
	case ec2_types.ArchitectureTypeX8664:
		return "amd64"
	case ec2_types.ArchitectureTypeArm64:
		return "arm64"
	default:
		return ""
	}
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

func (p *provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	instance, err := p.getAgent(ctx, agent)
	if err != nil {
		return err
	}

	_, err = p.client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{*instance.InstanceId},
	})
	return err
}

func (p *provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	log.Debug().Msgf("list deployed agent names")

	var names []string
	instances, err := p.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []ec2_types.Filter{
			{
				Name:   aws.String(fmt.Sprintf("tag:%s", engine.LabelPool)),
				Values: []string{p.config.PoolID},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			if instance.State.Name != ec2_types.InstanceStateNamePending &&
				instance.State.Name != ec2_types.InstanceStateNameRunning {
				continue
			}
			for _, tag := range instance.Tags {
				if *tag.Key == "Name" {
					log.Debug().Msgf("found agent %s", *tag.Value)
					names = append(names, *tag.Value)
				}
			}
		}
	}
	return names, nil
}
