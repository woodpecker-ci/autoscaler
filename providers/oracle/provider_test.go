package oracle

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/utils"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type fakeComputeAPI struct {
	launchFn    func(context.Context, core.LaunchInstanceRequest) (core.LaunchInstanceResponse, error)
	terminateFn func(context.Context, core.TerminateInstanceRequest) (core.TerminateInstanceResponse, error)
	listFn      func(context.Context, core.ListInstancesRequest) (core.ListInstancesResponse, error)
	listImgFn   func(context.Context, core.ListImagesRequest) (core.ListImagesResponse, error)
}

func (f *fakeComputeAPI) LaunchInstance(ctx context.Context, req core.LaunchInstanceRequest) (core.LaunchInstanceResponse, error) {
	if f.launchFn == nil {
		return core.LaunchInstanceResponse{}, nil
	}
	return f.launchFn(ctx, req)
}

func (f *fakeComputeAPI) TerminateInstance(ctx context.Context, req core.TerminateInstanceRequest) (core.TerminateInstanceResponse, error) {
	if f.terminateFn == nil {
		return core.TerminateInstanceResponse{}, nil
	}
	return f.terminateFn(ctx, req)
}

func (f *fakeComputeAPI) ListInstances(ctx context.Context, req core.ListInstancesRequest) (core.ListInstancesResponse, error) {
	if f.listFn == nil {
		return core.ListInstancesResponse{}, nil
	}
	return f.listFn(ctx, req)
}

func (f *fakeComputeAPI) ListImages(ctx context.Context, req core.ListImagesRequest) (core.ListImagesResponse, error) {
	if f.listImgFn == nil {
		return core.ListImagesResponse{}, nil
	}
	return f.listImgFn(ctx, req)
}

func newTestCommand(t *testing.T, args []string) *cli.Command {
	t.Helper()

	var captured *cli.Command
	cmd := &cli.Command{
		Flags: ProviderFlags,
		Action: func(_ context.Context, c *cli.Command) error {
			captured = c
			return nil
		},
	}

	require.NoError(t, cmd.Run(t.Context(), append([]string{"test"}, args...)))
	require.NotNil(t, captured)

	return captured
}

func newTestProvider(client computeAPI) *provider {
	return &provider{
		name:               "oracle",
		config:             &config.Config{PoolID: "pool-1", GRPCAddress: "grpc.example.com", Image: "woodpeckerci/woodpecker-agent:next"},
		client:             client,
		compartmentID:      "ocid1.compartment.oc1..aaa",
		availabilityDomain: "Uocm:EU-FRANKFURT-1-AD-1",
		subnetID:           "ocid1.subnet.oc1..bbb",
		shape:              "VM.Standard.E4.Flex",
		ocpus:              2,
		memoryGBs:          16,
		imageID:            "ocid1.image.oc1..ccc",
		sshAuthorizedKeys:  "ssh-ed25519 AAAA test",
		assignPublicIP:     true,
		tags:               map[string]string{tagPool: "pool-1", tagImage: "ocid1.image.oc1..ccc", "team": "ci"},
	}
}

func TestNewValidatesConfig(t *testing.T) {
	base := []string{
		"--oracle-compartment-id=ocid1.compartment.oc1..aaa",
		"--oracle-availability-domain=Uocm:EU-FRANKFURT-1-AD-1",
		"--oracle-subnet-id=ocid1.subnet.oc1..bbb",
	}

	tests := []struct {
		name string
		args []string
		err  error
	}{
		{name: "missing compartment", args: base[1:], err: ErrCompartmentIDRequired},
		{name: "missing availability domain", args: []string{base[0], base[2]}, err: ErrAvailabilityDomainRequired},
		{name: "missing subnet", args: base[:2], err: ErrSubnetIDRequired},
		{name: "empty shape", args: append(base, "--oracle-shape="), err: ErrShapeRequired},
		{name: "no image", args: append(base, "--oracle-operating-system="), err: ErrImageRequired},
		{name: "reserved tag", args: append(base, "--oracle-freeform-tags="+tagPrefix+"x=y"), err: ErrReservedTagPrefix},
		{name: "invalid tag key", args: append(base, "--oracle-freeform-tags=a.b=c"), err: ErrInvalidTag},
		{name: "incomplete credentials", args: append(base, "--oracle-tenancy-id=ocid1.tenancy.oc1..t"), err: ErrIncompleteCredentials},
		{
			name: "credentials without region",
			args: append(base,
				"--oracle-tenancy-id=ocid1.tenancy.oc1..t",
				"--oracle-user-id=ocid1.user.oc1..u",
				"--oracle-fingerprint=aa:bb",
				"--oracle-private-key=key",
			),
			err: ErrRegionRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(t.Context(), newTestCommand(t, tt.args), &config.Config{PoolID: "pool-1"})
			require.ErrorIs(t, err, tt.err)
		})
	}
}

