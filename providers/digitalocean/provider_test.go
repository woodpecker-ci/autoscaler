package digitalocean

import (
	"context"
	"errors"
	"testing"
	"text/template"

	"github.com/digitalocean/godo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// mockDroplets implements DropletsService for testing.
type mockDroplets struct {
	mock.Mock
}

func (m *mockDroplets) Create(ctx context.Context, req *godo.DropletCreateRequest) (*godo.Droplet, *godo.Response, error) {
	args := m.Called(ctx, req)
	var d *godo.Droplet
	if v := args.Get(0); v != nil {
		d = v.(*godo.Droplet)
	}
	var r *godo.Response
	if v := args.Get(1); v != nil {
		r = v.(*godo.Response)
	}
	return d, r, args.Error(2)
}

func (m *mockDroplets) Delete(ctx context.Context, id int) (*godo.Response, error) {
	args := m.Called(ctx, id)
	var r *godo.Response
	if v := args.Get(0); v != nil {
		r = v.(*godo.Response)
	}
	return r, args.Error(1)
}

func (m *mockDroplets) List(ctx context.Context, opt *godo.ListOptions) ([]godo.Droplet, *godo.Response, error) {
	args := m.Called(ctx, opt)
	var d []godo.Droplet
	if v := args.Get(0); v != nil {
		d = v.([]godo.Droplet)
	}
	var r *godo.Response
	if v := args.Get(1); v != nil {
		r = v.(*godo.Response)
	}
	return d, r, args.Error(2)
}

func TestDeployAgent(t *testing.T) {
	tests := []struct {
		name          string
		setupMocks    func(*mockDroplets)
		userdata      string
		expectedError string
	}{
		{
			name:          "InvalidUserData",
			setupMocks:    func(_ *mockDroplets) {},
			userdata:      "{{.InvalidField}}",
			expectedError: "RenderUserDataTemplate",
		},
		{
			name: "Success",
			setupMocks: func(md *mockDroplets) {
				md.On("Create", mock.Anything, mock.MatchedBy(func(req *godo.DropletCreateRequest) bool {
					return req.Name == "test-agent" &&
						req.Region == "nyc1" &&
						req.Size == "s-1vcpu-1gb" &&
						req.Image.Slug == "ubuntu-22-04-x64"
				})).Return(&godo.Droplet{ID: 123}, &godo.Response{}, nil)
			},
		},
		{
			name: "CreateFails",
			setupMocks: func(md *mockDroplets) {
				md.On("Create", mock.Anything, mock.Anything).
					Return(nil, nil, errors.New("API error"))
			},
			expectedError: "Droplets.Create",
		},
		{
			name: "WithSSHKeys",
			setupMocks: func(md *mockDroplets) {
				md.On("Create", mock.Anything, mock.MatchedBy(func(req *godo.DropletCreateRequest) bool {
					return len(req.SSHKeys) == 2 &&
						req.SSHKeys[0].Fingerprint == "fp1" &&
						req.SSHKeys[1].Fingerprint == "fp2"
				})).Return(&godo.Droplet{ID: 124}, &godo.Response{}, nil)
			},
		},
		{
			name: "WithVPCUUID",
			setupMocks: func(md *mockDroplets) {
				md.On("Create", mock.Anything, mock.MatchedBy(func(req *godo.DropletCreateRequest) bool {
					return req.VPCUUID == "vpc-123"
				})).Return(&godo.Droplet{ID: 125}, &godo.Response{}, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := new(mockDroplets)
			tt.setupMocks(md)

			p := &Provider{
				name:     "digitalocean",
				region:   "nyc1",
				size:     "s-1vcpu-1gb",
				image:    "ubuntu-22-04-x64",
				config:   &config.Config{},
				droplets: md,
				tags: []string{
					engine.LabelPool + "=test-pool",
					engine.LabelImage + "=ubuntu-22-04-x64",
				},
				userDataTemplate: template.Must(template.New("").Parse(tt.userdata)),
			}

			if tt.name == "WithSSHKeys" {
				p.sshKeys = []string{"fp1", "fp2"}
			}
			if tt.name == "WithVPCUUID" {
				p.vpcUUID = "vpc-123"
			}

			agent := &woodpecker.Agent{Name: "test-agent"}
			err := p.DeployAgent(t.Context(), agent)
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRemoveAgent(t *testing.T) {
	tests := []struct {
		name          string
		setupMocks    func(*mockDroplets)
		expectedError string
	}{
		{
			name: "Success",
			setupMocks: func(md *mockDroplets) {
				md.On("List", mock.Anything, mock.Anything).Return(
					[]godo.Droplet{{ID: 123, Name: "test-agent"}},
					&godo.Response{},
					nil,
				)
				md.On("Delete", mock.Anything, 123).
					Return(&godo.Response{}, nil)
			},
		},
		{
			name: "NotFound",
			setupMocks: func(md *mockDroplets) {
				md.On("List", mock.Anything, mock.Anything).Return(
					[]godo.Droplet{},
					&godo.Response{},
					nil,
				)
			},
		},
		{
			name: "ListFails",
			setupMocks: func(md *mockDroplets) {
				md.On("List", mock.Anything, mock.Anything).Return(
					nil, nil, errors.New("API error"),
				)
			},
			expectedError: "findDropletByName",
		},
		{
			name: "DeleteFails",
			setupMocks: func(md *mockDroplets) {
				md.On("List", mock.Anything, mock.Anything).Return(
					[]godo.Droplet{{ID: 123, Name: "test-agent"}},
					&godo.Response{},
					nil,
				)
				md.On("Delete", mock.Anything, 123).
					Return(nil, errors.New("API error"))
			},
			expectedError: "Droplets.Delete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := new(mockDroplets)
			tt.setupMocks(md)

			p := &Provider{
				name:     "digitalocean",
				config:   &config.Config{},
				droplets: md,
			}

			agent := &woodpecker.Agent{Name: "test-agent"}
			err := p.RemoveAgent(t.Context(), agent)
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestListDeployedAgentNames(t *testing.T) {
	tests := []struct {
		name          string
		setupMocks    func(*mockDroplets)
		expected      []string
		expectedError string
	}{
		{
			name: "ReturnsMatchingAgents",
			setupMocks: func(md *mockDroplets) {
				md.On("List", mock.Anything, mock.Anything).Return(
					[]godo.Droplet{
						{Name: "agent-1", Tags: []string{engine.LabelPool + "=test-pool"}},
						{Name: "agent-2", Tags: []string{engine.LabelPool + "=test-pool"}},
						{Name: "other", Tags: []string{engine.LabelPool + "=other-pool"}},
					},
					&godo.Response{},
					nil,
				)
			},
			expected: []string{"agent-1", "agent-2"},
		},
		{
			name: "EmptyList",
			setupMocks: func(md *mockDroplets) {
				md.On("List", mock.Anything, mock.Anything).Return(
					[]godo.Droplet{},
					&godo.Response{},
					nil,
				)
			},
			expected: nil,
		},
		{
			name: "ListFails",
			setupMocks: func(md *mockDroplets) {
				md.On("List", mock.Anything, mock.Anything).Return(
					nil, nil, errors.New("API error"),
				)
			},
			expectedError: "Droplets.List",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := new(mockDroplets)
			tt.setupMocks(md)

			p := &Provider{
				name:     "digitalocean",
				config:   &config.Config{PoolID: "test-pool"},
				droplets: md,
			}

			names, err := p.ListDeployedAgentNames(t.Context())
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, names)
			}
		})
	}
}
