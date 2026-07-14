package aws

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2_types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/autoscaler/providers/aws/ec2api/mocks"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var (
	testImage = ec2_types.Image{
		ImageId:      aws.String("ami-x86"),
		Architecture: ec2_types.ArchitectureValuesX8664,
	}
	testArmImage = ec2_types.Image{
		ImageId:      aws.String("ami-arm"),
		Architecture: ec2_types.ArchitectureValuesArm64,
	}
	testTypeInfo = ec2_types.InstanceTypeInfo{
		InstanceType:      ec2_types.InstanceType("m6i.large"),
		CurrentGeneration: aws.Bool(true),
		ProcessorInfo: &ec2_types.ProcessorInfo{
			SupportedArchitectures: []ec2_types.ArchitectureType{ec2_types.ArchitectureTypeX8664},
		},
	}
	testArmTypeInfo = ec2_types.InstanceTypeInfo{
		InstanceType: ec2_types.InstanceType("t4g.micro"),
		ProcessorInfo: &ec2_types.ProcessorInfo{
			SupportedArchitectures: []ec2_types.ArchitectureType{ec2_types.ArchitectureTypeArm64},
		},
	}
)

func newTestProvider(client *mocks.MockClient) *provider {
	return &provider{
		name:   "aws",
		region: "eu-central-1",
		client: client,
	}
}

func regionOptions(region string) func([]func(*ec2.Options)) bool {
	return func(options []func(*ec2.Options)) bool {
		var got ec2.Options
		for _, option := range options {
			option(&got)
		}
		return got.Region == region
	}
}

func mockRegionResources(client *mocks.MockClient, region string, image ec2_types.Image, subnet, securityGroup string) {
	client.On("DescribeImages", mock.Anything, mock.MatchedBy(func(in *ec2.DescribeImagesInput) bool {
		return assert.ObjectsAreEqual([]string{aws.ToString(image.ImageId)}, in.ImageIds)
	}), mock.MatchedBy(regionOptions(region))).
		Return(&ec2.DescribeImagesOutput{Images: []ec2_types.Image{image}}, nil).Once()
	client.On("DescribeSubnets", mock.Anything, mock.MatchedBy(func(in *ec2.DescribeSubnetsInput) bool {
		return assert.ObjectsAreEqual([]string{subnet}, in.SubnetIds)
	}), mock.MatchedBy(regionOptions(region))).
		Return(&ec2.DescribeSubnetsOutput{Subnets: []ec2_types.Subnet{{}}}, nil).Once()
	client.On("DescribeSecurityGroups", mock.Anything, mock.MatchedBy(func(in *ec2.DescribeSecurityGroupsInput) bool {
		return assert.ObjectsAreEqual([]string{securityGroup}, in.GroupIds)
	}), mock.MatchedBy(regionOptions(region))).
		Return(&ec2.DescribeSecurityGroupsOutput{SecurityGroups: []ec2_types.SecurityGroup{{}}}, nil).Once()
}