func TestParseFreeformTags(t *testing.T) {
	tags, err := parseFreeformTags([]string{"team=ci", "env=prod"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"team": "ci", "env": "prod"}, tags)

	_, err = parseFreeformTags([]string{"invalid"})
	require.ErrorIs(t, err, ErrInvalidTag)

	_, err = parseFreeformTags([]string{"my key=v"})
	require.ErrorIs(t, err, ErrInvalidTag)

	_, err = parseFreeformTags([]string{tagPool + "=other"})
	require.ErrorIs(t, err, ErrReservedTagPrefix)
}

func TestNewConfigurationProviderUsesAPIKeyCredentials(t *testing.T) {
	cmd := newTestCommand(t, []string{
		"--oracle-tenancy-id=ocid1.tenancy.oc1..t",
		"--oracle-user-id=ocid1.user.oc1..u",
		"--oracle-fingerprint=aa:bb",
		"--oracle-private-key=key",
		"--oracle-region=eu-frankfurt-1",
	})

	cp, err := newConfigurationProvider(cmd)
	require.NoError(t, err)

	tenancy, err := cp.TenancyOCID()
	require.NoError(t, err)
	assert.Equal(t, "ocid1.tenancy.oc1..t", tenancy)
	user, err := cp.UserOCID()
	require.NoError(t, err)
	assert.Equal(t, "ocid1.user.oc1..u", user)
	region, err := cp.Region()
	require.NoError(t, err)
	assert.Equal(t, "eu-frankfurt-1", region)
}

func TestSetupResolvesImageAndTags(t *testing.T) {
	var got core.ListImagesRequest
	p := newTestProvider(&fakeComputeAPI{
		listImgFn: func(_ context.Context, req core.ListImagesRequest) (core.ListImagesResponse, error) {
			got = req
			return core.ListImagesResponse{Items: []core.Image{{Id: utils.ToPtr("ocid1.image.oc1..resolved")}}}, nil
		},
	})
	p.imageID = ""
	p.tags = nil

	require.NoError(t, p.setup(t.Context(), "Canonical Ubuntu", "24.04", map[string]string{"team": "ci"}))

	assert.Equal(t, "ocid1.image.oc1..resolved", p.imageID)
	assert.Equal(t, "Canonical Ubuntu", *got.OperatingSystem)
	assert.Equal(t, "24.04", *got.OperatingSystemVersion)
	assert.Equal(t, "VM.Standard.E4.Flex", *got.Shape)
	assert.Equal(t, core.ListImagesSortByTimecreated, got.SortBy)
	assert.Equal(t, core.ListImagesSortOrderDesc, got.SortOrder)
	assert.Equal(t, map[string]string{
		tagPool:  "pool-1",
		tagImage: "ocid1.image.oc1..resolved",
		"team":   "ci",
	}, p.tags)
}

func TestSetupKeepsConfiguredImage(t *testing.T) {
	p := newTestProvider(&fakeComputeAPI{
		listImgFn: func(context.Context, core.ListImagesRequest) (core.ListImagesResponse, error) {
			t.Fatal("ListImages must not be called when an image ID is configured")
			return core.ListImagesResponse{}, nil
		},
	})

	require.NoError(t, p.setup(t.Context(), "Canonical Ubuntu", "24.04", nil))
	assert.Equal(t, "ocid1.image.oc1..ccc", p.imageID)
	assert.Equal(t, "ocid1.image.oc1..ccc", p.tags[tagImage])
}

