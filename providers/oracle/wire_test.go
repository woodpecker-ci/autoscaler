package oracle

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	b64 "encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/autoscaler/config"
)

type recordedRequest struct {
	method        string
	path          string
	query         map[string][]string
	authorization string
	body          map[string]any
}

func testSigningKey(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}))
}

func newWireTestProvider(t *testing.T, respond func(*recordedRequest) (int, string)) (*provider, *[]recordedRequest) {
	t.Helper()

	var recorded []recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		request := recordedRequest{
			method:        r.Method,
			path:          r.URL.Path,
			query:         r.URL.Query(),
			authorization: r.Header.Get("Authorization"),
		}
		if len(raw) > 0 {
			require.NoError(t, json.Unmarshal(raw, &request.body))
		}
		recorded = append(recorded, request)

		status, body := respond(&request)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, err = io.WriteString(w, body)
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	configProvider := common.NewRawConfigurationProvider(
		"ocid1.tenancy.oc1..tenancy",
		"ocid1.user.oc1..user",
		"us-phoenix-1",
		"aa:bb:cc:dd",
		testSigningKey(t),
		nil,
	)

	computeClient, err := core.NewComputeClientWithConfigurationProvider(configProvider)
	require.NoError(t, err)
	computeClient.Host = server.URL

	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(configProvider)
	require.NoError(t, err)
	identityClient.Host = server.URL

	p := &provider{
		name:                "oracle",
		config:              &config.Config{PoolID: "1"},
		computeClient:       computeClient,
		identityClient:      identityClient,
		compartmentID:       "ocid1.compartment.oc1..compartment",
		subnetID:            "ocid1.subnet.oc1..subnet",
		availabilityDomains: []string{"Uocm:PHX-AD-1"},
		shape:               "VM.Standard.E4.Flex",
		shapeIsFlexible:     true,
		ocpus:               2,
		memoryInGBs:         16,
		imageID:             "ocid1.image.oc1..image",
		bootVolumeSizeInGBs: minBootVolumeGB,
		assignPublicIP:      true,
		tags:                map[string]string{poolTagKey(): "1"},
	}

	return p, &recorded
}

func TestWireLaunchInstanceRequest(t *testing.T) {
	p, recorded := newWireTestProvider(t, func(*recordedRequest) (int, string) {
		return http.StatusOK, `{"id":"ocid1.instance.oc1..new"}`
	})

	require.NoError(t, p.DeployAgent(t.Context(), testAgent))
	require.Len(t, *recorded, 1)

	request := (*recorded)[0]
	assert.Equal(t, http.MethodPost, request.method)
	assert.Equal(t, "/20160918/instances", request.path)

	assert.Equal(t, "Uocm:PHX-AD-1", request.body["availabilityDomain"])
	assert.Equal(t, "ocid1.compartment.oc1..compartment", request.body["compartmentId"])
	assert.Equal(t, testAgent.Name, request.body["displayName"])
	assert.Equal(t, "VM.Standard.E4.Flex", request.body["shape"])

	freeformTags, ok := request.body["freeformTags"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "1", freeformTags["wp-autoscaler/pool"])

	shapeConfig, ok := request.body["shapeConfig"].(map[string]any)
	require.True(t, ok)
	assert.InDelta(t, 2, shapeConfig["ocpus"], 0)
	assert.InDelta(t, 16, shapeConfig["memoryInGBs"], 0)

	vnic, ok := request.body["createVnicDetails"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "ocid1.subnet.oc1..subnet", vnic["subnetId"])
	assert.Equal(t, true, vnic["assignPublicIp"])

	source, ok := request.body["sourceDetails"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "image", source["sourceType"])
	assert.Equal(t, "ocid1.image.oc1..image", source["imageId"])
	assert.InDelta(t, minBootVolumeGB, source["bootVolumeSizeInGBs"], 0)

	metadata, ok := request.body["metadata"].(map[string]any)
	require.True(t, ok)

	userData, ok := metadata[metadataUserKey].(string)
	require.True(t, ok)
	decoded, err := b64.StdEncoding.DecodeString(userData)
	require.NoError(t, err)
	assert.Contains(t, string(decoded), "ip -4 route add blackhole 169.254.169.254/32")

	assert.Contains(t, request.authorization, "Signature")
	assert.Contains(t, request.authorization, "keyId=")
}

func TestWireFixedShapeOmitsShapeConfig(t *testing.T) {
	p, recorded := newWireTestProvider(t, func(*recordedRequest) (int, string) {
		return http.StatusOK, `{"id":"ocid1.instance.oc1..new"}`
	})
	p.shape = "VM.Standard.E2.1.Micro"
	p.shapeIsFlexible = false

	require.NoError(t, p.DeployAgent(t.Context(), testAgent))
	require.Len(t, *recorded, 1)

	assert.NotContains(t, (*recorded)[0].body, "shapeConfig")
}