func mockAliasRegionResources(client *mocks.MockClient, region string, image ec2_types.Image, subnet, securityGroup string) {
	nameArchitecture := "amd64"
	if image.Architecture == ec2_types.ArchitectureValuesArm64 {
		nameArchitecture = "arm64"
	}
	image.Name = aws.String("debian-13-" + nameArchitecture + "-20260714-1")
	image.CreationDate = aws.String("2026-07-14T00:00:00Z")
	image.ImageOwnerAlias = aws.String("amazon")
	image.Public = aws.Bool(true)
	image.RootDeviceType = ec2_types.DeviceTypeEbs
	image.State = ec2_types.ImageStateAvailable
	image.VirtualizationType = ec2_types.VirtualizationTypeHvm
	client.On("DescribeImages", mock.Anything, mock.MatchedBy(func(input *ec2.DescribeImagesInput) bool {
		return assert.ObjectsAreEqual([]string{string(image.Architecture)}, filterValues(input.Filters, "architecture")) &&
			assert.ObjectsAreEqual([]string{"debian-13-" + nameArchitecture + "-*"}, filterValues(input.Filters, "name"))
	}), mock.MatchedBy(regionOptions(region))).
		Return(&ec2.DescribeImagesOutput{Images: []ec2_types.Image{image}}, nil).Once()
	client.On("DescribeSubnets", mock.Anything, mock.MatchedBy(func(input *ec2.DescribeSubnetsInput) bool {
		return assert.ObjectsAreEqual([]string{subnet}, input.SubnetIds)
	}), mock.MatchedBy(regionOptions(region))).
		Return(&ec2.DescribeSubnetsOutput{Subnets: []ec2_types.Subnet{{}}}, nil).Once()
	client.On("DescribeSecurityGroups", mock.Anything, mock.MatchedBy(func(input *ec2.DescribeSecurityGroupsInput) bool {
		return assert.ObjectsAreEqual([]string{securityGroup}, input.GroupIds)
	}), mock.MatchedBy(regionOptions(region))).
		Return(&ec2.DescribeSecurityGroupsOutput{SecurityGroups: []ec2_types.SecurityGroup{{}}}, nil).Once()
}

