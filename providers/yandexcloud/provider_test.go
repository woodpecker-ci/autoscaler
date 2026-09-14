package yandexcloud

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/autoscaler/providers/yandexcloud/ycapi"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type fakeClient struct {
	image        ycapi.Image
	subnet       ycapi.Subnet
	instances    []ycapi.Instance
	createCalls  []ycapi.CreateRequest
	deleteCalls  []string
	getImageErr  error
	getSubnetErr error
	listErr      error
	createErr    error
	deleteErr    error
}

func (f *fakeClient) GetImage(context.Context, string, string, string) (ycapi.Image, error) {
	if f.getImageErr != nil {
		return ycapi.Image{}, f.getImageErr
	}
	return f.image, nil
}

func (f *fakeClient) GetSubnet(context.Context, string) (ycapi.Subnet, error) {
	if f.getSubnetErr != nil {
		return ycapi.Subnet{}, f.getSubnetErr
	}
	return f.subnet, nil
}

func (f *fakeClient) Create(_ context.Context, request ycapi.CreateRequest) (ycapi.Instance, error) {
	f.createCalls = append(f.createCalls, request)
	if f.createErr != nil {
		return ycapi.Instance{}, f.createErr
	}
	return ycapi.Instance{ID: "instance-1", Name: request.Name, Labels: request.Labels}, nil
}

func (f *fakeClient) Delete(_ context.Context, instanceID string) error {
	f.deleteCalls = append(f.deleteCalls, instanceID)
	return f.deleteErr
}

func (f *fakeClient) List(context.Context, string) ([]ycapi.Instance, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.instances, nil
}

func testProvider(t *testing.T, client *fakeClient) *provider {
	t.Helper()

	p, err := newProvider(t.Context(), &config.Config{PoolID: "1"}, settings{
		folderID:         "folder-1",
		subnetID:         "subnet-1",
		imageID:          "image-1",
		platformID:       "standard-v3",
		cores:            2,
		memory:           4 * 1024 * 1024 * 1024,
		coreFraction:     100,
		diskType:         "network-hdd",
		diskSize:         20 * 1024 * 1024 * 1024,
		publicIPv4:       true,
		labels:           map[string]string{"team": "ci"},
		operationTimeout: time.Minute,
	}, client)
	require.NoError(t, err)
	typedProvider, ok := p.(*provider)
	require.True(t, ok)
	return typedProvider
}

func testCommand(t *testing.T, args ...string) *cli.Command {
	t.Helper()
	arguments := append([]string{"autoscaler"}, args...)
	var captured *cli.Command
	command := &cli.Command{
		Name:  "autoscaler",
		Flags: ProviderFlags,
		Action: func(_ context.Context, command *cli.Command) error {
			captured = command
			return nil
		},
	}
	require.NoError(t, command.Run(t.Context(), arguments))
	return captured
}

func TestBillingModel(t *testing.T) {
	assert.Equal(t, types.BillingPerSecond, (&provider{}).BillingModel())
}

func TestSettingsValidation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want error
	}{
		{
			name: "credentials required",
			args: []string{
				"--yandexcloud-folder-id=folder-1",
				"--yandexcloud-subnet-id=subnet-1",
				"--yandexcloud-image-id=image-1",
			},
			want: ErrCredentialsRequired,
		},
		{
			name: "credential conflict",
			args: []string{
				"--yandexcloud-folder-id=folder-1",
				"--yandexcloud-subnet-id=subnet-1",
				"--yandexcloud-image-id=image-1",
				"--yandexcloud-iam-token=token",
				"--yandexcloud-service-account-key-file=key.json",
			},
			want: ErrCredentialConflict,
		},
		{
			name: "image selectors conflict",
			args: []string{
				"--yandexcloud-iam-token=token",
				"--yandexcloud-folder-id=folder-1",
				"--yandexcloud-subnet-id=subnet-1",
				"--yandexcloud-image-id=image-1",
				"--yandexcloud-image-family=ubuntu",
			},
			want: ErrImageSelectorConflict,
		},
		{
			name: "reserved label",
			args: []string{
				"--yandexcloud-iam-token=token",
				"--yandexcloud-folder-id=folder-1",
				"--yandexcloud-subnet-id=subnet-1",
				"--yandexcloud-image-id=image-1",
				"--yandexcloud-labels=wp.autoscaler/pool=foreign",
			},
			want: ErrReservedLabelPrefix,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := testCommand(t, test.args...)
			_, err := settingsFromCommand(command, "1")
			require.Error(t, err)
			assert.ErrorIs(t, err, test.want)
		})
	}
}

