package equinixmetal

import (
	"context"
	"fmt"
	"testing"

	"github.com/equinix/equinix-sdk-go/services/metalv1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// mockDevicesAPI implements DevicesAPI for unit tests.
type mockDevicesAPI struct {
	mock.Mock
}

func (m *mockDevicesAPI) CreateDevice(ctx context.Context, projectID string, body metalv1.CreateDeviceRequest) (*metalv1.Device, error) {
	args := m.Called(ctx, projectID, body)
	if d := args.Get(0); d != nil {
		return d.(*metalv1.Device), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockDevicesAPI) DeleteDevice(ctx context.Context, deviceID string) error {
	return m.Called(ctx, deviceID).Error(0)
}

func (m *mockDevicesAPI) FindProjectDevicesByTag(ctx context.Context, projectID string, tag string) ([]metalv1.Device, error) {
	args := m.Called(ctx, projectID, tag)
	if d := args.Get(0); d != nil {
		return d.([]metalv1.Device), args.Error(1)
	}
	return nil, args.Error(1)
}

const (
	testProjectID = "proj-abc-123"
	testPoolID    = "pool-1"
)

func testProvider(client DevicesAPI) *Provider {
	return &Provider{
		name:      "equinixmetal",
		projectID: testProjectID,
		metro:     "sv",
		plan:      "c3.small.x86",
		os:        "ubuntu_22_04",
		labels: map[string]string{
			engine.LabelPool:  testPoolID,
			engine.LabelImage: "ubuntu_22_04",
		},
		config: &config.Config{
			PoolID:            testPoolID,
			GRPCAddress:       "grpc.example.com:9000",
			WorkflowsPerAgent: 2,
		},
		client: client,
	}
}

func poolTag() string {
	return fmt.Sprintf("%s=%s", engine.LabelPool, testPoolID)
}

func TestDeployAgent(t *testing.T) {
	m := &mockDevicesAPI{}
	p := testProvider(m)
	agent := &woodpecker.Agent{Name: "wp-agent-1", Token: "test-token"}

	m.On("CreateDevice", mock.Anything, testProjectID,
		mock.MatchedBy(func(b metalv1.CreateDeviceRequest) bool {
			in := b.DeviceCreateInMetroInput
			return in != nil && in.GetHostname() == agent.Name && in.GetMetro() == "sv"
		}),
	).Return(&metalv1.Device{}, nil)

	err := p.DeployAgent(context.Background(), agent)
	assert.NoError(t, err)
	m.AssertExpectations(t)
}

func TestDeployAgent_APIError(t *testing.T) {
	m := &mockDevicesAPI{}
	p := testProvider(m)
	agent := &woodpecker.Agent{Name: "wp-agent-fail", Token: "test-token"}

	m.On("CreateDevice", mock.Anything, testProjectID, mock.Anything).
		Return(nil, fmt.Errorf("quota exceeded"))

	err := p.DeployAgent(context.Background(), agent)
	assert.ErrorContains(t, err, "CreateDevice")
	assert.ErrorContains(t, err, "quota exceeded")
	m.AssertExpectations(t)
}

func TestRemoveAgent(t *testing.T) {
	m := &mockDevicesAPI{}
	p := testProvider(m)

	deviceID := "device-xyz-456"
	d := metalv1.Device{}
	d.SetHostname("wp-agent-1")
	d.SetId(deviceID)

	m.On("FindProjectDevicesByTag", mock.Anything, testProjectID, poolTag()).
		Return([]metalv1.Device{d}, nil)
	m.On("DeleteDevice", mock.Anything, deviceID).Return(nil)

	err := p.RemoveAgent(context.Background(), &woodpecker.Agent{Name: "wp-agent-1"})
	assert.NoError(t, err)
	m.AssertExpectations(t)
}

func TestRemoveAgent_NotFound(t *testing.T) {
	m := &mockDevicesAPI{}
	p := testProvider(m)

	m.On("FindProjectDevicesByTag", mock.Anything, testProjectID, poolTag()).
		Return([]metalv1.Device{}, nil)

	err := p.RemoveAgent(context.Background(), &woodpecker.Agent{Name: "wp-agent-missing"})
	assert.NoError(t, err)
	m.AssertNotCalled(t, "DeleteDevice", mock.Anything, mock.Anything)
	m.AssertExpectations(t)
}

func TestRemoveAgent_ListError(t *testing.T) {
	m := &mockDevicesAPI{}
	p := testProvider(m)

	m.On("FindProjectDevicesByTag", mock.Anything, testProjectID, poolTag()).
		Return(nil, fmt.Errorf("network error"))

	err := p.RemoveAgent(context.Background(), &woodpecker.Agent{Name: "wp-agent-1"})
	assert.ErrorContains(t, err, "FindProjectDevicesByTag")
	m.AssertExpectations(t)
}

func TestListDeployedAgentNames(t *testing.T) {
	m := &mockDevicesAPI{}
	p := testProvider(m)

	d1, d2 := metalv1.Device{}, metalv1.Device{}
	d1.SetHostname("wp-agent-1")
	d2.SetHostname("wp-agent-2")

	m.On("FindProjectDevicesByTag", mock.Anything, testProjectID, poolTag()).
		Return([]metalv1.Device{d1, d2}, nil)

	names, err := p.ListDeployedAgentNames(context.Background())
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"wp-agent-1", "wp-agent-2"}, names)
	m.AssertExpectations(t)
}

func TestListDeployedAgentNames_Empty(t *testing.T) {
	m := &mockDevicesAPI{}
	p := testProvider(m)

	m.On("FindProjectDevicesByTag", mock.Anything, testProjectID, poolTag()).
		Return([]metalv1.Device{}, nil)

	names, err := p.ListDeployedAgentNames(context.Background())
	assert.NoError(t, err)
	assert.Empty(t, names)
	m.AssertExpectations(t)
}