func TestResolveDeployCandidates(t *testing.T) {
	t.Run("RegionalCandidates", func(t *testing.T) {
		client := mocks.NewMockClient(t)
		mockAliasRegionResources(client, "eu-central-1", testArmImage, "subnet-arm", "sg-arm")
		mockAliasRegionResources(client, "us-east-1", testImage, "subnet-x86", "sg-x86")
		client.On("DescribeInstanceTypes", mock.Anything, mock.MatchedBy(func(in *ec2.DescribeInstanceTypesInput) bool {
			return in.InstanceTypes[0] == "t4g.micro"
		}), mock.MatchedBy(regionOptions("eu-central-1"))).
			Return(&ec2.DescribeInstanceTypesOutput{InstanceTypes: []ec2_types.InstanceTypeInfo{testArmTypeInfo}}, nil).Once()
		client.On("DescribeInstanceTypes", mock.Anything, mock.MatchedBy(func(in *ec2.DescribeInstanceTypesInput) bool {
			return in.InstanceTypes[0] == "m6i.large"
		}), mock.MatchedBy(regionOptions("us-east-1"))).
			Return(&ec2.DescribeInstanceTypesOutput{InstanceTypes: []ec2_types.InstanceTypeInfo{testTypeInfo}}, nil).Once()

		p := newTestProvider(client)
		err := p.resolveDeployCandidates(
			t.Context(),
			[]string{"t4g.micro:eu-central-1", "m6i.large:us-east-1"},
			"debian-13",
			[]string{"subnet-arm:eu-central-1", "subnet-x86:us-east-1"},
			[]string{"sg-arm:eu-central-1", "sg-x86:us-east-1"},
		)
		assert.NoError(t, err)
		assert.Equal(t, []string{"eu-central-1", "us-east-1"}, p.regions)
		assert.Equal(t, "t4g.micro", string(p.deployCandidates[0].instanceType.InstanceType))
		assert.Equal(t, "eu-central-1", p.deployCandidates[0].regionConfig.region)
		assert.Equal(t, "ami-arm", aws.ToString(p.deployCandidates[0].regionConfig.image.ImageId))
		assert.Equal(t, []string{"subnet-arm"}, p.deployCandidates[0].regionConfig.subnets)
		assert.Equal(t, []string{"sg-arm"}, p.deployCandidates[0].regionConfig.securityGroups)
		assert.Equal(t, "m6i.large", string(p.deployCandidates[1].instanceType.InstanceType))
		assert.Equal(t, "us-east-1", p.deployCandidates[1].regionConfig.region)
		assert.Equal(t, "ami-x86", aws.ToString(p.deployCandidates[1].regionConfig.image.ImageId))
	})

	t.Run("UnqualifiedValuesUseDefaultRegion", func(t *testing.T) {
		client := mocks.NewMockClient(t)
		mockRegionResources(client, "eu-central-1", testImage, "subnet-1", "sg-1")
		client.On("DescribeInstanceTypes", mock.Anything, mock.Anything, mock.MatchedBy(regionOptions("eu-central-1"))).
			Return(&ec2.DescribeInstanceTypesOutput{InstanceTypes: []ec2_types.InstanceTypeInfo{testTypeInfo}}, nil).Once()

		p := newTestProvider(client)
		err := p.resolveDeployCandidates(
			t.Context(),
			[]string{"m6i.large"}, "ami-x86", []string{"subnet-1"}, []string{"sg-1"},
		)
		assert.NoError(t, err)
		assert.Equal(t, []string{"eu-central-1"}, p.regions)
	})

	t.Run("FullyQualifiedValuesDoNotNeedDefaultRegion", func(t *testing.T) {
		client := mocks.NewMockClient(t)
		mockRegionResources(client, "us-east-1", testImage, "subnet-1", "sg-1")
		client.On("DescribeInstanceTypes", mock.Anything, mock.Anything, mock.MatchedBy(regionOptions("us-east-1"))).
			Return(&ec2.DescribeInstanceTypesOutput{InstanceTypes: []ec2_types.InstanceTypeInfo{testTypeInfo}}, nil).Once()

		p := newTestProvider(client)
		p.region = ""
		err := p.resolveDeployCandidates(
			t.Context(),
			[]string{"m6i.large:us-east-1"}, "ami-x86",
			[]string{"subnet-1:us-east-1"}, []string{"sg-1:us-east-1"},
		)
		assert.NoError(t, err)
	})

	t.Run("ImageAliasUsesInstanceTypeRegionAndArchitecture", func(t *testing.T) {
		client := mocks.NewMockClient(t)
		client.On("DescribeInstanceTypes", mock.Anything, mock.Anything, mock.MatchedBy(regionOptions("us-east-1"))).
			Return(&ec2.DescribeInstanceTypesOutput{InstanceTypes: []ec2_types.InstanceTypeInfo{testArmTypeInfo}}, nil).Once()
		client.On("DescribeImages", mock.Anything, mock.MatchedBy(func(input *ec2.DescribeImagesInput) bool {
			return assert.ObjectsAreEqual(
				[]string{"debian-13-arm64-*"},
				filterValues(input.Filters, "name"),
			)
		}), mock.MatchedBy(regionOptions("us-east-1"))).Return(&ec2.DescribeImagesOutput{Images: []ec2_types.Image{{
			ImageId:            aws.String("ami-debian-arm"),
			Name:               aws.String("debian-13-arm64-20260714-1"),
			Architecture:       ec2_types.ArchitectureValuesArm64,
			CreationDate:       aws.String("2026-07-14T00:00:00Z"),
			ImageOwnerAlias:    aws.String("amazon"),
			Public:             aws.Bool(true),
			RootDeviceType:     ec2_types.DeviceTypeEbs,
			State:              ec2_types.ImageStateAvailable,
			VirtualizationType: ec2_types.VirtualizationTypeHvm,
		}}}, nil).Once()
		client.On("DescribeSubnets", mock.Anything, mock.Anything, mock.MatchedBy(regionOptions("us-east-1"))).
			Return(&ec2.DescribeSubnetsOutput{Subnets: []ec2_types.Subnet{{}}}, nil).Once()

		p := newTestProvider(client)
		p.region = ""
		err := p.resolveDeployCandidates(
			t.Context(),
			[]string{"t4g.micro:us-east-1"}, "debian-13",
			[]string{"subnet-1:us-east-1"}, nil,
		)
		require.NoError(t, err)
		assert.Equal(t, "ami-debian-arm", aws.ToString(p.deployCandidates[0].regionConfig.image.ImageId))
	})

	t.Run("AmbiguousInstanceTypeArchitecture", func(t *testing.T) {
		client := mocks.NewMockClient(t)
		client.On("DescribeInstanceTypes", mock.Anything, mock.Anything, mock.MatchedBy(regionOptions("us-east-1"))).
			Return(&ec2.DescribeInstanceTypesOutput{InstanceTypes: []ec2_types.InstanceTypeInfo{{
				InstanceType: ec2_types.InstanceType("c3.large"),
				ProcessorInfo: &ec2_types.ProcessorInfo{SupportedArchitectures: []ec2_types.ArchitectureType{
					ec2_types.ArchitectureTypeI386,
					ec2_types.ArchitectureTypeX8664,
				}},
			}}}, nil).Once()

		p := newTestProvider(client)
		err := p.resolveDeployCandidates(
			t.Context(),
			[]string{"c3.large:us-east-1"}, "debian-13",
			[]string{"subnet-1:us-east-1"}, nil,
		)
		assert.ErrorIs(t, err, ErrInstanceTypeArchitecture)
		assert.ErrorContains(t, err, "[i386 x86_64]")
	})

	t.Run("ImageMustNotSpecifyRegion", func(t *testing.T) {
		client := mocks.NewMockClient(t)
		client.On("DescribeInstanceTypes", mock.Anything, mock.Anything, mock.MatchedBy(regionOptions("us-east-1"))).
			Return(&ec2.DescribeInstanceTypesOutput{InstanceTypes: []ec2_types.InstanceTypeInfo{testTypeInfo}}, nil).Once()

		p := newTestProvider(client)
		err := p.resolveDeployCandidates(
			t.Context(),
			[]string{"m6i.large:us-east-1"}, "ami-x86:us-east-1",
			[]string{"subnet-1:us-east-1"}, nil,
		)
		assert.ErrorContains(t, err, "aws-ami-id must not specify a region")
	})

	t.Run("UnqualifiedValueNeedsDefaultRegion", func(t *testing.T) {
		client := mocks.NewMockClient(t)
		p := newTestProvider(client)
		p.region = ""
		err := p.resolveDeployCandidates(
			t.Context(),
			[]string{"m6i.large"}, "ami-x86",
			[]string{"subnet-1:us-east-1"}, []string{"sg-1:us-east-1"},
		)
		assert.ErrorIs(t, err, ErrRegionNotSet)
	})

	t.Run("NoCandidates", func(t *testing.T) {
		client := mocks.NewMockClient(t)
		p := newTestProvider(client)
		err := p.resolveDeployCandidates(t.Context(), nil, "", nil, nil)
		assert.ErrorIs(t, err, ErrNoDeployCandidates)
	})

	t.Run("MissingAMIPerCandidateRegion", func(t *testing.T) {
		client := mocks.NewMockClient(t)
		client.On("DescribeInstanceTypes", mock.Anything, mock.Anything, mock.MatchedBy(regionOptions("us-east-1"))).
			Return(&ec2.DescribeInstanceTypesOutput{InstanceTypes: []ec2_types.InstanceTypeInfo{testTypeInfo}}, nil).Once()
		p := newTestProvider(client)
		err := p.resolveDeployCandidates(
			t.Context(),
			[]string{"m6i.large:us-east-1"}, "",
			[]string{"subnet-1:us-east-1"}, nil,
		)
		assert.ErrorIs(t, err, ErrAMINotFound)
	})

	t.Run("TypeNotOfferedInRegion", func(t *testing.T) {
		client := mocks.NewMockClient(t)
		client.On("DescribeInstanceTypes", mock.Anything, mock.Anything, mock.MatchedBy(regionOptions("us-east-1"))).
			Return(nil, &apiError{code: "InvalidInstanceType"}).Once()

		p := newTestProvider(client)
		err := p.resolveDeployCandidates(
			t.Context(),
			[]string{"m6i.large:us-east-1"}, "ami-x86",
			[]string{"subnet-1:us-east-1"}, []string{"sg-1:us-east-1"},
		)
		assert.ErrorIs(t, err, ErrTypeNotInRegion)
	})

	t.Run("ArchMismatch", func(t *testing.T) {
		client := mocks.NewMockClient(t)
		mockRegionResources(client, "eu-central-1", testImage, "subnet-1", "sg-1")
		client.On("DescribeInstanceTypes", mock.Anything, mock.Anything, mock.MatchedBy(regionOptions("eu-central-1"))).
			Return(&ec2.DescribeInstanceTypesOutput{InstanceTypes: []ec2_types.InstanceTypeInfo{testArmTypeInfo}}, nil).Once()

		p := newTestProvider(client)
		err := p.resolveDeployCandidates(
			t.Context(),
			[]string{"t4g.micro"}, "ami-x86", []string{"subnet-1"}, []string{"sg-1"},
		)
		assert.ErrorIs(t, err, ErrArchMismatch)
	})
}

