package oracle

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

func TestDeployAgentLaunchesConfiguredInstance(t *testing.T) {
	client := &mockComputeClient{}
	provider := testProvider(client)

	err := provider.DeployAgent(t.Context(), &woodpecker.Agent{
		Name:  "agent-1",
		Token: "secret",
	})
	require.NoError(t, err)
	require.Len(t, client.launchRequests, 1)

	details := client.launchRequests[0].LaunchInstanceDetails
	assert.Equal(t, "Uocm:PHX-AD-1", *details.AvailabilityDomain)
	assert.Equal(t, "ocid1.compartment.oc1..test", *details.CompartmentId)
	assert.Equal(t, "agent-1", *details.DisplayName)
	assert.Equal(t, "VM.Standard.E4.Flex", *details.Shape)
	assert.Equal(t, "ocid1.subnet.oc1..test", *details.CreateVnicDetails.SubnetId)
	assert.True(t, *details.CreateVnicDetails.AssignPublicIp)
	assert.Equal(t, "pool-1", details.FreeformTags[engine.LabelPool])
	assert.Equal(t, "ocid1.image.oc1..test", details.FreeformTags[engine.LabelImage])
	assert.Equal(t, float32(1), *details.ShapeConfig.Ocpus)
	assert.Equal(t, float32(6), *details.ShapeConfig.MemoryInGBs)
	assert.Equal(t, "ssh-ed25519 AAAA test", details.Metadata["ssh_authorized_keys"])

	decodedUserData, err := base64.StdEncoding.DecodeString(details.Metadata["user_data"])
	require.NoError(t, err)
	assert.Contains(t, string(decodedUserData), "WOODPECKER_AGENT_SECRET=secret")

	source, ok := details.SourceDetails.(core.InstanceSourceViaImageDetails)
	require.True(t, ok)
	assert.Equal(t, "ocid1.image.oc1..test", *source.ImageId)
}

func TestListDeployedAgentNamesFiltersPoolStateAndPages(t *testing.T) {
	nextPage := "next"
	client := &mockComputeClient{
		listResponses: []core.ListInstancesResponse{
			{
				Items: []core.Instance{
					testInstance("agent-1", "instance-1", "pool-1", core.InstanceLifecycleStateRunning),
					testInstance("other-pool", "instance-2", "pool-2", core.InstanceLifecycleStateRunning),
				},
				OpcNextPage: &nextPage,
			},
			{
				Items: []core.Instance{
					testInstance("agent-2", "instance-3", "pool-1", core.InstanceLifecycleStateProvisioning),
					testInstance("terminating", "instance-4", "pool-1", core.InstanceLifecycleStateTerminating),
				},
			},
		},
	}
	provider := testProvider(client)

	names, err := provider.ListDeployedAgentNames(t.Context())
	require.NoError(t, err)

	assert.Equal(t, []string{"agent-1", "agent-2"}, names)
	require.Len(t, client.listRequests, 2)
	assert.Nil(t, client.listRequests[0].Page)
	assert.Equal(t, nextPage, *client.listRequests[1].Page)
}

func TestRemoveAgentTerminatesMatchingInstance(t *testing.T) {
	client := &mockComputeClient{
		listResponses: []core.ListInstancesResponse{
			{Items: []core.Instance{
				testInstance("agent-1", "instance-1", "pool-1", core.InstanceLifecycleStateRunning),
			}},
		},
	}
	provider := testProvider(client)

	err := provider.RemoveAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	require.NoError(t, err)

	require.Len(t, client.listRequests, 1)
	assert.Equal(t, "agent-1", *client.listRequests[0].DisplayName)
	require.Len(t, client.terminateRequests, 1)
	assert.Equal(t, "instance-1", *client.terminateRequests[0].InstanceId)
}

func TestRemoveAgentErrorsOnDuplicateInstances(t *testing.T) {
	client := &mockComputeClient{
		listResponses: []core.ListInstancesResponse{
			{Items: []core.Instance{
				testInstance("agent-1", "instance-1", "pool-1", core.InstanceLifecycleStateRunning),
				testInstance("agent-1", "instance-2", "pool-1", core.InstanceLifecycleStateRunning),
			}},
		},
	}
	provider := testProvider(client)

	err := provider.RemoveAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple instances")
	assert.Empty(t, client.terminateRequests)
}

func TestDeployAgentWrapsRenderErrors(t *testing.T) {
	provider := testProvider(&mockComputeClient{})
	provider.config.UserData = "{{ .Missing }}"

	err := provider.DeployAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RenderUserDataTemplate")
}

func TestListDeployedAgentNamesWrapsClientError(t *testing.T) {
	provider := testProvider(&mockComputeClient{listErr: errors.New("boom")})

	_, err := provider.ListDeployedAgentNames(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ListInstances")
}

type mockComputeClient struct {
	launchRequests    []core.LaunchInstanceRequest
	terminateRequests []core.TerminateInstanceRequest
	listRequests      []core.ListInstancesRequest
	listResponses     []core.ListInstancesResponse
	listErr           error
}

func (m *mockComputeClient) LaunchInstance(_ context.Context, req core.LaunchInstanceRequest) (core.LaunchInstanceResponse, error) {
	m.launchRequests = append(m.launchRequests, req)
	return core.LaunchInstanceResponse{}, nil
}

func (m *mockComputeClient) TerminateInstance(_ context.Context, req core.TerminateInstanceRequest) (core.TerminateInstanceResponse, error) {
	m.terminateRequests = append(m.terminateRequests, req)
	return core.TerminateInstanceResponse{}, nil
}

func (m *mockComputeClient) ListInstances(_ context.Context, req core.ListInstancesRequest) (core.ListInstancesResponse, error) {
	m.listRequests = append(m.listRequests, req)
	if m.listErr != nil {
		return core.ListInstancesResponse{}, m.listErr
	}
	if len(m.listResponses) == 0 {
		return core.ListInstancesResponse{}, nil
	}
	resp := m.listResponses[0]
	m.listResponses = m.listResponses[1:]
	return resp, nil
}

func testProvider(client computeClient) *Provider {
	return &Provider{
		name:               "oracle",
		compartmentID:      "ocid1.compartment.oc1..test",
		availabilityDomain: "Uocm:PHX-AD-1",
		subnetID:           "ocid1.subnet.oc1..test",
		imageID:            "ocid1.image.oc1..test",
		shape:              "VM.Standard.E4.Flex",
		ocpus:              1,
		memoryInGBs:        6,
		sshAuthorizedKey:   "ssh-ed25519 AAAA test",
		assignPublicIP:     true,
		tags: map[string]string{
			engine.LabelPool:  "pool-1",
			engine.LabelImage: "ocid1.image.oc1..test",
		},
		config: &config.Config{
			PoolID:            "pool-1",
			GRPCAddress:       "grpc.example.com:9000",
			Image:             "woodpeckerci/woodpecker-agent:next",
			WorkflowsPerAgent: 2,
		},
		client: client,
	}
}

func testInstance(name, id, pool string, state core.InstanceLifecycleStateEnum) core.Instance {
	return core.Instance{
		DisplayName:    strPtr(name),
		Id:             strPtr(id),
		LifecycleState: state,
		FreeformTags: map[string]string{
			engine.LabelPool: pool,
		},
	}
}
