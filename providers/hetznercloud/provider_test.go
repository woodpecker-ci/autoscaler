package hetznercloud

import (
	"testing"
	"text/template"

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
		userdata      string
		sshkeys       []string
		expectedError string
		serverType    []string
	}{
		{
			name:          "InvalidUserData",
			setupMocks:    func(_ *mocks.MockClient) {},
			userdata:      "{{.InvalidField}}",
			expectedError: "RenderUserDataTemplate",
		},
		{
			name: "ServerTypeNotFound",
			setupMocks: func(mockClient *mocks.MockClient) {
				mockServerTypeClient := mocks.NewMockServerTypeClient(t)
				mockServerTypeClient.On("GetByName", mock.Anything, mock.Anything).Return(nil, nil, nil)
				mockClient.On("ServerType").Return(mockServerTypeClient)
			},
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
			expectedError: ErrImageNotFound.Error(),
		},
		{
			name: "ServerTypeWithLocation",
			setupMocks: func(mockClient *mocks.MockClient) {
				mockServerType := &hcloud.ServerType{Name: "cx11", Architecture: "x86"}
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := mocks.NewMockClient(t)
			tt.setupMocks(mockClient)

			provider := &Provider{
				client:     mockClient,
				config:     &config.Config{},
				userData:   template.Must(template.New("").Parse(tt.userdata)),
				sshKeys:    tt.sshkeys,
				serverType: []string{"cx11"},
			}

			if tt.serverType != nil {
				provider.serverType = tt.serverType
			}

			agent := &woodpecker.Agent{}
			err := provider.DeployAgent(t.Context(), agent)
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
