package digitalocean

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

func TestNewResolvesConfigAndDefaultSSHKey(t *testing.T) {
	api := newTestAPIServer(t, testAPIHandler{
		regions: []region{{Slug: "nyc1", Available: true}},
		sizes:   []size{{Slug: "s-1vcpu-1gb", Regions: []string{"nyc1"}, Available: true}},
		images:  []image{{ID: 101, Slug: "ubuntu-24-04-x64", Name: "24.04 x64", Distribution: "Ubuntu"}},
		sshKeys: []sshKey{
			{Name: "something-else", Fingerprint: "ff:00"},
			{Name: "woodpecker", Fingerprint: "aa:bb"},
		},
	})

	cmd := newTestCommand(t, ProviderFlags, []string{
		"--digitalocean-api-token=token",
		"--digitalocean-region=nyc1",
		"--digitalocean-size=s-1vcpu-1gb",
		"--digitalocean-image=ubuntu-24-04-x64",
	})

	p, err := newWithClient(t.Context(), cmd, &config.Config{PoolID: "pool-1"}, newClient(api.Client(), api.URL, "token"))
	require.NoError(t, err)

	doProvider := p.(*provider)
	assert.Equal(t, "nyc1", doProvider.region.Slug)
	assert.Equal(t, "s-1vcpu-1gb", doProvider.size.Slug)
	assert.Equal(t, "ubuntu-24-04-x64", doProvider.image.Slug)
	assert.Equal(t, []string{"aa:bb"}, doProvider.sshKeys)
	assert.Contains(t, doProvider.tags, "wp-autoscaler-pool-pool-1")
	assert.Contains(t, doProvider.tags, "wp-autoscaler-image-ubuntu-24-04-x64")
}

func TestNewResolvesConfiguredSSHKeys(t *testing.T) {
	api := newTestAPIServer(t, testAPIHandler{
		regions: []region{{Slug: "nyc1", Available: true}},
		sizes:   []size{{Slug: "s-1vcpu-1gb", Regions: []string{"nyc1"}, Available: true}},
		images:  []image{{ID: 101, Slug: "ubuntu-24-04-x64", Name: "24.04 x64", Distribution: "Ubuntu"}},
		sshKeys: []sshKey{
			{Name: "build", Fingerprint: "11:22"},
			{Name: "deploy", Fingerprint: "33:44"},
		},
	})

	cmd := newTestCommand(t, ProviderFlags, []string{
		"--digitalocean-api-token=token",
		"--digitalocean-ssh-keys=deploy",
		"--digitalocean-ssh-keys=11:22",
	})

	p, err := newWithClient(t.Context(), cmd, &config.Config{PoolID: "pool-1"}, newClient(api.Client(), api.URL, "token"))
	require.NoError(t, err)

	assert.Equal(t, []string{"33:44", "11:22"}, p.(*provider).sshKeys)
}

func TestDeployAgentCreatesDroplet(t *testing.T) {
	var created dropletCreateRequest
	api := newTestAPIServer(t, testAPIHandler{
		regions: []region{{Slug: "nyc1", Available: true}},
		sizes:   []size{{Slug: "s-1vcpu-1gb", Regions: []string{"nyc1"}, Available: true}},
		images:  []image{{ID: 101, Slug: "ubuntu-24-04-x64", Name: "24.04 x64", Distribution: "Ubuntu"}},
		sshKeys: []sshKey{{Name: "woodpecker", Fingerprint: "aa:bb"}},
		onCreateDroplet: func(t *testing.T, req dropletCreateRequest) {
			created = req
		},
	})

	cmd := newTestCommand(t, ProviderFlags, []string{
		"--digitalocean-api-token=token",
		"--digitalocean-tags=team-ci",
	})

	p, err := newWithClient(t.Context(), cmd, &config.Config{
		PoolID:      "pool-1",
		GRPCAddress: "grpc.example.com",
		Image:       "woodpeckerci/woodpecker-agent:next",
	}, newClient(api.Client(), api.URL, "token"))
	require.NoError(t, err)

	err = p.DeployAgent(t.Context(), &woodpecker.Agent{
		Name:  "pool-1-agent-1",
		Token: "secret",
	})
	require.NoError(t, err)

	assert.Equal(t, "pool-1-agent-1", created.Name)
	assert.Equal(t, "nyc1", created.Region)
	assert.Equal(t, "s-1vcpu-1gb", created.Size)
	assert.Equal(t, "ubuntu-24-04-x64", created.Image.Slug)
	assert.Equal(t, []string{"aa:bb"}, created.SSHKeys)
	assert.True(t, created.IPv6)
	assert.Contains(t, created.Tags, "team-ci")
	assert.Contains(t, created.Tags, "wp-autoscaler-pool-pool-1")
	assert.NotEmpty(t, created.UserData)
}

