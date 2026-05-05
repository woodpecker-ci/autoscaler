package oracle

import (
	"context"
	"fmt"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// mockComputeClient implements ComputeClient for unit tests.
type mockComputeClient struct {
	mock.Mock
}

func (m *mockComputeClient) LaunchInstance(ctx context.Context, req core.LaunchInstanceRequest) (core.LaunchInstanceResponse, error) {
	args := m.Called(ctx, req)
	return args.Get(0).(core.LaunchInstanceResponse), args.Error(1)
}

func (m *mockComputeClient) TerminateInstance(ctx context.Context, req core.TerminateInstanceRequest) (core.TerminateInstanceResponse, error) {
	args := m.Called(ctx, req)
	return args.Get(0).(core.TerminateInstanceResponse), args.Error(1)
}

func (m *mockComputeClient) ListInstances(ctx context.Context, req core.ListInstancesRequest) (core.ListInstancesResponse, error) {
	args := m.Called(ctx, req)
	return args.Get(0).(core.ListInstancesResponse), args.Error(1)
}

const (
	testCompartmentID      = "ocid1.compartment.oc1..test"
	testAvailabilityDomain = "IiCz:US-ASHBURN-AD-1"
	testPoolID             = "pool-1"
	testImageID            = "ocid1.image.oc1..test"
	testSubnetID           = "ocid1.subnet.oc1..test"
)

func testProvider(client ComputeClient) *Provider {
	return &Provider{
		name:               "oracle",
		compartmentID:      testCompartmentID,
		availabilityDomain: testAvailabilityDomain,
		imageID:            testImageID,
		shape:              "VM.Standard.E4.Flex",
		subnetID:           testSubnetID,
		shapeOCPUs:         1,
		shapeMemoryGBs:     6,
		config: &config.Config{
			PoolID:            testPoolID,
			GRPCAddress:       "grpc.example.com:9000",
			WorkflowsPerAgent: 2,
		},
		client: client,
	}
}

func activeInstance(id, name string) core.Instance {
	return core.Instance{
		Id:             common.String(id),
		DisplayName:    common.String(name),
		LifecycleState: core.InstanceLifecycleStateRunning,
		FreeformTags:   map[string]string{engine.LabelPool: testPoolID},
	}
}

func listResponse(instances []core.Instance, nextPage *string) core.ListInstancesResponse {
	return core.ListInstancesResponse{
		Items:       instances,
		OpcNextPage: nextPage,
	}
}

func TestDeployAgent(t *testing.T) {
	m := &mockComputeClient{}
	p := testProvider(m)
	agent := &woodpecker.Agent{Name: "wp-agent-1", Token: "test-token"}

	m.On("LaunchInstance", mock.Anything,
		mock.MatchedBy(func(req core.LaunchInstanceRequest) bool {
			d := req.LaunchInstanceDetails
			return *d.DisplayName == agent.Name &&
				*d.CompartmentId == testCompartmentID &&
				*d.Shape == "VM.Standard.E4.Flex"
		}),
	).Return(core.LaunchInstanceResponse{
		Instance: core.Instance{Id: common.String("ocid1.instance.oc1..new")},
	}, nil)

	assert.NoError(t, p.DeployAgent(context.Background(), agent))
	m.AssertExpectations(t)
}

func TestDeployAgent_APIError(t *testing.T) {
	m := &mockComputeClient{}
	p := testProvider(m)
	agent := &woodpecker.Agent{Name: "wp-agent-fail", Token: "test-token"}

	m.On("LaunchInstance", mock.Anything, mock.Anything).
		Return(core.LaunchInstanceResponse{}, fmt.Errorf("out of capacity"))

	err := p.DeployAgent(context.Background(), agent)
	assert.ErrorContains(t, err, "LaunchInstance")
	assert.ErrorContains(t, err, "out of capacity")
	m.AssertExpectations(t)
}

func TestRemoveAgent(t *testing.T) {
	m := &mockComputeClient{}
	p := testProvider(m)

	inst := activeInstance("ocid1.instance.oc1..abc", "wp-agent-1")

	m.On("ListInstances", mock.Anything, mock.MatchedBy(func(req core.ListInstancesRequest) bool {
		return *req.CompartmentId == testCompartmentID && req.Page == nil
	})).Return(listResponse([]core.Instance{inst}, nil), nil)

	m.On("TerminateInstance", mock.Anything, mock.MatchedBy(func(req core.TerminateInstanceRequest) bool {
		return *req.InstanceId == *inst.Id
	})).Return(core.TerminateInstanceResponse{}, nil)

	assert.NoError(t, p.RemoveAgent(context.Background(), &woodpecker.Agent{Name: "wp-agent-1"}))
	m.AssertExpectations(t)
}

func TestRemoveAgent_NotFound(t *testing.T) {
	m := &mockComputeClient{}
	p := testProvider(m)

	m.On("ListInstances", mock.Anything, mock.Anything).
		Return(listResponse(nil, nil), nil)

	assert.NoError(t, p.RemoveAgent(context.Background(), &woodpecker.Agent{Name: "wp-agent-missing"}))
	m.AssertNotCalled(t, "TerminateInstance")
	m.AssertExpectations(t)
}

func TestRemoveAgent_ListError(t *testing.T) {
	m := &mockComputeClient{}
	p := testProvider(m)

	m.On("ListInstances", mock.Anything, mock.Anything).
		Return(core.ListInstancesResponse{}, fmt.Errorf("network timeout"))

	err := p.RemoveAgent(context.Background(), &woodpecker.Agent{Name: "wp-agent-1"})
	assert.ErrorContains(t, err, "findInstance")
	m.AssertExpectations(t)
}

func TestListDeployedAgentNames(t *testing.T) {
	m := &mockComputeClient{}
	p := testProvider(m)

	inst1 := activeInstance("ocid1.instance.oc1..aaa", "wp-agent-1")
	inst2 := activeInstance("ocid1.instance.oc1..bbb", "wp-agent-2")

	m.On("ListInstances", mock.Anything, mock.Anything).
		Return(listResponse([]core.Instance{inst1, inst2}, nil), nil)

	names, err := p.ListDeployedAgentNames(context.Background())
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"wp-agent-1", "wp-agent-2"}, names)
	m.AssertExpectations(t)
}