type apiError struct{ code string }

func (e *apiError) Error() string                 { return e.code }
func (e *apiError) ErrorCode() string             { return e.code }
func (e *apiError) ErrorMessage() string          { return e.code }
func (e *apiError) ErrorFault() smithy.ErrorFault { return smithy.FaultServer }

func TestIsCapacityError(t *testing.T) {
	for _, code := range []string{
		"InsufficientInstanceCapacity",
		"Server.InsufficientInstanceCapacity",
		"Unsupported",
		"MaxSpotInstanceCountExceeded",
		"SpotMaxPriceTooLow",
		"UnfulfillableCapacity",
	} {
		assert.True(t, isCapacityError(fmt.Errorf("wrapped: %w", &apiError{code: code})), code)
	}

	assert.False(t, isCapacityError(&apiError{code: "InvalidSubnetID.NotFound"}))
	assert.False(t, isCapacityError(errors.New("no api error")))
}

func TestGetAgent(t *testing.T) {
	instance := ec2_types.Instance{InstanceId: aws.String("i-1")}

	t.Run("FoundInFallbackRegion", func(t *testing.T) {
		client := mocks.NewMockClient(t)
		client.On("DescribeInstances", mock.Anything, mock.Anything, mock.MatchedBy(regionOptions("eu-central-1"))).
			Return(&ec2.DescribeInstancesOutput{}, nil).Once()
		client.On("DescribeInstances", mock.Anything, mock.Anything, mock.MatchedBy(regionOptions("us-east-1"))).
			Return(&ec2.DescribeInstancesOutput{
				Reservations: []ec2_types.Reservation{{Instances: []ec2_types.Instance{instance}}},
			}, nil).Once()

		p := newTestProvider(client)
		p.regions = []string{"eu-central-1", "us-east-1"}
		got, region, err := p.getAgent(t.Context(), &woodpecker.Agent{Name: "pool-1-agent-abcd"})
		assert.NoError(t, err)
		assert.Equal(t, "us-east-1", region)
		assert.Equal(t, "i-1", aws.ToString(got.InstanceId))
	})

	t.Run("QueriesOnlyNonTerminatedStates", func(t *testing.T) {
		client := mocks.NewMockClient(t)
		client.On("DescribeInstances", mock.Anything, mock.MatchedBy(func(in *ec2.DescribeInstancesInput) bool {
			for _, f := range in.Filters {
				if aws.ToString(f.Name) == "instance-state-name" {
					return assert.ObjectsAreEqual([]string{"pending", "running"}, f.Values)
				}
			}
			return false
		}), mock.MatchedBy(regionOptions("eu-central-1"))).
			Return(&ec2.DescribeInstancesOutput{
				Reservations: []ec2_types.Reservation{{Instances: []ec2_types.Instance{instance}}},
			}, nil)

		p := newTestProvider(client)
		p.regions = []string{"eu-central-1"}
		_, _, err := p.getAgent(t.Context(), &woodpecker.Agent{Name: "pool-1-agent-abcd"})
		assert.NoError(t, err)
	})
}
