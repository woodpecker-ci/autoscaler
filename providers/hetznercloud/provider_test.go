package hetznercloud

import (
	"testing"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/providers/hetznercloud/hcapi/mocks"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

func TestDeployAgent(t *testing.T) {
	tests := []struct {
		name          string
		setupMocks    func(*mocks.MockClient)
		sshkeys       []string
		expectedError string
		serverType    []string
	}{
		{
			name: "ServerTypeNotFound",
			setupMocks: func(mockClient *mocks.MockClient) {
				mockServerTypeClient := mocks.NewMockServerTypeClient(t)
				mockServerTypeClient.On("GetByName", mock.Anything, mock.Anything).Return(nil, nil, nil)
				mockClient.On("ServerType").Return(mockServerTypeClient)
			},
			serverType:    []string{"cx11"},
			expectedError: ErrServerTypeNotFound.Error(),
		},
		{
			name: "ImageNotFound",
			setupMocks: func(mockClient *mocks.MockClient) {
				mockServerType := &hcloud.ServerType{Name: "cx11", Architecture: "x86"}
				mockServerTypeClient := mocks.NewMockServerTypeClient(t)
				mockServerTypeClient.On("GetByName", mock.Anything, mock.Anything).Return(mockServerType, nil, nil)
				mockClient.On("ServerType").Return(mockServerTypeClient)

				mockImageClient := mocks.NewMockImageClient(t)
				mockImageClient.On("GetByNameAndArchitecture", mock.Anything, mock.Anything, hcloud.ArchitectureX86).Return(nil, nil, nil)
				mockClient.On("Image").Return(mockImageClient)
			},
			serverType:    []string{"cx11"},
			expectedError: ErrImageNotFound.Error(),
		},
		{
			name: "ServerTypeWithLocation",
			setupMocks: func(mockClient *mocks.MockClient) {
				// The mocked server type must advertise nbg1 in its
				// Locations list, otherwise the provider's
				// serverTypeSupportsLocation filter throws it away
				// before the deploy is attempted.
				mockServerType := &hcloud.ServerType{
					Name:         "cx11",
					Architecture: "x86",
					Locations: []hcloud.ServerTypeLocation{
						{Location: &hcloud.Location{Name: "nbg1"}},
					},
				}
				mockServerTypeClient := mocks.NewMockServerTypeClient(t)
				mockServerTypeClient.On("GetByName", mock.Anything, "cx11").Return(mockServerType, nil, nil)
				mockClient.On("ServerType").Return(mockServerTypeClient)

				mockImage := &hcloud.Image{}
				mockImageClient := mocks.NewMockImageClient(t)
				mockImageClient.On("GetByNameAndArchitecture", mock.Anything, mock.Anything, hcloud.ArchitectureX86).Return(mockImage, nil, nil)
				mockClient.On("Image").Return(mockImageClient)

				mockServerClient := mocks.NewMockServerClient(t)
				mockServerClient.On("Create", mock.Anything, mock.MatchedBy(func(opts hcloud.ServerCreateOpts) bool {
					return opts.ServerType == mockServerType &&
						opts.ServerType.Name == "cx11" &&
						opts.Location.Name == "nbg1"
				})).Return(hcloud.ServerCreateResult{Server: &hcloud.Server{}}, &hcloud.Response{}, nil)
				mockClient.On("Server").Return(mockServerClient)
			},
			serverType: []string{"cx11:nbg1"},
		},
		{
			// First candidate's location is unavailable; provider should
			// log and fall through to the second candidate, which succeeds.
			name: "FallbackOnUnavailable",
			setupMocks: func(mockClient *mocks.MockClient) {
				st1 := &hcloud.ServerType{
					Name: "cx11", Architecture: "x86",
					Locations: []hcloud.ServerTypeLocation{
						{Location: &hcloud.Location{Name: "nbg1"}},
					},
				}
				st2 := &hcloud.ServerType{
					Name: "cx21", Architecture: "x86",
					Locations: []hcloud.ServerTypeLocation{
						{Location: &hcloud.Location{Name: "fsn1"}},
					},
				}
				mockServerTypeClient := mocks.NewMockServerTypeClient(t)
				mockServerTypeClient.On("GetByName", mock.Anything, "cx11").Return(st1, nil, nil)
				mockServerTypeClient.On("GetByName", mock.Anything, "cx21").Return(st2, nil, nil)
				mockClient.On("ServerType").Return(mockServerTypeClient)

				mockImageClient := mocks.NewMockImageClient(t)
				mockImageClient.On("GetByNameAndArchitecture", mock.Anything, mock.Anything, hcloud.ArchitectureX86).Return(&hcloud.Image{}, nil, nil)
				mockClient.On("Image").Return(mockImageClient)

				unavailable := hcloud.Error{Code: hcloud.ErrorCodeResourceUnavailable, Message: "unavailable"}
				mockServerClient := mocks.NewMockServerClient(t)
				mockServerClient.On("Create", mock.Anything, mock.MatchedBy(func(opts hcloud.ServerCreateOpts) bool {
					return opts.ServerType.Name == "cx11"
				})).Return(hcloud.ServerCreateResult{}, &hcloud.Response{}, unavailable).Once()
				mockServerClient.On("Create", mock.Anything, mock.MatchedBy(func(opts hcloud.ServerCreateOpts) bool {
					return opts.ServerType.Name == "cx21"
				})).Return(hcloud.ServerCreateResult{Server: &hcloud.Server{}}, &hcloud.Response{}, nil).Once()
				mockClient.On("Server").Return(mockServerClient)
			},
			serverType: []string{"cx11:nbg1", "cx21:fsn1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := mocks.NewMockClient(t)
			tt.setupMocks(mockClient)

			p := &provider{
				client:  mockClient,
				config:  &config.Config{},
				sshKeys: tt.sshkeys,
			}

			var err error
			if tt.serverType != nil {
				err = p.resolveServerConfigs(t.Context(), tt.serverType, "ubuntu-24.04")
			}
			if err == nil {
				agent := &woodpecker.Agent{}
				err = p.DeployAgent(t.Context(), agent)
			}

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