func TestSetupImageNotFound(t *testing.T) {
	p := newTestProvider(&fakeComputeAPI{})
	p.imageID = ""

	err := p.setup(t.Context(), "Canonical Ubuntu", "99.99", nil)
	require.ErrorIs(t, err, ErrImageNotFound)
}

func TestDeployAgentLaunchesInstance(t *testing.T) {
	var got core.LaunchInstanceDetails
	p := newTestProvider(&fakeComputeAPI{
		launchFn: func(_ context.Context, req core.LaunchInstanceRequest) (core.LaunchInstanceResponse, error) {
			got = req.LaunchInstanceDetails
			return core.LaunchInstanceResponse{}, nil
		},
	})

	require.NoError(t, p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "pool-1-agent-1", Token: "secret"}))

	assert.Equal(t, "pool-1-agent-1", *got.DisplayName)
	assert.Equal(t, "ocid1.compartment.oc1..aaa", *got.CompartmentId)
	assert.Equal(t, "Uocm:EU-FRANKFURT-1-AD-1", *got.AvailabilityDomain)
	assert.Equal(t, "VM.Standard.E4.Flex", *got.Shape)
	assert.Equal(t, float32(2), *got.ShapeConfig.Ocpus)
	assert.Equal(t, float32(16), *got.ShapeConfig.MemoryInGBs)
	assert.Equal(t, "ocid1.subnet.oc1..bbb", *got.CreateVnicDetails.SubnetId)
	assert.True(t, *got.CreateVnicDetails.AssignPublicIp)
	assert.Equal(t, p.tags, got.FreeformTags)
	assert.Equal(t, "ssh-ed25519 AAAA test", got.Metadata["ssh_authorized_keys"])

	source, ok := got.SourceDetails.(core.InstanceSourceViaImageDetails)
	require.True(t, ok)
	assert.Equal(t, "ocid1.image.oc1..ccc", *source.ImageId)

	userData, err := base64.StdEncoding.DecodeString(got.Metadata["user_data"])
	require.NoError(t, err)
	assert.Contains(t, string(userData), "#cloud-config")
	assert.Contains(t, string(userData), "grpc.example.com")
	assert.Contains(t, string(userData), "secret")
	assert.Contains(t, string(userData), "ip -4 route add blackhole 169.254.169.254/32")
}

func TestDeployAgentOmitsShapeConfigForFixedShapes(t *testing.T) {
	var got core.LaunchInstanceDetails
	p := newTestProvider(&fakeComputeAPI{
		launchFn: func(_ context.Context, req core.LaunchInstanceRequest) (core.LaunchInstanceResponse, error) {
			got = req.LaunchInstanceDetails
			return core.LaunchInstanceResponse{}, nil
		},
	})
	p.shape = "VM.Standard2.1"
	p.sshAuthorizedKeys = ""

	require.NoError(t, p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "pool-1-agent-1"}))

	assert.Nil(t, got.ShapeConfig)
	assert.NotContains(t, got.Metadata, "ssh_authorized_keys")
}

func TestDeployAgentLaunchError(t *testing.T) {
	p := newTestProvider(&fakeComputeAPI{
		launchFn: func(context.Context, core.LaunchInstanceRequest) (core.LaunchInstanceResponse, error) {
			return core.LaunchInstanceResponse{}, errors.New("boom")
		},
	})

	err := p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "pool-1-agent-1"})
	require.ErrorContains(t, err, "LaunchInstance: boom")
}