func TestSettingsParseSizesAndCredentialMode(t *testing.T) {
	command := testCommand(t,
		"--yandexcloud-iam-token=token",
		"--yandexcloud-folder-id=folder-1",
		"--yandexcloud-subnet-id=subnet-1",
		"--yandexcloud-image-family=ubuntu-24-04",
		"--yandexcloud-memory=4GiB",
		"--yandexcloud-disk-size=20GiB",
		"--yandexcloud-labels=team=ci",
	)

	settings, err := settingsFromCommand(command, "1")
	require.NoError(t, err)
	assert.Equal(t, int64(4*1024*1024*1024), settings.memory)
	assert.Equal(t, int64(20*1024*1024*1024), settings.diskSize)
	assert.Equal(t, "iam-token", settings.credentialMode)
	assert.Equal(t, map[string]string{"team": "ci"}, settings.labels)
}

func TestDeployAgentBuildsYandexRequest(t *testing.T) {
	client := &fakeClient{
		image:  ycapi.Image{ID: "image-1", Status: "READY", MinDiskSize: 10 * 1024 * 1024 * 1024},
		subnet: ycapi.Subnet{ID: "subnet-1", FolderID: "folder-1", ZoneID: "ru-central1-b"},
	}
	p := testProvider(t, client)
	p.config = &config.Config{
		PoolID:            "1",
		GRPCAddress:       "grpc.example.com:9000",
		Image:             "woodpecker-agent:test",
		WorkflowsPerAgent: 2,
	}

	err := p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "pool-1-agent-abcd", Token: "secret"})
	require.NoError(t, err)
	require.Len(t, client.createCalls, 1)

	request := client.createCalls[0]
	assert.Equal(t, "folder-1", request.FolderID)
	assert.Equal(t, "ru-central1-b", request.ZoneID)
	assert.Equal(t, "pool-1-agent-abcd", request.Name)
	assert.Equal(t, int64(2), request.Cores)
	assert.Equal(t, "image-1", request.ImageID)
	assert.True(t, request.PublicIPv4)
	assert.Equal(t, "ci", request.Labels["team"])
	assert.Equal(t, "1", request.Labels[engine.LabelPool])
	assert.Contains(t, request.UserData, "169.254.169.254/32")
}

func TestListAndRemoveArePoolScoped(t *testing.T) {
	client := &fakeClient{
		image:  ycapi.Image{ID: "image-1", Status: "READY"},
		subnet: ycapi.Subnet{ID: "subnet-1", FolderID: "folder-1", ZoneID: "ru-central1-b"},
		instances: []ycapi.Instance{
			{ID: "agent-1", Name: "pool-1-agent-a", Labels: map[string]string{engine.LabelPool: "1"}, Status: "RUNNING"},
			{ID: "foreign", Name: "pool-1-agent-b", Labels: map[string]string{engine.LabelPool: "2"}, Status: "RUNNING"},
			{ID: "deleting", Name: "pool-1-agent-c", Labels: map[string]string{engine.LabelPool: "1"}, Status: "DELETING"},
		},
	}
	p := testProvider(t, client)

	names, err := p.ListDeployedAgentNames(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"pool-1-agent-a"}, names)

	err = p.RemoveAgent(t.Context(), &woodpecker.Agent{Name: "pool-1-agent-a"})
	require.NoError(t, err)
	assert.Equal(t, []string{"agent-1"}, client.deleteCalls)

	err = p.RemoveAgent(t.Context(), &woodpecker.Agent{Name: "pool-1-agent-b"})
	require.NoError(t, err)
	assert.Equal(t, []string{"agent-1"}, client.deleteCalls)
}

func TestRemoveRejectsDuplicateInstances(t *testing.T) {
	client := &fakeClient{
		image:  ycapi.Image{ID: "image-1", Status: "READY"},
		subnet: ycapi.Subnet{ID: "subnet-1", FolderID: "folder-1", ZoneID: "ru-central1-b"},
		instances: []ycapi.Instance{
			{ID: "agent-1", Name: "pool-1-agent-a", Labels: map[string]string{engine.LabelPool: "1"}},
			{ID: "agent-2", Name: "pool-1-agent-a", Labels: map[string]string{engine.LabelPool: "1"}},
		},
	}
	p := testProvider(t, client)

	err := p.RemoveAgent(t.Context(), &woodpecker.Agent{Name: "pool-1-agent-a"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDuplicateInstance)
}

func TestNewProviderRejectsWrongSubnetFolder(t *testing.T) {
	client := &fakeClient{
		image:  ycapi.Image{ID: "image-1", Status: "READY"},
		subnet: ycapi.Subnet{ID: "subnet-1", FolderID: "other-folder", ZoneID: "ru-central1-b"},
	}

	_, err := newProvider(t.Context(), &config.Config{PoolID: "1"}, settings{
		folderID:         "folder-1",
		subnetID:         "subnet-1",
		imageID:          "image-1",
		diskSize:         20 * 1024 * 1024 * 1024,
		operationTimeout: time.Minute,
	}, client)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSubnetFolderMismatch)
}

func TestParseSizeRejectsInvalidValues(t *testing.T) {
	_, err := parseSize("not-a-size", 1)
	require.Error(t, err)

	_, err = parseSize("0GiB", 1)
	require.Error(t, err)
}
