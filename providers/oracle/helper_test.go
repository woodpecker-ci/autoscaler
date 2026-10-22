package oracle

import (
	"context"
	"errors"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/autoscaler/providers/oracle/ociapi/mocks"
)

type stubServiceError struct {
	code    string
	message string
	status  int
}

func (e stubServiceError) Error() string           { return e.message }
func (e stubServiceError) GetHTTPStatusCode() int  { return e.status }
func (e stubServiceError) GetMessage() string      { return e.message }
func (e stubServiceError) GetCode() string         { return e.code }
func (e stubServiceError) GetOpcRequestID() string { return "opc-request-id" }

func testProvider(compute *mocks.MockComputeClient, identityClient *mocks.MockIdentityClient) *provider {
	return &provider{
		name:                "oracle",
		config:              &config.Config{PoolID: "1"},
		computeClient:       compute,
		identityClient:      identityClient,
		compartmentID:       "ocid1.compartment.oc1..compartment",
		subnetID:            "ocid1.subnet.oc1..subnet",
		availabilityDomains: []string{"AD-1", "AD-2"},
		shapes:              []shapeCandidate{testShape()},
		ocpus:               1,
		memoryInGBs:         8,
		bootVolumeSizeInGBs: minBootVolumeGB,
		assignPublicIP:      true,
		tags:                map[string]string{poolTagKey(): "1"},
	}
}

func instance(name, poolID string, state core.InstanceLifecycleStateEnum) core.Instance {
	return core.Instance{
		Id:             common.String("ocid1.instance.oc1.." + name),
		DisplayName:    common.String(name),
		LifecycleState: state,
		FreeformTags:   map[string]string{poolTagKey(): poolID},
	}
}

func TestPoolTagKeyIsAcceptedByOracleCloud(t *testing.T) {
	assert.NotContains(t, poolTagKey(), ".")
	assert.NotContains(t, poolTagKey(), " ")
	assert.NotContains(t, tagKeyPrefix(), ".")
	assert.True(t, len(poolTagKey()) <= 100)
}

func TestParseTags(t *testing.T) {
	t.Run("SplitsKeyValuePairs", func(t *testing.T) {
		sanitized, tags, err := parseTags([]string{"owner=ci", "env=prod"})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"owner": "ci", "env": "prod"}, tags)
		assert.ElementsMatch(t, []string{"owner=ci", "env=prod"}, sanitized)
	})

	t.Run("KeepsEmptyValues", func(t *testing.T) {
		_, tags, err := parseTags([]string{"owner="})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"owner": ""}, tags)
	})

	t.Run("RejectsEntryWithoutSeparator", func(t *testing.T) {
		_, _, err := parseTags([]string{"owner"})
		assert.ErrorIs(t, err, ErrInvalidTag)
	})

	t.Run("SanitizesKeySoReservedPrefixCannotBeSmuggledIn", func(t *testing.T) {
		sanitized, tags, err := parseTags([]string{engine.LabelPool + "=42"})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{poolTagKey(): "42"}, tags)
		assert.Equal(t, []string{poolTagKey() + "=42"}, sanitized)
	})
}

func TestIsCapacityError(t *testing.T) {
	t.Run("OutOfHostCapacityMessage", func(t *testing.T) {
		err := stubServiceError{code: "InternalError", message: "Out of host capacity.", status: 500}
		assert.True(t, isCapacityError(err))
	})

	t.Run("CapacityErrorCode", func(t *testing.T) {
		err := stubServiceError{code: "LimitExceeded", message: "quota reached", status: 400}
		assert.True(t, isCapacityError(err))
	})

	t.Run("WrappedError", func(t *testing.T) {
		err := stubServiceError{code: "TooManyRequests", message: "slow down", status: 429}
		assert.True(t, isCapacityError(errors.Join(errors.New("launch failed"), err)))
	})

	t.Run("UnrelatedServiceError", func(t *testing.T) {
		err := stubServiceError{code: "NotAuthorizedOrNotFound", message: "subnet not found", status: 404}
		assert.False(t, isCapacityError(err))
	})

	t.Run("PlainError", func(t *testing.T) {
		assert.False(t, isCapacityError(errors.New("connection reset")))
	})
}

