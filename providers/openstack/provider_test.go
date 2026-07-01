package openstack

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	th "github.com/gophercloud/gophercloud/v2/testhelper"
	fakeclient "github.com/gophercloud/gophercloud/v2/testhelper/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// newTestCommand builds a parsed *cli.Command exposing the given flag values,
// so provider constructors that read configuration through the CLI can be
// exercised in isolation. Only the supplied flags are defined; any others read
// by the code under test resolve to their zero value.
func newTestCommand(t *testing.T, flags map[string]string) *cli.Command {
	t.Helper()

	defs := make([]cli.Flag, 0, len(flags))
	args := []string{"autoscaler"}
	for name, value := range flags {
		defs = append(defs, &cli.StringFlag{Name: name})
		args = append(args, "--"+name, value)
	}

	var captured *cli.Command
	cmd := &cli.Command{
		Name:  "autoscaler",
		Flags: defs,
		Action: func(_ context.Context, c *cli.Command) error {
			captured = c
			return nil
		},
	}

	require.NoError(t, cmd.Run(t.Context(), args))
	require.NotNil(t, captured)
	return captured
}

// newTestProvider wires a provider up to an in-memory HTTP server that stands
// in for the OpenStack compute API. Handlers are registered on the returned
// mux by each test. The server is torn down automatically when the test ends.
func newTestProvider(t *testing.T) (*provider, *http.ServeMux) {
	t.Helper()

	fakeServer := th.SetupHTTP()
	t.Cleanup(fakeServer.Teardown)

	p := &provider{
		name:          "openstack",
		config:        &config.Config{PoolID: "pool-7"},
		computeClient: fakeclient.ServiceClient(fakeServer),
	}

	return p, fakeServer.Mux
}

// writeJSON is a small helper for handlers that need to return a JSON body.
func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// handleWaitActive registers a GET handler for the server so that
// servers.WaitForStatus observes an ACTIVE server immediately.
func handleWaitActive(mux *http.ServeMux, id string) {
	mux.HandleFunc("GET /servers/"+id, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"server":{"id":"`+id+`","status":"ACTIVE"}}`)
	})
}

func TestBillingModel(t *testing.T) {
	p := &provider{}
	assert.Equal(t, types.BillingPerSecond, p.BillingModel())
}

func TestNewRejectsFlavorRefAndName(t *testing.T) {
	c := newTestCommand(t, map[string]string{
		"openstack-flavor-ref":  "f-1",
		"openstack-flavor-name": "m1.small",
	})

	_, err := New(t.Context(), c, &config.Config{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Flavor")
}

func TestNewRejectsImageRefAndName(t *testing.T) {
	c := newTestCommand(t, map[string]string{
		"openstack-image-ref":  "i-1",
		"openstack-image-name": "ubuntu",
	})

	_, err := New(t.Context(), c, &config.Config{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Image")
}

func TestNewFailsIfVolumeSizeNotInt(t *testing.T) {
	c := newTestCommand(t, map[string]string{
		"openstack-volume-size": "Not a number",
	})

	_, err := New(t.Context(), c, &config.Config{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "integer")
}

func TestDeployAgentInvalidUserData(t *testing.T) {
	p, _ := newTestProvider(t)
	p.config = &config.Config{UserData: "{{.InvalidField}}"}

	err := p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RenderUserDataTemplate")
}

func TestDeployAgentWithRefs(t *testing.T) {
	p, mux := newTestProvider(t)
	p.flavorRef = "flavor-123"
	p.imageRef = "image-456"
	p.network = "net-789"
	p.securityGroups = []string{"default", "web"}
	p.metadata = map[string]string{labelPool: "pool-7", "team": "ci"}

	var got map[string]any
	mux.HandleFunc("POST /servers", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Server map[string]any `json:"server"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		got = body.Server
		writeJSON(w, http.StatusAccepted, `{"server":{"id":"srv-1","status":"BUILD"}}`)
	})
	handleWaitActive(mux, "srv-1")

	err := p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	require.NoError(t, err)

	require.NotNil(t, got)
	assert.Equal(t, "agent-1", got["name"])
	assert.Equal(t, "flavor-123", got["flavorRef"])
	assert.Equal(t, "image-456", got["imageRef"])
	assert.NotEmpty(t, got["user_data"], "rendered user data should be sent")

	metadata, ok := got["metadata"].(map[string]any)
	require.True(t, ok, "metadata should be present")
	assert.Equal(t, "pool-7", metadata[labelPool])
	assert.Equal(t, "ci", metadata["team"])

	networks, ok := got["networks"].([]any)
	require.True(t, ok, "networks should be present")
	require.Len(t, networks, 1)
	assert.Equal(t, "net-789", networks[0].(map[string]any)["uuid"])

	sgs, ok := got["security_groups"].([]any)
	require.True(t, ok, "security groups should be present")
	require.Len(t, sgs, 2)
	assert.Equal(t, "default", sgs[0].(map[string]any)["name"])
	assert.Equal(t, "web", sgs[1].(map[string]any)["name"])

	_, hasKey := got["key_name"]
	assert.False(t, hasKey, "no keypair configured, key_name must be absent")
}

