package equinixmetal

import (
	"context"
	"errors"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type fakeDevicesService struct {
	createFn func(context.Context, string, deviceCreateRequest) (*deviceRecord, error)
	listFn   func(context.Context, string, string) ([]deviceRecord, error)
	deleteFn func(context.Context, string, bool) error
}

func (f *fakeDevicesService) Create(ctx context.Context, projectID string, req deviceCreateRequest) (*deviceRecord, error) {
	if f.createFn == nil {
		return nil, nil
	}
	return f.createFn(ctx, projectID, req)
}

func (f *fakeDevicesService) List(ctx context.Context, projectID, tag string) ([]deviceRecord, error) {
	if f.listFn == nil {
		return nil, nil
	}
	return f.listFn(ctx, projectID, tag)
}

func (f *fakeDevicesService) Delete(ctx context.Context, deviceID string, force bool) error {
	if f.deleteFn == nil {
		return nil
	}
	return f.deleteFn(ctx, deviceID, force)
}

func TestDeployAgentInvalidUserData(t *testing.T) {
	p := &Provider{
		config:           &config.Config{},
		userDataTemplate: template.Must(template.New("").Parse("{{.InvalidField}}")),
		devices:          &fakeDevicesService{},
	}

	err := p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RenderUserDataTemplate")
}

func TestDeployAgentCreatesDeviceWithExpectedFields(t *testing.T) {
	t.Parallel()

	var got deviceCreateRequest
	var gotProjectID string
	p := &Provider{
		name:         "equinixmetal",
		projectID:    "project-123",
		metro:        "sv",
		plans:        []string{"c3.small.x86"},
		operatingSys: "ubuntu_24_04",
		billingCycle: "hourly",
		tags: []string{
			"team=ci",
		},
		projectSSHKeys: []string{"ssh-key-1"},
		spotInstance:   true,
		spotPriceMax:   1.25,
		config: &config.Config{
			PoolID: "pool-7",
			Image:  "woodpeckerci/woodpecker-agent:next",
		},
		userDataTemplate: template.Must(template.New("").Parse("#!/bin/sh\necho ready")),
		devices: &fakeDevicesService{createFn: func(_ context.Context, projectID string, req deviceCreateRequest) (*deviceRecord, error) {
			gotProjectID = projectID
			got = req
			return &deviceRecord{ID: "dev-1", Hostname: req.Hostname}, nil
		}},
	}

	err := p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	require.NoError(t, err)
	assert.Equal(t, "project-123", gotProjectID)
	assert.Equal(t, "agent-1", got.Hostname)
	assert.Equal(t, "c3.small.x86", got.Plan)
	assert.Equal(t, "ubuntu_24_04", got.OperatingSys)
	assert.Equal(t, "hourly", got.BillingCycle)
	assert.Equal(t, "sv", got.Metro)
	assert.Empty(t, got.Facility)
	assert.Equal(t, []string{"ssh-key-1"}, got.ProjectSSHKeys)
	assert.True(t, got.SpotInstance)
	assert.Equal(t, 1.25, got.SpotPriceMax)
	assert.Contains(t, got.Tags, "team=ci")
	assert.Contains(t, got.Tags, engine.LabelPool+"=pool-7")
	assert.Contains(t, got.Tags, engine.LabelImage+"=woodpeckerci/woodpecker-agent:next")
	assert.Contains(t, got.UserData, "echo ready")
}

func TestListDeployedAgentNamesReturnsPoolDevices(t *testing.T) {
	p := &Provider{
		name:      "equinixmetal",
		projectID: "project-123",
		config:    &config.Config{PoolID: "pool-7"},
		devices: &fakeDevicesService{listFn: func(_ context.Context, projectID, tag string) ([]deviceRecord, error) {
			require.Equal(t, "project-123", projectID)
			require.Equal(t, engine.LabelPool+"=pool-7", tag)
			return []deviceRecord{
				{Hostname: "agent-1", Tags: []string{engine.LabelPool + "=pool-7"}},
				{Hostname: "agent-2", Tags: []string{engine.LabelPool + "=pool-7"}},
			}, nil
		}},
	}

	names, err := p.ListDeployedAgentNames(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"agent-1", "agent-2"}, names)
}

func TestRemoveAgentDeletesMatchingPoolDevice(t *testing.T) {
	t.Parallel()

	var deletedID string
	p := &Provider{
		name:      "equinixmetal",
		projectID: "project-123",
		config:    &config.Config{PoolID: "pool-7"},
		devices: &fakeDevicesService{
			listFn: func(_ context.Context, _, _ string) ([]deviceRecord, error) {
				return []deviceRecord{{ID: "dev-1", Hostname: "agent-1", Tags: []string{engine.LabelPool + "=pool-7"}}}, nil
			},
			deleteFn: func(_ context.Context, deviceID string, force bool) error {
				deletedID = deviceID
				assert.False(t, force)
				return nil
			},
		},
	}

	err := p.RemoveAgent(context.Background(), &woodpecker.Agent{Name: "agent-1"})
	require.NoError(t, err)
	assert.Equal(t, "dev-1", deletedID)
}

func TestRemoveAgentReturnsErrorOnDuplicateHostnames(t *testing.T) {
	p := &Provider{
		name:      "equinixmetal",
		projectID: "project-123",
		config:    &config.Config{PoolID: "pool-7"},
		devices: &fakeDevicesService{listFn: func(_ context.Context, _, _ string) ([]deviceRecord, error) {
			return []deviceRecord{
				{ID: "dev-1", Hostname: "agent-1", Tags: []string{engine.LabelPool + "=pool-7"}},
				{ID: "dev-2", Hostname: "agent-1", Tags: []string{engine.LabelPool + "=pool-7"}},
			}, nil
		}},
	}

	err := p.RemoveAgent(context.Background(), &woodpecker.Agent{Name: "agent-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple devices")
}

func TestListDeployedAgentNamesPropagatesErrors(t *testing.T) {
	p := &Provider{
		name:      "equinixmetal",
		projectID: "project-123",
		config:    &config.Config{PoolID: "pool-7"},
		devices: &fakeDevicesService{listFn: func(_ context.Context, _, _ string) ([]deviceRecord, error) {
			return nil, errors.New("boom")
		}},
	}

	names, err := p.ListDeployedAgentNames(context.Background())
	require.Error(t, err)
	assert.Nil(t, names)
	assert.Contains(t, err.Error(), "boom")
}

func TestValidateRequiresExactlyOneLocation(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		wantErr  error
	}{
		{
			name: "missing location",
			provider: Provider{
				projectID:    "project-123",
				plans:        []string{"c3.small.x86"},
				operatingSys: "ubuntu_24_04",
			},
			wantErr: ErrLocationRequired,
		},
		{
			name: "metro and facility conflict",
			provider: Provider{
				projectID:    "project-123",
				plans:        []string{"c3.small.x86"},
				operatingSys: "ubuntu_24_04",
				metro:        "sv",
				facility:     []string{"sv15"},
			},
			wantErr: ErrLocationConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.provider.validate()
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestValidateRejectsReservedTagPrefix(t *testing.T) {
	p := Provider{
		projectID:    "project-123",
		plans:        []string{"c3.small.x86"},
		operatingSys: "ubuntu_24_04",
		metro:        "sv",
		tags:         []string{engine.LabelPool + "=override"},
	}

	err := p.validate()
	require.ErrorIs(t, err, ErrReservedTagPrefix)
}