func TestResolveAvailabilityDomains(t *testing.T) {
	t.Run("ConfiguredListIsUsedVerbatim", func(t *testing.T) {
		identityClient := mocks.NewMockIdentityClient(t)
		p := testProvider(mocks.NewMockComputeClient(t), identityClient)
		p.availabilityDomains = nil

		require.NoError(t, p.resolveAvailabilityDomains(t.Context(), []string{"AD-3", "AD-1"}))
		assert.Equal(t, []string{"AD-3", "AD-1"}, p.availabilityDomains)
	})

	t.Run("DiscoversDomainsOfCompartment", func(t *testing.T) {
		identityClient := mocks.NewMockIdentityClient(t)
		identityClient.On("ListAvailabilityDomains", mock.Anything, mock.Anything).
			Return(identity.ListAvailabilityDomainsResponse{
				Items: []identity.AvailabilityDomain{
					{Name: common.String("Uocm:EU-FRANKFURT-1-AD-1")},
					{Name: common.String("Uocm:EU-FRANKFURT-1-AD-2")},
				},
			}, nil).Once()

		p := testProvider(mocks.NewMockComputeClient(t), identityClient)
		p.availabilityDomains = nil

		require.NoError(t, p.resolveAvailabilityDomains(t.Context(), nil))
		assert.Equal(t, []string{"Uocm:EU-FRANKFURT-1-AD-1", "Uocm:EU-FRANKFURT-1-AD-2"}, p.availabilityDomains)
	})

	t.Run("EmptyCompartmentIsAnError", func(t *testing.T) {
		identityClient := mocks.NewMockIdentityClient(t)
		identityClient.On("ListAvailabilityDomains", mock.Anything, mock.Anything).
			Return(identity.ListAvailabilityDomainsResponse{}, nil).Once()

		p := testProvider(mocks.NewMockComputeClient(t), identityClient)
		p.availabilityDomains = nil

		assert.ErrorIs(t, p.resolveAvailabilityDomains(t.Context(), nil), ErrNoAvailabilityDomains)
	})
}

func TestResolveShapes(t *testing.T) {
	t.Run("KeepsConfigOrderWithFlexibilityAndPlatform", func(t *testing.T) {
		compute := mocks.NewMockComputeClient(t)
		compute.On("ListShapes", mock.Anything, mock.Anything).
			Return(core.ListShapesResponse{Items: []core.Shape{
				{Shape: common.String("VM.Standard.E2.1.Micro"), IsFlexible: common.Bool(false)},
				{Shape: common.String("VM.Standard.A1.Flex"), IsFlexible: common.Bool(true)},
				{Shape: common.String("VM.Standard.E4.Flex"), IsFlexible: common.Bool(true)},
			}}, nil).Once()

		p := testProvider(compute, mocks.NewMockIdentityClient(t))
		require.NoError(t, p.resolveShapes(t.Context(), []string{"VM.Standard.A1.Flex", "VM.Standard.E2.1.Micro"}))
		assert.Equal(t, []shapeCandidate{
			{name: "VM.Standard.A1.Flex", flexible: true, platform: "linux/arm64"},
			{name: "VM.Standard.E2.1.Micro", flexible: false, platform: "linux/amd64"},
		}, p.shapes)
	})

	t.Run("FollowsPaginationAndFailsOnUnknownShape", func(t *testing.T) {
		compute := mocks.NewMockComputeClient(t)
		compute.On("ListShapes", mock.Anything, mock.MatchedBy(func(r core.ListShapesRequest) bool {
			return r.Page == nil
		})).Return(core.ListShapesResponse{
			Items:       []core.Shape{{Shape: common.String("VM.Standard.E2.1.Micro")}},
			OpcNextPage: common.String("page-2"),
		}, nil).Once()
		compute.On("ListShapes", mock.Anything, mock.MatchedBy(func(r core.ListShapesRequest) bool {
			return r.Page != nil && *r.Page == "page-2"
		})).Return(core.ListShapesResponse{
			Items: []core.Shape{{Shape: common.String("VM.Standard.A1.Flex")}},
		}, nil).Once()

		p := testProvider(compute, mocks.NewMockIdentityClient(t))
		assert.ErrorIs(t, p.resolveShapes(t.Context(), []string{"VM.Standard.A1.Flex", "VM.Standard.Nonexistent"}), ErrShapeNotFound)
	})

	t.Run("EmptyListIsAnError", func(t *testing.T) {
		p := testProvider(mocks.NewMockComputeClient(t), mocks.NewMockIdentityClient(t))
		assert.ErrorIs(t, p.resolveShapes(t.Context(), nil), ErrNoShapes)
	})
}

