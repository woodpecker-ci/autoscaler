package digitalocean

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/digitalocean/godo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// dropletCreateBody mirrors the JSON DigitalOcean receives for a droplet create request.
type dropletCreateBody struct {
	Name     string   `json:"name"`
	Region   string   `json:"region"`
	Size     string   `json:"size"`
	Image    string   `json:"image"`
	SSHKeys  []string `json:"ssh_keys"`
	IPv6     bool     `json:"ipv6"`
	Tags     []string `json:"tags"`
	UserData string   `json:"user_data"`
}

func TestNewResolvesConfigAndDefaultSSHKey(t *testing.T) {
	api := newTestAPIServer(t, testAPIHandler{
		regions: []godo.Region{{Slug: "nyc1", Available: true}},
		sizes:   []godo.Size{{Slug: "s-1vcpu-1gb", Regions: []string{"nyc1"}, Available: true}},
		images:  []godo.Image{{ID: 101, Slug: "ubuntu-24-04-x64", Name: "24.04 x64", Distribution: "Ubuntu"}},
		sshKeys: []godo.Key{
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

	p, err := newWithClient(t.Context(), cmd, &config.Config{PoolID: "pool-1"}, newTestClient(t, api))
	require.NoError(t, err)

	doProvider, ok := p.(*provider)
	require.True(t, ok)
	assert.Equal(t, "nyc1", doProvider.region.Slug)
	assert.Equal(t, "s-1vcpu-1gb", doProvider.size.Slug)
	assert.Equal(t, "ubuntu-24-04-x64", doProvider.image.Slug)
	assert.Equal(t, []godo.DropletCreateSSHKey{{Fingerprint: "aa:bb"}}, doProvider.sshKeys)
	assert.Contains(t, doProvider.tags, "wp-autoscaler-pool-pool-1")
	assert.Contains(t, doProvider.tags, "wp-autoscaler-image-ubuntu-24-04-x64")
}

func TestNewResolvesConfiguredSSHKeys(t *testing.T) {
	api := newTestAPIServer(t, testAPIHandler{
		regions: []godo.Region{{Slug: "nyc1", Available: true}},
		sizes:   []godo.Size{{Slug: "s-1vcpu-1gb", Regions: []string{"nyc1"}, Available: true}},
		images:  []godo.Image{{ID: 101, Slug: "ubuntu-24-04-x64", Name: "24.04 x64", Distribution: "Ubuntu"}},
		sshKeys: []godo.Key{
			{Name: "build", Fingerprint: "11:22"},
			{Name: "deploy", Fingerprint: "33:44"},
		},
	})

	cmd := newTestCommand(t, ProviderFlags, []string{
		"--digitalocean-api-token=token",
		"--digitalocean-ssh-keys=deploy",
		"--digitalocean-ssh-keys=11:22",
	})

	p, err := newWithClient(t.Context(), cmd, &config.Config{PoolID: "pool-1"}, newTestClient(t, api))
	require.NoError(t, err)

	doProvider, ok := p.(*provider)
	require.True(t, ok)
	assert.Equal(t, []godo.DropletCreateSSHKey{{Fingerprint: "33:44"}, {Fingerprint: "11:22"}}, doProvider.sshKeys)
}

func TestDeployAgentCreatesDroplet(t *testing.T) {
	var created dropletCreateBody
	api := newTestAPIServer(t, testAPIHandler{
		regions: []godo.Region{{Slug: "nyc1", Available: true}},
		sizes:   []godo.Size{{Slug: "s-1vcpu-1gb", Regions: []string{"nyc1"}, Available: true}},
		images:  []godo.Image{{ID: 101, Slug: "ubuntu-24-04-x64", Name: "24.04 x64", Distribution: "Ubuntu"}},
		sshKeys: []godo.Key{{Name: "woodpecker", Fingerprint: "aa:bb"}},
		onCreateDroplet: func(_ *testing.T, req dropletCreateBody) {
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
	}, newTestClient(t, api))
	require.NoError(t, err)

	err = p.DeployAgent(t.Context(), &woodpecker.Agent{
		Name:  "pool-1-agent-1",
		Token: "secret",
	})
	require.NoError(t, err)

	assert.Equal(t, "pool-1-agent-1", created.Name)
	assert.Equal(t, "nyc1", created.Region)
	assert.Equal(t, "s-1vcpu-1gb", created.Size)
	assert.Equal(t, "ubuntu-24-04-x64", created.Image)
	assert.Equal(t, []string{"aa:bb"}, created.SSHKeys)
	assert.True(t, created.IPv6)
	assert.Contains(t, created.Tags, "team-ci")
	assert.Contains(t, created.Tags, "wp-autoscaler-pool-pool-1")
	assert.NotEmpty(t, created.UserData)
}

func TestListDeployedAgentNames(t *testing.T) {
	api := newTestAPIServer(t, testAPIHandler{
		dropletsByTag: map[string][]godo.Droplet{
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
		client: newTestClient(t, api),
	}

	names, err := p.ListDeployedAgentNames(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"pool-1-agent-1", "pool-1-agent-2"}, names)
}

func TestRemoveAgentDeletesMatchingDroplet(t *testing.T) {
	var deleted []int
	api := newTestAPIServer(t, testAPIHandler{
		dropletsByTag: map[string][]godo.Droplet{
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
		client: newTestClient(t, api),
	}

	err := p.RemoveAgent(t.Context(), &woodpecker.Agent{Name: "pool-1-agent-1"})
	require.NoError(t, err)
	assert.Equal(t, []int{99}, deleted)
}

func TestSanitizeTagPart(t *testing.T) {
	assert.Equal(t, "pool-1", sanitizeTagPart("Pool/1"))
	assert.Equal(t, "default", sanitizeTagPart("///"))
}

func newWithClient(ctx context.Context, c *cli.Command, config *config.Config, client *godo.Client) (types.Provider, error) {
	return newProviderWithClient(ctx, c, config, client)
}

func newTestClient(t *testing.T, api *httptest.Server) *godo.Client {
	t.Helper()

	client, err := godo.New(api.Client(), godo.SetBaseURL(api.URL+"/"))
	require.NoError(t, err)

	return client
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
	regions         []godo.Region
	sizes           []godo.Size
	images          []godo.Image
	sshKeys         []godo.Key
	dropletsByTag   map[string][]godo.Droplet
	onCreateDroplet func(*testing.T, dropletCreateBody)
	onDeleteDroplet func(*testing.T, int)
}

func newTestAPIServer(t *testing.T, handler testAPIHandler) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/regions":
			_ = json.NewEncoder(w).Encode(map[string]any{"regions": handler.regions})
		case r.Method == http.MethodGet && r.URL.Path == "/v2/sizes":
			_ = json.NewEncoder(w).Encode(map[string]any{"sizes": handler.sizes})
		case r.Method == http.MethodGet && r.URL.Path == "/v2/images":
			_ = json.NewEncoder(w).Encode(map[string]any{"images": handler.images})
		case r.Method == http.MethodGet && r.URL.Path == "/v2/account/keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"ssh_keys": handler.sshKeys})
		case r.Method == http.MethodGet && r.URL.Path == "/v2/droplets":
			tag := r.URL.Query().Get("tag_name")
			_ = json.NewEncoder(w).Encode(map[string]any{"droplets": handler.dropletsByTag[tag]})
		case r.Method == http.MethodPost && r.URL.Path == "/v2/droplets":
			var req dropletCreateBody
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			if handler.onCreateDroplet != nil {
				handler.onCreateDroplet(t, req)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"droplet": godo.Droplet{ID: 1, Name: req.Name, Status: "new"}})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v2/droplets/"):
			id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/v2/droplets/"))
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