func TestListDeployedAgentNames_FiltersTerminated(t *testing.T) {
	m := &mockComputeClient{}
	p := testProvider(m)

	running := activeInstance("ocid1.instance.oc1..aaa", "wp-agent-1")
	terminating := core.Instance{
		Id:             common.String("ocid1.instance.oc1..bbb"),
		DisplayName:    common.String("wp-agent-2"),
		LifecycleState: core.InstanceLifecycleStateTerminating,
		FreeformTags:   map[string]string{engine.LabelPool: testPoolID},
	}
	terminated := core.Instance{
		Id:             common.String("ocid1.instance.oc1..ccc"),
		DisplayName:    common.String("wp-agent-3"),
		LifecycleState: core.InstanceLifecycleStateTerminated,
		FreeformTags:   map[string]string{engine.LabelPool: testPoolID},
	}

	m.On("ListInstances", mock.Anything, mock.Anything).
		Return(listResponse([]core.Instance{running, terminating, terminated}, nil), nil)

	names, err := p.ListDeployedAgentNames(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, []string{"wp-agent-1"}, names)
	m.AssertExpectations(t)
}

func TestListDeployedAgentNames_FiltersOtherPools(t *testing.T) {
	m := &mockComputeClient{}
	p := testProvider(m)

	ours := activeInstance("ocid1.instance.oc1..aaa", "wp-agent-1")
	theirs := core.Instance{
		Id:             common.String("ocid1.instance.oc1..bbb"),
		DisplayName:    common.String("wp-agent-other"),
		LifecycleState: core.InstanceLifecycleStateRunning,
		FreeformTags:   map[string]string{engine.LabelPool: "different-pool"},
	}

	m.On("ListInstances", mock.Anything, mock.Anything).
		Return(listResponse([]core.Instance{ours, theirs}, nil), nil)

	names, err := p.ListDeployedAgentNames(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, []string{"wp-agent-1"}, names)
	m.AssertExpectations(t)
}

func TestListDeployedAgentNames_Pagination(t *testing.T) {
	m := &mockComputeClient{}
	p := testProvider(m)

	page2Token := "page-2-token"
	inst1 := activeInstance("ocid1.instance.oc1..aaa", "wp-agent-1")
	inst2 := activeInstance("ocid1.instance.oc1..bbb", "wp-agent-2")

	m.On("ListInstances", mock.Anything, mock.MatchedBy(func(req core.ListInstancesRequest) bool {
		return req.Page == nil
	})).Return(listResponse([]core.Instance{inst1}, common.String(page2Token)), nil).Once()

	m.On("ListInstances", mock.Anything, mock.MatchedBy(func(req core.ListInstancesRequest) bool {
		return req.Page != nil && *req.Page == page2Token
	})).Return(listResponse([]core.Instance{inst2}, nil), nil).Once()

	names, err := p.ListDeployedAgentNames(context.Background())
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"wp-agent-1", "wp-agent-2"}, names)
	m.AssertExpectations(t)
}

func TestListDeployedAgentNames_Empty(t *testing.T) {
	m := &mockComputeClient{}
	p := testProvider(m)

	m.On("ListInstances", mock.Anything, mock.Anything).
		Return(listResponse(nil, nil), nil)

	names, err := p.ListDeployedAgentNames(context.Background())
	assert.NoError(t, err)
	assert.Empty(t, names)
	m.AssertExpectations(t)
}