func TestListDeployedAgentNamesFiltersAndPaginates(t *testing.T) {
	instance := func(id, name, pool string, state core.InstanceLifecycleStateEnum) core.Instance {
		return core.Instance{
			Id:             &id,
			DisplayName:    &name,
			LifecycleState: state,
			FreeformTags:   map[string]string{tagPool: pool},
		}
	}

	var requests []core.ListInstancesRequest
	p := newTestProvider(&fakeComputeAPI{
		listFn: func(_ context.Context, req core.ListInstancesRequest) (core.ListInstancesResponse, error) {
			requests = append(requests, req)
			if req.Page == nil {
				return core.ListInstancesResponse{
					Items: []core.Instance{
						instance("1", "pool-1-agent-1", "pool-1", core.InstanceLifecycleStateRunning),
						instance("2", "pool-2-agent-1", "pool-2", core.InstanceLifecycleStateRunning),
						instance("3", "pool-1-agent-2", "pool-1", core.InstanceLifecycleStateTerminated),
					},
					OpcNextPage: utils.ToPtr("page-2"),
				}, nil
			}
			return core.ListInstancesResponse{
				Items: []core.Instance{
					instance("4", "pool-1-agent-3", "pool-1", core.InstanceLifecycleStateProvisioning),
					instance("5", "untagged", "", core.InstanceLifecycleStateRunning),
				},
			}, nil
		},
	})

	names, err := p.ListDeployedAgentNames(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"pool-1-agent-1", "pool-1-agent-3"}, names)

	require.Len(t, requests, 2)
	assert.Equal(t, "ocid1.compartment.oc1..aaa", *requests[0].CompartmentId)
	assert.Equal(t, "Uocm:EU-FRANKFURT-1-AD-1", *requests[0].AvailabilityDomain)
	assert.Nil(t, requests[0].DisplayName)
	assert.Equal(t, "page-2", *requests[1].Page)
}

func TestRemoveAgentTerminatesMatchingInstance(t *testing.T) {
	var terminated []string
	p := newTestProvider(&fakeComputeAPI{
		listFn: func(_ context.Context, req core.ListInstancesRequest) (core.ListInstancesResponse, error) {
			assert.Equal(t, "pool-1-agent-1", *req.DisplayName)
			return core.ListInstancesResponse{Items: []core.Instance{{
				Id:             utils.ToPtr("ocid1.instance.oc1..99"),
				DisplayName:    utils.ToPtr("pool-1-agent-1"),
				LifecycleState: core.InstanceLifecycleStateRunning,
				FreeformTags:   map[string]string{tagPool: "pool-1"},
			}}}, nil
		},
		terminateFn: func(_ context.Context, req core.TerminateInstanceRequest) (core.TerminateInstanceResponse, error) {
			terminated = append(terminated, *req.InstanceId)
			return core.TerminateInstanceResponse{}, nil
		},
	})

	require.NoError(t, p.RemoveAgent(t.Context(), &woodpecker.Agent{Name: "pool-1-agent-1"}))
	assert.Equal(t, []string{"ocid1.instance.oc1..99"}, terminated)
}

func TestRemoveAgentIgnoresUnknownInstance(t *testing.T) {
	p := newTestProvider(&fakeComputeAPI{
		terminateFn: func(context.Context, core.TerminateInstanceRequest) (core.TerminateInstanceResponse, error) {
			t.Fatal("TerminateInstance must not be called")
			return core.TerminateInstanceResponse{}, nil
		},
	})

	require.NoError(t, p.RemoveAgent(t.Context(), &woodpecker.Agent{Name: "missing"}))
}

func TestRemoveAgentRejectsAmbiguousMatches(t *testing.T) {
	p := newTestProvider(&fakeComputeAPI{
		listFn: func(context.Context, core.ListInstancesRequest) (core.ListInstancesResponse, error) {
			mk := func(id string) core.Instance {
				return core.Instance{
					Id:             &id,
					DisplayName:    utils.ToPtr("pool-1-agent-1"),
					LifecycleState: core.InstanceLifecycleStateRunning,
					FreeformTags:   map[string]string{tagPool: "pool-1"},
				}
			}
			return core.ListInstancesResponse{Items: []core.Instance{mk("1"), mk("2")}}, nil
		},
	})

	err := p.RemoveAgent(t.Context(), &woodpecker.Agent{Name: "pool-1-agent-1"})
	require.ErrorIs(t, err, ErrMultipleInstances)
}