func TestDeployAgentWithKeypair(t *testing.T) {
	p, mux := newTestProvider(t)
	p.flavorRef = "flavor-123"
	p.imageRef = "image-456"
	p.keypair = "my-key"

	var got map[string]any
	mux.HandleFunc("POST /servers", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Server map[string]any `json:"server"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		got = body.Server
		writeJSON(w, http.StatusAccepted, `{"server":{"id":"srv-1","status":"ACTIVE"}}`)
	})
	handleWaitActive(mux, "srv-1")

	err := p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	require.NoError(t, err)
	assert.Equal(t, "my-key", got["key_name"])
}

func TestDeployAgentOmitsNetworkWhenUnset(t *testing.T) {
	p, mux := newTestProvider(t)
	p.flavorRef = "flavor-123"
	p.imageRef = "image-456"

	var got map[string]any
	mux.HandleFunc("POST /servers", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Server map[string]any `json:"server"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		got = body.Server
		writeJSON(w, http.StatusAccepted, `{"server":{"id":"srv-1","status":"ACTIVE"}}`)
	})
	handleWaitActive(mux, "srv-1")

	err := p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	require.NoError(t, err)
	_, hasNetworks := got["networks"]
	assert.False(t, hasNetworks, "networks must be omitted when no network is configured")
}

func TestDeployAgentResolvesFlavorName(t *testing.T) {
	p, mux := newTestProvider(t)
	p.flavorName = "m1.small"
	p.imageRef = "image-456"

	mux.HandleFunc("GET /flavors/detail", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"flavors":[
			{"id":"flavor-aaa","name":"m1.tiny"},
			{"id":"flavor-bbb","name":"m1.small"}
		]}`)
	})

	var got map[string]any
	mux.HandleFunc("POST /servers", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Server map[string]any `json:"server"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		got = body.Server
		writeJSON(w, http.StatusAccepted, `{"server":{"id":"srv-1","status":"ACTIVE"}}`)
	})
	handleWaitActive(mux, "srv-1")

	err := p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	require.NoError(t, err)
	assert.Equal(t, "flavor-bbb", got["flavorRef"], "should resolve the ID for m1.small")
	assert.Equal(t, "flavor-bbb", p.flavorRef, "resolved flavor ref should be cached on the provider")
}

func TestDeployAgentFlavorNameNotFound(t *testing.T) {
	p, mux := newTestProvider(t)
	p.flavorName = "does-not-exist"
	p.imageRef = "image-456"

	mux.HandleFunc("GET /flavors/detail", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"flavors":[{"id":"flavor-aaa","name":"m1.tiny"}]}`)
	})

	err := p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "No flavor ID found")
}

func TestDeployAgentResolvesImageName(t *testing.T) {
	p, mux := newTestProvider(t)
	p.flavorRef = "flavor-123"
	p.imageName = "ubuntu-24.04"

	mux.HandleFunc("GET /images", func(w http.ResponseWriter, _ *http.Request) {
		// Newest first; the provider takes the first entry. No "next" field
		// means this is the only page.
		writeJSON(w, http.StatusOK, `{"images":[
			{"id":"image-newest","name":"ubuntu-24.04"},
			{"id":"image-older","name":"ubuntu-24.04"}
		]}`)
	})

	var got map[string]any
	mux.HandleFunc("POST /servers", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Server map[string]any `json:"server"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		got = body.Server
		writeJSON(w, http.StatusAccepted, `{"server":{"id":"srv-1","status":"ACTIVE"}}`)
	})
	handleWaitActive(mux, "srv-1")

	err := p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	require.NoError(t, err)
	assert.Equal(t, "image-newest", got["imageRef"])
	assert.Equal(t, "image-newest", p.imageRef)
}

func TestDeployAgentImageNameNotFound(t *testing.T) {
	p, mux := newTestProvider(t)
	p.flavorRef = "flavor-123"
	p.imageName = "ghost-image"

	mux.HandleFunc("GET /images", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"images":[]}`)
	})

	err := p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "No image ID found")
}

func TestDeployAgentOnEphemeralStorage(t *testing.T) {
	p, mux := newTestProvider(t)
	p.flavorRef = "flavor-123"
	p.imageRef = "image-123"

	var got map[string]any
	mux.HandleFunc("POST /servers", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Server map[string]any `json:"server"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		got = body.Server
		writeJSON(w, http.StatusAccepted, `{"server":{"id":"srv-1","status":"ACTIVE"}}`)
	})
	handleWaitActive(mux, "srv-1")

	err := p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	require.NoError(t, err)
	assert.Equal(t, "image-123", got["imageRef"])

	_, hasBdm := got["block_device_mapping_v2"]
	assert.False(t, hasBdm)
}

