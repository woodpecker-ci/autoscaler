package oracle

import (
	b64 "encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/autoscaler/providers/oracle/ociapi/mocks"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var testAgent = &woodpecker.Agent{Name: "pool-1-agent-abcd", Token: "agent-token"}

func launchIn(availabilityDomain string) func(core.LaunchInstanceRequest) bool {
	return func(r core.LaunchInstanceRequest) bool {
		return *r.AvailabilityDomain == availabilityDomain
	}
}

func outOfCapacity() error {
	return stubServiceError{code: "InternalError", message: "Out of host capacity.", status: 500}
}

func TestDeployAgent(t *testing.T) {
	t.Run("LaunchesInFirstAvailabilityDomain", func(t *testing.T) {
		compute := mocks.NewMockComputeClient(t)
		compute.On("LaunchInstance", mock.Anything, mock.MatchedBy(launchIn("AD-1"))).
			Return(core.LaunchInstanceResponse{}, nil).Once()

		p := testProvider(compute, mocks.NewMockIdentityClient(t))
		require.NoError(t, p.DeployAgent(t.Context(), testAgent))
	})

	t.Run("FallsBackToNextAvailabilityDomainOnCapacityError", func(t *testing.T) {
		compute := mocks.NewMockComputeClient(t)
		compute.On("LaunchInstance", mock.Anything, mock.MatchedBy(launchIn("AD-1"))).
			Return(core.LaunchInstanceResponse{}, outOfCapacity()).Once()
		compute.On("LaunchInstance", mock.Anything, mock.MatchedBy(launchIn("AD-2"))).
			Return(core.LaunchInstanceResponse{}, nil).Once()

		p := testProvider(compute, mocks.NewMockIdentityClient(t))
		require.NoError(t, p.DeployAgent(t.Context(), testAgent))
	})

	t.Run("ExhaustedAvailabilityDomainsAreReported", func(t *testing.T) {
		compute := mocks.NewMockComputeClient(t)
		compute.On("LaunchInstance", mock.Anything, mock.Anything).
			Return(core.LaunchInstanceResponse{}, outOfCapacity()).Twice()

		p := testProvider(compute, mocks.NewMockIdentityClient(t))
		assert.ErrorIs(t, p.DeployAgent(t.Context(), testAgent), ErrAllAvailabilityDomainsOut)
	})

	t.Run("NonCapacityErrorDoesNotRetry", func(t *testing.T) {
		compute := mocks.NewMockComputeClient(t)
		compute.On("LaunchInstance", mock.Anything, mock.Anything).
			Return(core.LaunchInstanceResponse{}, stubServiceError{
				code:    "NotAuthorizedOrNotFound",
				message: "subnet not found",
				status:  404,
			}).Once()

		p := testProvider(compute, mocks.NewMockIdentityClient(t))
		err := p.DeployAgent(t.Context(), testAgent)
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrAllAvailabilityDomainsOut)
	})
}

func TestLaunchDetails(t *testing.T) {
	compute := mocks.NewMockComputeClient(t)
	var captured core.LaunchInstanceDetails
	compute.On("LaunchInstance", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			request, ok := args.Get(1).(core.LaunchInstanceRequest)
			require.True(t, ok)
			captured = request.LaunchInstanceDetails
		}).
		Return(core.LaunchInstanceResponse{}, nil).Once()

	p := testProvider(compute, mocks.NewMockIdentityClient(t))
	p.sshKey = "ssh-ed25519 AAAA test"
	p.tags["owner"] = "ci"
	require.NoError(t, p.DeployAgent(t.Context(), testAgent))

	assert.Equal(t, testAgent.Name, *captured.DisplayName)
	assert.Equal(t, p.compartmentID, *captured.CompartmentId)
	assert.Equal(t, p.shape, *captured.Shape)
	assert.Equal(t, p.subnetID, *captured.CreateVnicDetails.SubnetId)
	assert.True(t, *captured.CreateVnicDetails.AssignPublicIp)
	assert.Equal(t, "1", captured.FreeformTags[poolTagKey()])
	assert.Equal(t, "ci", captured.FreeformTags["owner"])
	assert.Equal(t, "ssh-ed25519 AAAA test", captured.Metadata[metadataSSHKey])

	source, ok := captured.SourceDetails.(core.InstanceSourceViaImageDetails)
	require.True(t, ok)
	assert.Equal(t, p.imageID, *source.ImageId)
	assert.Equal(t, int64(minBootVolumeGB), *source.BootVolumeSizeInGBs)

	require.NotNil(t, captured.ShapeConfig)
	assert.InDelta(t, float32(1), *captured.ShapeConfig.Ocpus, 0)
	assert.InDelta(t, float32(8), *captured.ShapeConfig.MemoryInGBs, 0)
}

func TestFixedShapeGetsNoShapeConfig(t *testing.T) {
	compute := mocks.NewMockComputeClient(t)
	var captured core.LaunchInstanceDetails
	compute.On("LaunchInstance", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			request, ok := args.Get(1).(core.LaunchInstanceRequest)
			require.True(t, ok)
			captured = request.LaunchInstanceDetails
		}).
		Return(core.LaunchInstanceResponse{}, nil).Once()

	p := testProvider(compute, mocks.NewMockIdentityClient(t))
	p.shape = "VM.Standard.E2.1.Micro"
	p.shapeIsFlexible = false
	require.NoError(t, p.DeployAgent(t.Context(), testAgent))

	assert.Nil(t, captured.ShapeConfig)
}