func TestListDeployedAgentNames(t *testing.T) {
	api := newTestAPIServer(t, testAPIHandler{
		dropletsByTag: map[string][]droplet{
			"wp-autoscaler-pool-pool-1": {
				{ID: 1, Name: "pool-1-agent-1", Status: "new"},
				{ID: 2, Name: "pool-1-agent-2", Status: "active"},
				{ID: 3, Name: "pool-1-agent-3", Status: "off"},
			},
		},
	})

	p := &provider{
		name:   "digitalocean",
		config: &config.Config{PoolID: "pool-1"},
		client: newClient(api.Client(), api.URL, "token"),
	}

	names, err := p.ListDeployedAgentNames(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"pool-1-agent-1", "pool-1-agent-2"}, names)
}

func TestRemoveAgentDeletesMatchingDroplet(t *testing.T) {
	var deleted []int
	api := newTestAPIServer(t, testAPIHandler{
		dropletsByTag: map[string][]droplet{
			"wp-autoscaler-pool-pool-1": {
				{ID: 99, Name: "pool-1-agent-1", Status: "active"},
			},
		},
		onDeleteDroplet: func(_ *testing.T, id int) {
			deleted = append(deleted, id)
		},
	})

	p := &provider{
		name:   "digitalocean",
		config: &config.Config{PoolID: "pool-1"},
		client: newClient(api.Client(), api.URL, "token"),
	}

	err := p.RemoveAgent(t.Context(), &woodpecker.Agent{Name: "pool-1-agent-1"})
	require.NoError(t, err)
	assert.Equal(t, []int{99}, deleted)
}

func TestSanitizeTagPart(t *testing.T) {
	assert.Equal(t, "pool-1", sanitizeTagPart("Pool/1"))
	assert.Equal(t, "default", sanitizeTagPart("///"))
}

func newWithClient(ctx context.Context, c *cli.Command, config *config.Config, client *client) (types.Provider, error) {
	return newProviderWithClient(ctx, c, config, client)
}

func newTestCommand(t *testing.T, flags []cli.Flag, args []string) *cli.Command {
	t.Helper()

	var captured *cli.Command
	cmd := &cli.Command{
		Flags: flags,
		Action: func(_ context.Context, c *cli.Command) error {
			captured = c
			return nil
		},
	}

	err := cmd.Run(t.Context(), append([]string{"test"}, args...))
	require.NoError(t, err)
	require.NotNil(t, captured)

	return captured
}

type testAPIHandler struct {
	regions         []region
	sizes           []size
	images          []image
	sshKeys         []sshKey
	dropletsByTag   map[string][]droplet
	onCreateDroplet func(*testing.T, dropletCreateRequest)
	onDeleteDroplet func(*testing.T, int)
}

func newTestAPIServer(t *testing.T, handler testAPIHandler) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/regions":
			_ = json.NewEncoder(w).Encode(map[string]any{"regions": handler.regions})
		case r.Method == http.MethodGet && r.URL.Path == "/sizes":
			_ = json.NewEncoder(w).Encode(map[string]any{"sizes": handler.sizes})
		case r.Method == http.MethodGet && r.URL.Path == "/images":
			_ = json.NewEncoder(w).Encode(map[string]any{"images": handler.images})
		case r.Method == http.MethodGet && r.URL.Path == "/account/keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"ssh_keys": handler.sshKeys})
		case r.Method == http.MethodGet && r.URL.Path == "/droplets":
			tag := r.URL.Query().Get("tag_name")
			_ = json.NewEncoder(w).Encode(map[string]any{"droplets": handler.dropletsByTag[tag]})
		case r.Method == http.MethodPost && r.URL.Path == "/droplets":
			var req dropletCreateRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			if handler.onCreateDroplet != nil {
				handler.onCreateDroplet(t, req)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"droplet": droplet{ID: 1, Name: req.Name, Status: "new"}})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/droplets/"):
			id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/droplets/"))
			require.NoError(t, err)
			if handler.onDeleteDroplet != nil {
				handler.onDeleteDroplet(t, id)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))

	t.Cleanup(server.Close)
	return server
}