func TestDeployAgentOnVolume(t *testing.T) {
	p, mux := newTestProvider(t)
	p.flavorRef = "flavor-123"
	p.imageRef = "image-123"
	p.volumeSize = 20

	var got map[string]any
	mux.HandleFunc("POST /servers", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Server map[string]any `json:"server"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		got = body.Server
		writeJSON(w, http.StatusAccepted, `{"server":{"id":"srv-1","status":"ACTIVE"}}`)
	})
	handleWaitActive(mux, "srv-1")

	err := p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	require.NoError(t, err)
	bdm := got["block_device_mapping_v2"].([]any)[0].(map[string]any)
	assert.Equal(t, true, bdm["delete_on_termination"])
	assert.Equal(t, "volume", bdm["destination_type"])
	assert.Equal(t, "image-123", bdm["uuid"])
	assert.Equal(t, 20.0, bdm["volume_size"])
	assert.Equal(t, "", got["imageRef"])
}

func TestDeployAgentCreateError(t *testing.T) {
	p, mux := newTestProvider(t)
	p.flavorRef = "flavor-123"
	p.imageRef = "image-456"

	mux.HandleFunc("POST /servers", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusInternalServerError, `{"error":"boom"}`)
	})

	err := p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "servers.Create")
}

func TestListDeployedAgentNames(t *testing.T) {
	p, mux := newTestProvider(t)

	mux.HandleFunc("GET /servers/detail", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"servers":[
			{"id":"srv-1","name":"agent-1","status":"ACTIVE","metadata":{"wp.autoscaler-pool":"pool-7"}},
			{"id":"srv-2","name":"agent-2","status":"ACTIVE","metadata":{"wp.autoscaler-pool":"pool-7"}},
			{"id":"srv-3","name":"other-pool","status":"ACTIVE","metadata":{"wp.autoscaler-pool":"pool-99"}},
			{"id":"srv-4","name":"unmanaged","status":"ACTIVE","metadata":{}}
		]}`)
	})

	names, err := p.ListDeployedAgentNames(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"agent-1", "agent-2"}, names, "only servers tagged with our pool are returned")
}

func TestListDeployedAgentNamesEmpty(t *testing.T) {
	p, mux := newTestProvider(t)

	mux.HandleFunc("GET /servers/detail", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"servers":[]}`)
	})

	names, err := p.ListDeployedAgentNames(t.Context())
	require.NoError(t, err)
	assert.Empty(t, names)
}

func TestListDeployedAgentNamesError(t *testing.T) {
	p, mux := newTestProvider(t)

	mux.HandleFunc("GET /servers/detail", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusInternalServerError, `{"error":"boom"}`)
	})

	names, err := p.ListDeployedAgentNames(t.Context())
	require.Error(t, err)
	assert.Nil(t, names)
	assert.Contains(t, err.Error(), "servers.List")
}

func TestRemoveAgent(t *testing.T) {
	p, mux := newTestProvider(t)

	mux.HandleFunc("GET /servers/detail", func(w http.ResponseWriter, r *http.Request) {
		// The provider filters server-side by an anchored name; echo the
		// requested server back so the client-side match succeeds too.
		assert.Equal(t, "^agent-1$", r.URL.Query().Get("name"))
		writeJSON(w, http.StatusOK, `{"servers":[
			{"id":"srv-1","name":"agent-1","status":"ACTIVE","metadata":{}}
		]}`)
	})

	deleted := false
	mux.HandleFunc("DELETE /servers/srv-1", func(w http.ResponseWriter, _ *http.Request) {
		deleted = true
		w.WriteHeader(http.StatusNoContent)
	})

	err := p.RemoveAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	require.NoError(t, err)
	assert.True(t, deleted, "the matching server should be deleted")
}

func TestRemoveAgentNotFound(t *testing.T) {
	p, mux := newTestProvider(t)

	mux.HandleFunc("GET /servers/detail", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"servers":[]}`)
	})
	// No DELETE handler registered: if the provider attempts a delete the
	// request 404s and the test fails.

	err := p.RemoveAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	require.NoError(t, err, "removing a non-existent agent is a no-op")
}

func TestRemoveAgentListError(t *testing.T) {
	p, mux := newTestProvider(t)

	mux.HandleFunc("GET /servers/detail", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusInternalServerError, `{"error":"boom"}`)
	})

	err := p.RemoveAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "findServerIDByName")
}

func TestFindServerIDByNameNoClientSideMatch(t *testing.T) {
	// The API filter is a best-effort regex; the provider still compares names
	// exactly on the client side. A server whose name only partially matches
	// must not be returned.
	p, mux := newTestProvider(t)

	mux.HandleFunc("GET /servers/detail", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"servers":[
			{"id":"srv-1","name":"agent-1-replica","status":"ACTIVE","metadata":{}}
		]}`)
	})

	id, err := p.findServerIDByName(t.Context(), "agent-1")
	require.NoError(t, err)
	assert.Empty(t, id, "partial name matches must be rejected")
}
