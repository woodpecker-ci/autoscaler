package aws

import (
	"context"
	b64 "encoding/base64"
	"errors"
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

var (
	ErrInstanceTypeNotFound = errors.New("instance type not found")
	ErrAMINotFound          = errors.New("AMI not found")
	ErrSubnetsNotSet        = errors.New("aws-subnets must be set")
	ErrArchMismatch         = errors.New("instance type architecture not supported by AMI")
	ErrTypeNotInRegion      = errors.New("instance type not offered in region")
	ErrNoDeployCandidates   = errors.New("no deploy candidates resolved")
)

// deployCandidate is one resolved instance-type/region pair the provider will
// try, in order, when deploying an agent. An empty region lets AWS decide.
type deployCandidate struct {
	instanceType ec2_types.InstanceTypeInfo
	region       string
}

type Provider struct {
	name                  string
	config                *config.Config
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
	deployCandidates []deployCandidate
	image            ec2_types.Image
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (types.Provider, error) {
	if len(c.StringSlice("aws-subnets")) == 0 {
		return nil, ErrSubnetsNotSet
	}
	p := &Provider{
		name:                  "aws",
		config:                config,
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

	// AMI must be resolved first: its architecture constrains which instance
	// types are valid deploy candidates.
	if err := p.resolveImage(ctx, c.String("aws-ami-id")); err != nil {
		return nil, err
	}
	if err := p.resolveDeployCandidates(ctx, c.StringSlice("aws-instance-type")); err != nil {
		return nil, err
	}

	p.printResolvedConfig()

	return p, nil
}

func (p *Provider) printResolvedConfig() {
	log.Info().
		Str("ami", aws.ToString(p.image.ImageId)).
		Str("ami_arch", string(p.image.Architecture)).
		Msg("resolved AMI")
	for _, c := range p.deployCandidates {
		region := c.region
		if region == "" {
			region = "<aws-decides>"
		}
		log.Info().
			Str("type", string(c.instanceType.InstanceType)).
			Str("region", region).
			Bool("current_gen", aws.ToBool(c.instanceType.CurrentGeneration)).
			Msg("deploy candidate")
	}
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, nil, cloudinit.RenderOption{})
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
		ImageId: p.image.ImageId,
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

	var result *ec2.RunInstancesOutput
	for i, c := range p.deployCandidates {
		runInstancesInput.InstanceType = c.instanceType.InstanceType

		// An empty region keeps the client's default region.
		var optFns []func(*ec2.Options)
		if c.region != "" {
			region := c.region
			optFns = append(optFns, func(o *ec2.Options) { o.Region = region })
		}

		log.Info().
			Str("type", string(c.instanceType.InstanceType)).
			Str("region", c.region).
			Msg("create agent")

		result, err = p.client.RunInstances(ctx, &runInstancesInput, optFns...)
		if err == nil {
			break
		}

		// Continue to next fallback entry only if capacity is unavailable.
		if !isInsufficientCapacity(err) {
			return fmt.Errorf("%s: RunInstances: %w", p.name, err)
		}

		// Only log and continue if there are more candidates left.
		if i < len(p.deployCandidates)-1 {
			log.Warn().Msgf(
				"create agent failed: type = %s region = %s: %s",
				c.instanceType.InstanceType, c.region, err,
			)
			continue
		}

		// Last candidate failed.
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

func (p *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	instance, err := p.getAgent(ctx, agent)
	if err != nil {
		return err
	}

	_, err = p.client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{*instance.InstanceId},
	})
	return err
}

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
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
