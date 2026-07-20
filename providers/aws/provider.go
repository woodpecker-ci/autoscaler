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
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2_types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/autoscaler/providers/aws/ec2api"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type provider struct {
	name                  string
	config                *config.Config
	tags                  []string
	region                string
	iamInstanceProfileArn string
	useSpotInstances      bool
	client                ec2api.Client
	lock                  sync.Mutex
	subnetRR              int
	sshKeyName            string
	// resolved config
	deployCandidates []deployCandidate
	regions          []string
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (types.Provider, error) {
	p := &provider{
		name:                  "aws",
		config:                config,
		tags:                  c.StringSlice("aws-tags"),
		region:                c.String("aws-region"),
		iamInstanceProfileArn: c.String("aws-iam-instance-profile-arn"),
		useSpotInstances:      c.Bool("aws-use-spot-instances"),
		sshKeyName:            c.String("aws-ssh-key-name"),
	}
	loadOptions := []func(*awsconfig.LoadOptions) error{}
	credentialsOption, err := staticCredentialsOption(
		c.String("aws-access-key-id"),
		c.String("aws-secret-access-key"),
	)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}
	if credentialsOption != nil {
		loadOptions = append(loadOptions, credentialsOption)
	}
	if p.region != "" {
		loadOptions = append(loadOptions, awsconfig.WithRegion(p.region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration, %w", err)
	}
	if p.region == "" {
		// Fall back to the region the SDK resolved from its default chain
		// (AWS_REGION, shared config, IMDS) so unqualified values keep
		// working without the aws-region flag.
		p.region = cfg.Region
	}
	instanceTypes := c.StringSlice("aws-instance-type")
	authenticationRegion, err := p.authenticationRegion(instanceTypes)
	if err != nil {
		return nil, err
	}
	identityClient := sts.NewFromConfig(cfg, func(options *sts.Options) {
		options.Region = authenticationRegion
	})
	if err := authenticateAWS(ctx, identityClient); err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}
	p.client = ec2.NewFromConfig(cfg)

	if err := p.resolveDeployCandidates(
		ctx,
		instanceTypes,
		c.String("aws-ami-id"),
		c.StringSlice("aws-subnets"),
		c.StringSlice("aws-security-groups"),
	); err != nil {
		return nil, err
	}

	p.printResolvedConfig()

	return p, nil
}

func (p *provider) authenticationRegion(instanceTypes []string) (string, error) {
	if p.region != "" {
		return p.region, nil
	}
	if len(instanceTypes) == 0 {
		return "", fmt.Errorf("%s: %w", p.name, ErrNoDeployCandidates)
	}

	_, region, err := p.valueRegion("aws-instance-type", instanceTypes[0])
	return region, err
}

type identityClient interface {
	GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

func authenticateAWS(ctx context.Context, client identityClient) error {
	identity, err := client.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("authenticate with AWS: %w", err)
	}

	log.Info().
		Str("account", aws.ToString(identity.Account)).
		Str("arn", aws.ToString(identity.Arn)).
		Msg("authenticated with AWS")
	return nil
}

func staticCredentialsOption(accessKeyID, secretAccessKey string) (func(*awsconfig.LoadOptions) error, error) {
	if accessKeyID == "" && secretAccessKey == "" {
		return nil, nil
	}
	if accessKeyID == "" {
		return nil, fmt.Errorf("aws-access-key-id must be set when aws-secret-access-key is set")
	}
	if secretAccessKey == "" {
		return nil, fmt.Errorf("aws-secret-access-key must be set when aws-access-key-id is set")
	}

	return awsconfig.WithCredentialsProvider(
		credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
	), nil
}

func (p *provider) printResolvedConfig() {
	for _, c := range p.deployCandidates {
		log.Info().
			Str("type", string(c.instanceType.InstanceType)).
			Str("region", c.regionConfig.region).
			Str("ami", aws.ToString(c.regionConfig.image.ImageId)).
			Str("ami_arch", string(c.regionConfig.image.Architecture)).
			Bool("current_gen", aws.ToBool(c.instanceType.CurrentGeneration)).
			Msg("deploy candidate")
	}
}

