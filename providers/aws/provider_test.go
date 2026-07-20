package aws

import (
	"slices"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2_types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/autoscaler/providers/aws/ec2api/mocks"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

func newDeployTestProvider(client *mocks.MockClient, candidates []deployCandidate) *provider {
	p := newTestProvider(client)
	p.config = &config.Config{PoolID: "1"}
	p.deployCandidates = candidates
	for _, candidate := range candidates {
		if !slices.Contains(p.regions, candidate.regionConfig.region) {
			p.regions = append(p.regions, candidate.regionConfig.region)
		}
	}
	return p
}

func testCandidates() []deployCandidate {
	return []deployCandidate{
		{
			instanceType: testArmTypeInfo,
			regionConfig: regionConfig{
				region:         "eu-central-1",
				image:          testArmImage,
				subnets:        []string{"subnet-arm"},
				securityGroups: []string{"sg-arm"},
			},
		},
		{
			instanceType: testTypeInfo,
			regionConfig: regionConfig{
				region:         "us-east-1",
				image:          testImage,
				subnets:        []string{"subnet-x86"},
				securityGroups: []string{"sg-x86"},
			},
		},
	}
}

// testX86Candidates returns two candidates for the same capability
// (linux/amd64) in different regions, so failover stays within one
// capability.
func testX86Candidates() []deployCandidate {
	return []deployCandidate{
		{
			instanceType: testTypeInfo,
			regionConfig: regionConfig{
				region:         "eu-central-1",
				image:          testImage,
				subnets:        []string{"subnet-x86-eu"},
				securityGroups: []string{"sg-x86-eu"},
			},
		},
		{
			instanceType: testTypeInfo,
			regionConfig: regionConfig{
				region:         "us-east-1",
				image:          testImage,
				subnets:        []string{"subnet-x86"},
				securityGroups: []string{"sg-x86"},
			},
		},
	}
}

// testUnknownArchCandidate has an image architecture the goarch mapping does
// not know.
func testUnknownArchCandidate() deployCandidate {
	return deployCandidate{
		instanceType: ec2_types.InstanceTypeInfo{InstanceType: ec2_types.InstanceType("t1.micro")},
		regionConfig: regionConfig{
			region:         "eu-central-1",
			image:          ec2_types.Image{ImageId: aws.String("ami-i386"), Architecture: ec2_types.ArchitectureValuesI386},
			subnets:        []string{"subnet-i386"},
			securityGroups: []string{"sg-i386"},
		},
	}
}

func mockAgentVisible(client *mocks.MockClient, agentName string) {
	client.On("DescribeInstances", mock.Anything, mock.Anything, mock.Anything).
		Return(&ec2.DescribeInstancesOutput{
			Reservations: []ec2_types.Reservation{{Instances: []ec2_types.Instance{{
				InstanceId: aws.String("i-1"),
				State:      &ec2_types.InstanceState{Name: ec2_types.InstanceStateNameRunning},
				Tags:       []ec2_types.Tag{{Key: aws.String("Name"), Value: aws.String(agentName)}},
			}}}},
		}, nil)
}

func runRequest(instanceType, imageID, subnet, securityGroup string) func(*ec2.RunInstancesInput) bool {
	return func(in *ec2.RunInstancesInput) bool {
		return in.InstanceType == ec2_types.InstanceType(instanceType) &&
			aws.ToString(in.ImageId) == imageID &&
			aws.ToString(in.SubnetId) == subnet &&
			assert.ObjectsAreEqual([]string{securityGroup}, in.SecurityGroupIds)
	}
}

var (
	dockerAmd64Cap = types.Capability{Platform: "linux/amd64", Backend: types.BackendDocker}
	dockerArm64Cap = types.Capability{Platform: "linux/arm64", Backend: types.BackendDocker}
)

func TestDeployAgentFallback(t *testing.T) {
	agent := &woodpecker.Agent{Name: "pool-1-agent-abcd"}
	runOut := &ec2.RunInstancesOutput{Instances: []ec2_types.Instance{{InstanceId: aws.String("i-1")}}}

	t.Run("FirstCandidateSucceeds", func(t *testing.T) {
		client := mocks.NewMockClient(t)
		client.On("RunInstances", mock.Anything, mock.Anything, mock.Anything).
			Return(runOut, nil).Once()
		mockAgentVisible(client, agent.Name)

		p := newDeployTestProvider(client, testCandidates())
		assert.NoError(t, p.DeployAgent(t.Context(), agent, dockerArm64Cap))
	})

	t.Run("CapacityErrorFallsBackToRegionalCandidate", func(t *testing.T) {
		client := mocks.NewMockClient(t)
		client.On("RunInstances", mock.Anything,
			mock.MatchedBy(runRequest("m6i.large", "ami-x86", "subnet-x86-eu", "sg-x86-eu")),
			mock.MatchedBy(regionOptions("eu-central-1"))).
			Return(nil, &apiError{code: "InsufficientInstanceCapacity"}).Once()
		client.On("RunInstances", mock.Anything,
			mock.MatchedBy(runRequest("m6i.large", "ami-x86", "subnet-x86", "sg-x86")),
			mock.MatchedBy(regionOptions("us-east-1"))).
			Return(runOut, nil).Once()
		mockAgentVisible(client, agent.Name)

		p := newDeployTestProvider(client, testX86Candidates())
		assert.NoError(t, p.DeployAgent(t.Context(), agent, dockerAmd64Cap))
	})

	t.Run("CapabilityFiltersCandidates", func(t *testing.T) {
		// Mixed arm+x86 chain, arm requested: the x86 candidate must never
		// be tried, even when the arm one is out of capacity.
		client := mocks.NewMockClient(t)
		client.On("RunInstances", mock.Anything,
			mock.MatchedBy(runRequest("t4g.micro", "ami-arm", "subnet-arm", "sg-arm")),
			mock.MatchedBy(regionOptions("eu-central-1"))).
			Return(nil, &apiError{code: "InsufficientInstanceCapacity"}).Once()

		p := newDeployTestProvider(client, testCandidates())
		err := p.DeployAgent(t.Context(), agent, dockerArm64Cap)
		assert.ErrorContains(t, err, "all 1 deploy candidates out of capacity")
	})

	t.Run("NoCandidateForCapability", func(t *testing.T) {
		client := mocks.NewMockClient(t)

		p := newDeployTestProvider(client, testX86Candidates())
		err := p.DeployAgent(t.Context(), agent, dockerArm64Cap)
		assert.ErrorIs(t, err, ErrNoMatchingCandidate)
	})

	t.Run("UnknownArchitectureFailsAtRuntime", func(t *testing.T) {
		client := mocks.NewMockClient(t)

		p := newDeployTestProvider(client, append(testCandidates(), testUnknownArchCandidate()))
		err := p.DeployAgent(t.Context(), agent, dockerAmd64Cap)
		assert.ErrorIs(t, err, ErrUnknownArchitecture)
	})

	t.Run("NonDockerBackendRejected", func(t *testing.T) {
		client := mocks.NewMockClient(t)

		p := newDeployTestProvider(client, testCandidates())
		err := p.DeployAgent(t.Context(), agent,
			types.Capability{Platform: "linux/amd64", Backend: types.BackendKubernetes})
		assert.ErrorIs(t, err, ErrNoMatchingCandidate)
	})

	t.Run("NonCapacityErrorAborts", func(t *testing.T) {
		client := mocks.NewMockClient(t)
		client.On("RunInstances", mock.Anything, mock.Anything, mock.Anything).
			Return(nil, &apiError{code: "InvalidSubnetID.NotFound"}).Once()

		p := newDeployTestProvider(client, testCandidates())
		err := p.DeployAgent(t.Context(), agent, dockerArm64Cap)
		assert.ErrorContains(t, err, "InvalidSubnetID.NotFound")
	})

	t.Run("EmptyRunInstancesResult", func(t *testing.T) {
		client := mocks.NewMockClient(t)
		client.On("RunInstances", mock.Anything, mock.Anything, mock.Anything).
			Return(&ec2.RunInstancesOutput{}, nil).Once()

		p := newDeployTestProvider(client, testCandidates())
		err := p.DeployAgent(t.Context(), agent, dockerArm64Cap)
		assert.ErrorContains(t, err, "returned no instances")
	})

	t.Run("AllCandidatesOutOfCapacity", func(t *testing.T) {
		client := mocks.NewMockClient(t)
		client.On("RunInstances", mock.Anything, mock.Anything, mock.Anything).
			Return(nil, &apiError{code: "InsufficientInstanceCapacity"}).Twice()

		p := newDeployTestProvider(client, testX86Candidates())
		err := p.DeployAgent(t.Context(), agent, dockerAmd64Cap)
		assert.ErrorContains(t, err, "all 2 deploy candidates out of capacity")
	})
}

func TestCapabilities(t *testing.T) {
	t.Run("DerivedFromCandidates", func(t *testing.T) {
		p := newDeployTestProvider(mocks.NewMockClient(t), testCandidates())
		caps, err := p.Capabilities(t.Context())
		assert.NoError(t, err)
		assert.ElementsMatch(t, []types.Capability{dockerArm64Cap, dockerAmd64Cap}, caps)
	})

	t.Run("DeduplicatesSamePlatform", func(t *testing.T) {
		p := newDeployTestProvider(mocks.NewMockClient(t), testX86Candidates())
		caps, err := p.Capabilities(t.Context())
		assert.NoError(t, err)
		assert.Equal(t, []types.Capability{dockerAmd64Cap}, caps)
	})

	t.Run("UnknownArchitectureFail", func(t *testing.T) {
		p := newDeployTestProvider(mocks.NewMockClient(t),
			append(testCandidates(), testUnknownArchCandidate()))
		_, err := p.Capabilities(t.Context())
		assert.ErrorIs(t, err, ErrUnknownArchitecture)
	})
}

func TestRemoveAgentSkipsOSShutdown(t *testing.T) {
	agent := &woodpecker.Agent{Name: "pool-1-agent-abcd"}

	client := mocks.NewMockClient(t)
	mockAgentVisible(client, agent.Name)
	client.On("TerminateInstances", mock.Anything,
		mock.MatchedBy(func(in *ec2.TerminateInstancesInput) bool {
			return assert.ObjectsAreEqual([]string{"i-1"}, in.InstanceIds) &&
				aws.ToBool(in.SkipOsShutdown)
		}),
		mock.Anything).
		Return(&ec2.TerminateInstancesOutput{}, nil).Once()

	p := newTestProvider(client)
	p.regions = []string{"eu-central-1"}
	assert.NoError(t, p.RemoveAgent(t.Context(), agent))
}