func TestUserDataIsBase64EncodedAndBlackholesTheMetadataService(t *testing.T) {
	compute := mocks.NewMockComputeClient(t)
	var captured core.LaunchInstanceDetails
	compute.On("LaunchInstance", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			request, ok := args.Get(1).(core.LaunchInstanceRequest)
			require.True(t, ok)
			captured = request.LaunchInstanceDetails
		}).
		Return(core.LaunchInstanceResponse{}, nil).Once()

	p := testProvider(compute, mocks.NewMockIdentityClient(t))
	require.NoError(t, p.DeployAgent(t.Context(), testAgent))

	decoded, err := b64.StdEncoding.DecodeString(captured.Metadata[metadataUserKey])
	require.NoError(t, err)

	userData := string(decoded)
	assert.True(t, strings.HasPrefix(userData, "#cloud-config"))
	assert.Contains(t, userData, "ip -4 route add blackhole 169.254.169.254/32")
	assert.Contains(t, userData, testAgent.Token)
}

func TestListDeployedAgentNames(t *testing.T) {
	t.Run("ReturnsOnlyLiveInstancesOfThisPool", func(t *testing.T) {
		compute := mocks.NewMockComputeClient(t)
		compute.On("ListInstances", mock.Anything, mock.Anything).
			Return(core.ListInstancesResponse{Items: []core.Instance{
				instance("pool-1-agent-a", "1", core.InstanceLifecycleStateRunning),
				instance("pool-1-agent-b", "1", core.InstanceLifecycleStateProvisioning),
				instance("pool-2-agent-c", "2", core.InstanceLifecycleStateRunning),
				instance("pool-1-agent-d", "1", core.InstanceLifecycleStateTerminating),
				instance("pool-1-agent-e", "1", core.InstanceLifecycleStateTerminated),
			}}, nil).Once()

		p := testProvider(compute, mocks.NewMockIdentityClient(t))
		names, err := p.ListDeployedAgentNames(t.Context())
		require.NoError(t, err)
		assert.Equal(t, []string{"pool-1-agent-a", "pool-1-agent-b"}, names)
	})

	t.Run("FollowsPagination", func(t *testing.T) {
		compute := mocks.NewMockComputeClient(t)
		compute.On("ListInstances", mock.Anything, mock.MatchedBy(func(r core.ListInstancesRequest) bool {
			return r.Page == nil
		})).Return(core.ListInstancesResponse{
			Items:       []core.Instance{instance("pool-1-agent-a", "1", core.InstanceLifecycleStateRunning)},
			OpcNextPage: common.String("page-2"),
		}, nil).Once()
		compute.On("ListInstances", mock.Anything, mock.MatchedBy(func(r core.ListInstancesRequest) bool {
			return r.Page != nil && *r.Page == "page-2"
		})).Return(core.ListInstancesResponse{
			Items: []core.Instance{instance("pool-1-agent-b", "1", core.InstanceLifecycleStateRunning)},
		}, nil).Once()

		p := testProvider(compute, mocks.NewMockIdentityClient(t))
		names, err := p.ListDeployedAgentNames(t.Context())
		require.NoError(t, err)
		assert.Equal(t, []string{"pool-1-agent-a", "pool-1-agent-b"}, names)
	})
}

func TestRemoveAgent(t *testing.T) {
	t.Run("TerminatesInstanceAndItsBootVolume", func(t *testing.T) {
		compute := mocks.NewMockComputeClient(t)
		compute.On("ListInstances", mock.Anything, mock.MatchedBy(func(r core.ListInstancesRequest) bool {
			return r.DisplayName != nil && *r.DisplayName == testAgent.Name
		})).Return(core.ListInstancesResponse{Items: []core.Instance{
			instance(testAgent.Name, "1", core.InstanceLifecycleStateRunning),
		}}, nil).Once()
		compute.On("TerminateInstance", mock.Anything, mock.MatchedBy(func(r core.TerminateInstanceRequest) bool {
			return *r.InstanceId == "ocid1.instance.oc1.."+testAgent.Name && !*r.PreserveBootVolume
		})).Return(core.TerminateInstanceResponse{}, nil).Once()

		p := testProvider(compute, mocks.NewMockIdentityClient(t))
		require.NoError(t, p.RemoveAgent(t.Context(), testAgent))
	})

	t.Run("MissingInstanceIsNotAnError", func(t *testing.T) {
		compute := mocks.NewMockComputeClient(t)
		compute.On("ListInstances", mock.Anything, mock.Anything).
			Return(core.ListInstancesResponse{}, nil).Once()

		p := testProvider(compute, mocks.NewMockIdentityClient(t))
		require.NoError(t, p.RemoveAgent(t.Context(), testAgent))
	})

	t.Run("InstanceOfAnotherPoolIsNeverTerminated", func(t *testing.T) {
		compute := mocks.NewMockComputeClient(t)
		compute.On("ListInstances", mock.Anything, mock.Anything).
			Return(core.ListInstancesResponse{Items: []core.Instance{
				instance(testAgent.Name, "2", core.InstanceLifecycleStateRunning),
			}}, nil).Once()

		p := testProvider(compute, mocks.NewMockIdentityClient(t))
		require.NoError(t, p.RemoveAgent(t.Context(), testAgent))
	})

	t.Run("ListErrorIsPropagated", func(t *testing.T) {
		compute := mocks.NewMockComputeClient(t)
		compute.On("ListInstances", mock.Anything, mock.Anything).
			Return(core.ListInstancesResponse{}, errors.New("connection reset")).Once()

		p := testProvider(compute, mocks.NewMockIdentityClient(t))
		assert.Error(t, p.RemoveAgent(t.Context(), testAgent))
	})
}

func TestBillingModel(t *testing.T) {
	p := &provider{name: "oracle", config: &config.Config{}}
	assert.Equal(t, types.BillingPerSecond, p.BillingModel())
}