func TestShapeArch(t *testing.T) {
	for shape, arch := range map[string]string{
		"VM.Standard.E4.Flex":    "amd64",
		"VM.Standard.E2.1.Micro": "amd64",
		"VM.Standard3.Flex":      "amd64",
		"VM.Standard.A1.Flex":    "arm64",
		"VM.Standard.A2.Flex":    "arm64",
		"BM.Standard.A1.160":     "arm64",
		"VM.GPU.A10.1":           "amd64",
		"BM.GPU.A10.4":           "amd64",
	} {
		assert.Equal(t, arch, shapeArch(shape), shape)
	}
}

func TestResolveImages(t *testing.T) {
	t.Run("ExplicitImageSkipsLookup", func(t *testing.T) {
		p := testProvider(mocks.NewMockComputeClient(t), mocks.NewMockIdentityClient(t))

		require.NoError(t, p.resolveImages(t.Context(), "ocid1.image.oc1..explicit", "Canonical Ubuntu", "24.04"))
		assert.Equal(t, "ocid1.image.oc1..explicit", p.shapes[0].imageID)
	})

	t.Run("PicksMostRecentImageForEachShape", func(t *testing.T) {
		compute := mocks.NewMockComputeClient(t)
		for shape, image := range map[string]string{
			"VM.Standard.E4.Flex": "ocid1.image.oc1..x86",
			"VM.Standard.A1.Flex": "ocid1.image.oc1..arm",
		} {
			compute.On("ListImages", mock.Anything, mock.MatchedBy(func(r core.ListImagesRequest) bool {
				return *r.OperatingSystem == "Canonical Ubuntu" &&
					*r.OperatingSystemVersion == "24.04" &&
					*r.Shape == shape &&
					r.SortBy == core.ListImagesSortByTimecreated &&
					r.SortOrder == core.ListImagesSortOrderDesc
			})).Return(core.ListImagesResponse{
				Items: []core.Image{{Id: common.String(image)}},
			}, nil).Once()
		}

		p := testProvider(compute, mocks.NewMockIdentityClient(t))
		p.shapes = append(p.shapes, shapeCandidate{name: "VM.Standard.A1.Flex", platform: "linux/arm64"})
		require.NoError(t, p.resolveImages(t.Context(), "", "Canonical Ubuntu", "24.04"))
		assert.Equal(t, "ocid1.image.oc1..x86", p.shapes[0].imageID)
		assert.Equal(t, "ocid1.image.oc1..arm", p.shapes[1].imageID)
	})

	t.Run("NoImageIsAnError", func(t *testing.T) {
		compute := mocks.NewMockComputeClient(t)
		compute.On("ListImages", mock.Anything, mock.Anything).
			Return(core.ListImagesResponse{}, nil).Once()

		p := testProvider(compute, mocks.NewMockIdentityClient(t))
		assert.ErrorIs(t, p.resolveImages(t.Context(), "", "Canonical Ubuntu", "24.04"), ErrImageNotFound)
	})
}

