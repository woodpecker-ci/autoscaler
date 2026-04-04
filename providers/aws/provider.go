package aws

import (
	"context"
	b64 "encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type Provider struct {
	name                  string
	config                *config.Config
	instanceType          string
	fallbackInstanceTypes []string
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
	userDataTemplate      *template.Template
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (engine.Provider, error) {
	if len(c.StringSlice("aws-subnets")) == 0 {
		return nil, fmt.Errorf("aws-subnets must be set")
	}
	p := &Provider{
		name:                  "aws",
		config:                config,
		instanceType:          c.String("aws-instance-type"),
		fallbackInstanceTypes: c.StringSlice("aws-fallback-instance-types"),
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

	// # TODO: Deprecated remove in v2.0
	if u := c.String("aws-user-data"); u != "" {
		log.Warn().Msg("aws-user-data is deprecated, please use provider-user-data instead")
		userDataTmpl, err := template.New("user-data").Parse(u)
		if err != nil {
			return nil, fmt.Errorf("%s: template.New.Parse %w", p.name, err)
		}
		p.userDataTemplate = userDataTmpl
	}

	return p, nil
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := engine.RenderUserDataTemplate(p.config, agent, p.userDataTemplate)
	if err != nil {
		return fmt.Errorf("%s: engine.RenderUserDataTemplate: %w", p.name, err)
	}

	// Generate base tags for instance
	tags := []types.Tag{{
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
		var rt types.Tag
		if len(parts) >= tagKVParts {
			rt = types.Tag{
				Key:   aws.String(parts[0]),
				Value: aws.String(parts[1]),
			}
		} else {
			rt = types.Tag{
				Key: aws.String(parts[0]),
			}
		}

		tags = append(tags, rt)
	}

	runInstancesInput := ec2.RunInstancesInput{
		IamInstanceProfile: &types.IamInstanceProfileSpecification{
			Arn: aws.String(p.iamInstanceProfileArn),
		},
		ImageId: aws.String(p.amiID),
		MetadataOptions: &types.InstanceMetadataOptionsRequest{
			HttpEndpoint:            types.InstanceMetadataEndpointStateEnabled,
			HttpPutResponseHopLimit: aws.Int32(1),
			HttpTokens:              types.HttpTokensStateRequired,
		},
		SecurityGroupIds: p.securityGroups,
		MinCount:         aws.Int32(1),
		MaxCount:         aws.Int32(1),
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: "instance",
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
		runInstancesInput.InstanceMarketOptions = &types.InstanceMarketOptionsRequest{
			MarketType: types.MarketTypeSpot,
		}
	}

	if p.sshKeyName != "" {
		runInstancesInput.KeyName = aws.String(p.sshKeyName)
	}

	runInstancesInput.UserData = aws.String(b64.StdEncoding.EncodeToString([]byte(userData)))

	// Try primary instance type, then fallback types on capacity errors
	instanceTypes := []string{p.instanceType}
	instanceTypes = append(instanceTypes, p.fallbackInstanceTypes...)

	var result *ec2.RunInstancesOutput
	for i, instanceType := range instanceTypes {
		runInstancesInput.InstanceType = types.InstanceType(instanceType)

		if i > 0 {
			log.Info().Msgf("retrying with fallback instance type %s (%d/%d)", instanceType, i, len(instanceTypes)-1)
		}

		result, err = p.client.RunInstances(ctx, &runInstancesInput)
		if err == nil {
			if i > 0 {
				log.Info().Msgf("successfully launched instance with fallback type %s", instanceType)
			}
			break
		}

		if !isCapacityError(err) || i == len(instanceTypes)-1 {
			return fmt.Errorf("%s: RunInstances: %w", p.name, err)
		}

		log.Warn().Msgf("instance type %s failed with capacity error: %v", instanceType, err)
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

func (p *Provider) getAgent(ctx context.Context, agent *woodpecker.Agent) (*types.Instance, error) {
	instances, err := p.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []types.Filter{
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
		Filters: []types.Filter{
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
			if instance.State.Name != types.InstanceStateNamePending &&
				instance.State.Name != types.InstanceStateNameRunning {
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

// isCapacityError checks if an AWS error is related to instance capacity issues
// that could be resolved by trying a different instance type.
func isCapacityError(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}

	switch apiErr.ErrorCode() {
	case "InsufficientInstanceCapacity",
		"InstanceLimitExceeded",
		"Unsupported":
		return true
	default:
		return false
	}
}