func (p *provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent, capability types.Capability) error {
	// Filter to the candidates that can serve the requested capability;
	// the capacity failover below only iterates within this set. Candidates
	// with an unknown architecture were not advertised and remain unusable,
	// but must not block known candidates.
	candidates := make([]deployCandidate, 0, len(p.deployCandidates))
	if capability.Backend == types.BackendDocker {
		for _, c := range p.deployCandidates {
			platform, err := candidatePlatform(c)
			if err != nil {
				log.Error().Err(err).Msg("skipping deploy candidate with unknown architecture")
				continue
			}
			if platform == capability.Platform {
				candidates = append(candidates, c)
			}
		}
	}
	if len(candidates) == 0 {
		return fmt.Errorf("%s: %w: platform=%s backend=%s",
			p.name, ErrNoMatchingCandidate, capability.Platform, capability.Backend)
	}

	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, cloudinit.RenderOption{})
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
		MetadataOptions: &ec2_types.InstanceMetadataOptionsRequest{
			HttpEndpoint:            ec2_types.InstanceMetadataEndpointStateEnabled,
			HttpPutResponseHopLimit: aws.Int32(1),
			HttpTokens:              ec2_types.HttpTokensStateRequired,
		},
		MinCount: aws.Int32(1),
		MaxCount: aws.Int32(1),
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
	for i, c := range candidates {
		runInstancesInput.InstanceType = c.instanceType.InstanceType
		runInstancesInput.ImageId = c.regionConfig.image.ImageId
		runInstancesInput.SecurityGroupIds = c.regionConfig.securityGroups

		// When multiple subnets are given, assign agent to a subnet in a round-robin fashion.
		p.lock.Lock()
		runInstancesInput.SubnetId = aws.String(c.regionConfig.subnets[p.subnetRR%len(c.regionConfig.subnets)])
		p.subnetRR = (p.subnetRR + 1) % len(c.regionConfig.subnets)
		p.lock.Unlock()

		log.Info().
			Str("type", string(c.instanceType.InstanceType)).
			Str("region", c.regionConfig.region).
			Msg("create agent")

		result, err = p.client.RunInstances(ctx, &runInstancesInput, regionOpt(c.regionConfig.region))
		if err == nil {
			break
		}

		// Continue to next fallback entry only if capacity is unavailable.
		if !isCapacityError(err) {
			return fmt.Errorf("%s: RunInstances: %w", p.name, err)
		}

		// Only log and continue if there are more candidates left.
		if i < len(candidates)-1 {
			log.Warn().Msgf(
				"create agent failed: type = %s region = %s: %s",
				c.instanceType.InstanceType, c.regionConfig.region, err,
			)
			continue
		}

		// Last candidate failed: the whole fallback chain is exhausted.
		return fmt.Errorf("%s: all %d deploy candidates out of capacity, last: RunInstances: %w",
			p.name, len(candidates), err)
	}

	if result == nil || len(result.Instances) == 0 {
		return fmt.Errorf("%s: RunInstances returned no instances", p.name)
	}

	// Wait until instance is available. Sometimes it can take a second or two for the tag based
	// filter to show the instance we just created in AWS
	log.Debug().Msgf("waiting for instance %s", *result.Instances[0].InstanceId)
	for range 5 {
		agents, err := p.ListDeployedAgentNames(ctx)
		if err != nil {
			return fmt.Errorf("%s: ListDeployedAgentNames: %w", p.name, err)
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

func (p *provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	instance, region, err := p.getAgent(ctx, agent)
	if err != nil {
		return err
	}

	_, err = p.client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{*instance.InstanceId},
		// Skip the graceful OS shutdown so an unresponsive or hung guest
		// cannot block termination and leave a dangling instance behind.
		SkipOsShutdown: aws.Bool(true),
	}, regionOpt(region))
	return err
}

func (p *provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	log.Debug().Msgf("list deployed agent names")

	var names []string
	for _, region := range p.regions {
		instances, err := p.instancesByTag(ctx, region, engine.LabelPool, p.config.PoolID)
		if err != nil {
			return nil, err
		}
		for _, instance := range instances {
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

func (p *provider) Capabilities(_ context.Context) ([]types.Capability, error) {
	seen := map[string]bool{}
	var caps []types.Capability
	var errs error

	for _, c := range p.deployCandidates {
		platform, err := candidatePlatform(c)
		if err != nil {
			// Discovery must not take the autoscaler down over one candidate
			// with an architecture the mapping does not know; report it and
			// keep the remaining candidates usable.
			log.Error().Err(err).Msg("skipping deploy candidate with unknown architecture")
			errs = errors.Join(errs, err)
			continue
		}
		if !seen[platform] {
			seen[platform] = true
			caps = append(caps, types.Capability{
				Platform: platform,
				Backend:  types.BackendDocker,
			})
		}
	}
	if len(caps) == 0 && errs != nil {
		return nil, errs
	}
	return caps, nil
}

func (p *provider) BillingModel() types.BillingModel {
	return types.BillingPerSecond
}