func TestResolveSubnet(t *testing.T) {
	subnet := func(t *testing.T, s core.Subnet) *mocks.MockNetworkClient {
		t.Helper()
		network := mocks.NewMockNetworkClient(t)
		network.On("GetSubnet", mock.Anything, mock.MatchedBy(func(r core.GetSubnetRequest) bool {
			return *r.SubnetId == "ocid1.subnet.oc1..subnet"
		})).Return(core.GetSubnetResponse{Subnet: s}, nil).Once()
		return network
	}

	t.Run("RegionalSubnetKeepsAvailabilityDomains", func(t *testing.T) {
		p := testProvider(mocks.NewMockComputeClient(t), mocks.NewMockIdentityClient(t))
		p.networkClient = subnet(t, core.Subnet{ProhibitPublicIpOnVnic: common.Bool(false)})

		require.NoError(t, p.resolveSubnet(t.Context()))
		assert.Equal(t, []string{"AD-1", "AD-2"}, p.availabilityDomains)
	})

	t.Run("AvailabilityDomainSubnetNarrowsTheList", func(t *testing.T) {
		p := testProvider(mocks.NewMockComputeClient(t), mocks.NewMockIdentityClient(t))
		p.networkClient = subnet(t, core.Subnet{AvailabilityDomain: common.String("AD-2")})

		require.NoError(t, p.resolveSubnet(t.Context()))
		assert.Equal(t, []string{"AD-2"}, p.availabilityDomains)
	})

	t.Run("AvailabilityDomainSubnetOutsideTheListIsAnError", func(t *testing.T) {
		p := testProvider(mocks.NewMockComputeClient(t), mocks.NewMockIdentityClient(t))
		p.networkClient = subnet(t, core.Subnet{AvailabilityDomain: common.String("AD-3")})

		assert.ErrorIs(t, p.resolveSubnet(t.Context()), ErrSubnetAvailabilityDomain)
	})

	t.Run("PrivateSubnetRejectsPublicIP", func(t *testing.T) {
		p := testProvider(mocks.NewMockComputeClient(t), mocks.NewMockIdentityClient(t))
		p.networkClient = subnet(t, core.Subnet{ProhibitPublicIpOnVnic: common.Bool(true)})

		assert.ErrorIs(t, p.resolveSubnet(t.Context()), ErrPublicIPProhibited)

		p.networkClient = subnet(t, core.Subnet{ProhibitPublicIpOnVnic: common.Bool(true)})
		p.assignPublicIP = false
		require.NoError(t, p.resolveSubnet(t.Context()))
	})
}

func newTestCommand(t *testing.T, flags []cli.Flag, args ...string) *cli.Command {
	t.Helper()

	var captured *cli.Command
	cmd := &cli.Command{
		Name:  "autoscaler",
		Flags: flags,
		Action: func(_ context.Context, c *cli.Command) error {
			captured = c
			return nil
		},
	}

	require.NoError(t, cmd.Run(t.Context(), append([]string{"autoscaler"}, args...)))
	require.NotNil(t, captured)
	return captured
}

func TestNewConfigurationProvider(t *testing.T) {
	t.Run("NamesEveryMissingCredential", func(t *testing.T) {
		cmd := newTestCommand(t, ProviderFlags,
			"--oracle-tenancy-ocid", "ocid1.tenancy.oc1..tenancy",
			"--oracle-region", "eu-frankfurt-1",
		)

		_, err := newConfigurationProvider(cmd)
		require.ErrorIs(t, err, ErrIncompleteAPIKeyAuth)
		assert.Contains(t, err.Error(), "WOODPECKER_ORACLE_USER_OCID, WOODPECKER_ORACLE_FINGERPRINT, WOODPECKER_ORACLE_PRIVATE_KEY")
	})

	t.Run("BuildsAPIKeyProvider", func(t *testing.T) {
		cmd := newTestCommand(t, ProviderFlags,
			"--oracle-tenancy-ocid", "ocid1.tenancy.oc1..tenancy",
			"--oracle-user-ocid", "ocid1.user.oc1..user",
			"--oracle-region", "eu-frankfurt-1",
			"--oracle-fingerprint", "aa:bb:cc",
			"--oracle-private-key", "-----BEGIN PRIVATE KEY-----",
		)

		configProvider, err := newConfigurationProvider(cmd)
		require.NoError(t, err)

		tenancy, err := configProvider.TenancyOCID()
		require.NoError(t, err)
		assert.Equal(t, "ocid1.tenancy.oc1..tenancy", tenancy)

		region, err := configProvider.Region()
		require.NoError(t, err)
		assert.Equal(t, "eu-frankfurt-1", region)
	})
}