func TestWireListAndTerminateRequests(t *testing.T) {
	instanceJSON := `{
		"id": "ocid1.instance.oc1..agent",
		"displayName": "` + testAgent.Name + `",
		"availabilityDomain": "Uocm:PHX-AD-1",
		"compartmentId": "ocid1.compartment.oc1..compartment",
		"lifecycleState": "RUNNING",
		"region": "phx",
		"shape": "VM.Standard.E4.Flex",
		"timeCreated": "2026-09-07T00:00:00.000Z",
		"freeformTags": {"wp-autoscaler/pool": "1"}
	}`

	p, recorded := newWireTestProvider(t, func(r *recordedRequest) (int, string) {
		if r.method == http.MethodGet {
			return http.StatusOK, "[" + instanceJSON + "]"
		}
		return http.StatusNoContent, ""
	})

	names, err := p.ListDeployedAgentNames(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{testAgent.Name}, names)

	require.NoError(t, p.RemoveAgent(t.Context(), testAgent))
	require.Len(t, *recorded, 3)

	list := (*recorded)[0]
	assert.Equal(t, http.MethodGet, list.method)
	assert.Equal(t, "/20160918/instances", list.path)
	assert.Equal(t, []string{"ocid1.compartment.oc1..compartment"}, list.query["compartmentId"])
	assert.Equal(t, []string{"100"}, list.query["limit"])

	lookup := (*recorded)[1]
	assert.Equal(t, []string{testAgent.Name}, lookup.query["displayName"])

	terminate := (*recorded)[2]
	assert.Equal(t, http.MethodDelete, terminate.method)
	assert.Equal(t, "/20160918/instances/ocid1.instance.oc1..agent", terminate.path)
	assert.Equal(t, []string{"false"}, terminate.query["preserveBootVolume"])
}

func TestWireResolveShapeAndImageRequests(t *testing.T) {
	p, recorded := newWireTestProvider(t, func(r *recordedRequest) (int, string) {
		if r.path == "/20160918/shapes" {
			return http.StatusOK, `[{"shape":"VM.Standard.E4.Flex","isFlexible":true}]`
		}
		return http.StatusOK, `[{
			"id": "ocid1.image.oc1..resolved",
			"displayName": "Canonical-Ubuntu-24.04-2026.09.01-0",
			"compartmentId": "ocid1.compartment.oc1..compartment",
			"createImageAllowed": true,
			"lifecycleState": "AVAILABLE",
			"operatingSystem": "Canonical Ubuntu",
			"operatingSystemVersion": "24.04",
			"timeCreated": "2026-09-01T00:00:00.000Z"
		}]`
	})
	p.shapeIsFlexible = false

	require.NoError(t, p.resolveShape(t.Context()))
	assert.True(t, p.shapeIsFlexible)

	require.NoError(t, p.resolveImage(t.Context(), "", "Canonical Ubuntu", "24.04"))
	assert.Equal(t, "ocid1.image.oc1..resolved", p.imageID)

	require.Len(t, *recorded, 2)
	assert.Equal(t, []string{"ocid1.compartment.oc1..compartment"}, (*recorded)[0].query["compartmentId"])

	images := (*recorded)[1]
	assert.Equal(t, "/20160918/images", images.path)
	assert.Equal(t, []string{"Canonical Ubuntu"}, images.query["operatingSystem"])
	assert.Equal(t, []string{"24.04"}, images.query["operatingSystemVersion"])
	assert.Equal(t, []string{"VM.Standard.E4.Flex"}, images.query["shape"])
	assert.Equal(t, []string{"TIMECREATED"}, images.query["sortBy"])
	assert.Equal(t, []string{"DESC"}, images.query["sortOrder"])
}

func TestWireAvailabilityDomainFallbackOnRealServiceError(t *testing.T) {
	p, recorded := newWireTestProvider(t, func(r *recordedRequest) (int, string) {
		if r.body["availabilityDomain"] == "Uocm:PHX-AD-1" {
			return http.StatusInternalServerError, `{"code":"InternalError","message":"Out of host capacity."}`
		}
		return http.StatusOK, `{"id":"ocid1.instance.oc1..new"}`
	})
	p.availabilityDomains = []string{"Uocm:PHX-AD-1", "Uocm:PHX-AD-2"}

	require.NoError(t, p.DeployAgent(t.Context(), testAgent))
	require.Len(t, *recorded, 2)
	assert.Equal(t, "Uocm:PHX-AD-2", (*recorded)[1].body["availabilityDomain"])
}
