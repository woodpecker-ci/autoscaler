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
	// pointer to distinguish "absent" (public networking on) from explicit false
	PublicNetworking *bool  `json:"public_networking"`
	VPCUUID          string `json:"vpc_uuid"`
}

func TestNewResolvesConfigAndUsesExistingAutoSSHKey(t *testing.T) {
	api := newTestAPIServer(t, testAPIHandler{
		regions: []godo.Region{{Slug: "nyc1", Available: true}},
		sizes:   []godo.Size{{Slug: "s-1vcpu-1gb", Regions: []string{"nyc1"}, Available: true}},
		images:  []godo.Image{{ID: 101, Slug: "ubuntu-24-04-x64", Name: "24.04 x64", Distribution: "Ubuntu"}},
		sshKeys: []godo.Key{
			{Name: "something-else", Fingerprint: "ff:00"},
			{Name: autoSSHKeyName, Fingerprint: "aa:bb"},
		},
		onCreateKey: func(t *testing.T, _ godo.KeyCreateRequest) {
			t.Error("Keys.Create must not be called when the auto key already exists")
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

func TestNewCreatesAutoSSHKeyWhenNoneConfigured(t *testing.T) {
	var created godo.KeyCreateRequest
	api := newTestAPIServer(t, testAPIHandler{
		regions: []godo.Region{{Slug: "nyc1", Available: true}},
		sizes:   []godo.Size{{Slug: "s-1vcpu-1gb", Regions: []string{"nyc1"}, Available: true}},
		images:  []godo.Image{{ID: 101, Slug: "ubuntu-24-04-x64", Name: "24.04 x64", Distribution: "Ubuntu"}},
		sshKeys: []godo.Key{{Name: "something-else", Fingerprint: "ff:00"}},
		onCreateKey: func(_ *testing.T, req godo.KeyCreateRequest) {
			created = req
		},
	})

	cmd := newTestCommand(t, ProviderFlags, []string{"--digitalocean-api-token=token"})

	p, err := newWithClient(t.Context(), cmd, &config.Config{PoolID: "pool-1"}, newTestClient(t, api))
	require.NoError(t, err)

	assert.Equal(t, autoSSHKeyName, created.Name)
	assert.True(t, strings.HasPrefix(created.PublicKey, "ssh-ed25519 "), "expected an ed25519 public key, got %q", created.PublicKey)

	doProvider, ok := p.(*provider)
	require.True(t, ok)
	// the fake API returns the fingerprint "ge:ne:ra:te:d" for created keys
	assert.Equal(t, []godo.DropletCreateSSHKey{{Fingerprint: "ge:ne:ra:te:d"}}, doProvider.sshKeys)
}

func TestNewFailsOnUnknownConfiguredSSHKey(t *testing.T) {
	api := newTestAPIServer(t, testAPIHandler{
		regions: []godo.Region{{Slug: "nyc1", Available: true}},
		sizes:   []godo.Size{{Slug: "s-1vcpu-1gb", Regions: []string{"nyc1"}, Available: true}},
		images:  []godo.Image{{ID: 101, Slug: "ubuntu-24-04-x64", Name: "24.04 x64", Distribution: "Ubuntu"}},
		sshKeys: []godo.Key{{Name: "build", Fingerprint: "11:22"}},
	})

	cmd := newTestCommand(t, ProviderFlags, []string{
		"--digitalocean-api-token=token",
		"--digitalocean-ssh-keys=missing",
	})

	_, err := newWithClient(t.Context(), cmd, &config.Config{PoolID: "pool-1"}, newTestClient(t, api))
	require.ErrorIs(t, err, ErrSSHKeyNotFound)
}

func TestNewSizeNotAvailable(t *testing.T) {
	tests := []struct {
		name  string
		sizes []godo.Size
		want  string
	}{
		{
			name:  "not offered in region",
			sizes: []godo.Size{{Slug: "s-1vcpu-1gb", Regions: []string{"fra1"}, Available: true}},
			want:  "not offered in region nyc1",
		},
		{
			name:  "currently not available",
			sizes: []godo.Size{{Slug: "s-1vcpu-1gb", Regions: []string{"nyc1"}, Available: false}},
			want:  "currently not available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newTestAPIServer(t, testAPIHandler{
				regions: []godo.Region{{Slug: "nyc1", Available: true}},
				sizes:   tt.sizes,
			})

			cmd := newTestCommand(t, ProviderFlags, []string{"--digitalocean-api-token=token"})

			_, err := newWithClient(t.Context(), cmd, &config.Config{PoolID: "pool-1"}, newTestClient(t, api))
			require.ErrorIs(t, err, ErrSizeNotAvailable)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestNewRejectsIPv6WithoutIPv4(t *testing.T) {
	// public IPv6 defaults to true, so disabling only IPv4 must fail fast
	cmd := newTestCommand(t, ProviderFlags, []string{
		"--digitalocean-api-token=token",
		"--digitalocean-public-ipv4-enable=false",
	})

	_, err := newWithClient(t.Context(), cmd, &config.Config{PoolID: "pool-1"}, nil)
	require.ErrorIs(t, err, ErrIPv6RequiresIPv4)
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
		sshKeys: []godo.Key{{Name: autoSSHKeyName, Fingerprint: "aa:bb"}},
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
	}, types.Capability{Platform: "linux/amd64", Backend: types.BackendDocker})
	require.NoError(t, err)

	assert.Equal(t, "pool-1-agent-1", created.Name)
	assert.Equal(t, "nyc1", created.Region)
	assert.Equal(t, "s-1vcpu-1gb", created.Size)
	assert.Equal(t, "ubuntu-24-04-x64", created.Image)
	assert.Equal(t, []string{"aa:bb"}, created.SSHKeys)
	assert.True(t, created.IPv6)
	assert.Nil(t, created.PublicNetworking, "public_networking must be omitted with the default public IPv4")
	assert.Contains(t, created.Tags, "team-ci")
	assert.Contains(t, created.Tags, "wp-autoscaler-pool-pool-1")
	assert.NotEmpty(t, created.UserData)
	assert.Contains(t, created.UserData, "ip -4 route add blackhole 169.254.169.254/32")
}

func TestDeployAgentWithoutIPv6(t *testing.T) {
	var created dropletCreateBody
	api := newTestAPIServer(t, testAPIHandler{
		regions: []godo.Region{{Slug: "nyc1", Available: true}},
		sizes:   []godo.Size{{Slug: "s-1vcpu-1gb", Regions: []string{"nyc1"}, Available: true}},
		images:  []godo.Image{{ID: 101, Slug: "ubuntu-24-04-x64", Name: "24.04 x64", Distribution: "Ubuntu"}},
		sshKeys: []godo.Key{{Name: autoSSHKeyName, Fingerprint: "aa:bb"}},
		onCreateDroplet: func(_ *testing.T, req dropletCreateBody) {
			created = req
		},
	})

	cmd := newTestCommand(t, ProviderFlags, []string{
		"--digitalocean-api-token=token",
		"--digitalocean-public-ipv6-enable=false",
	})

	p, err := newWithClient(t.Context(), cmd, &config.Config{PoolID: "pool-1"}, newTestClient(t, api))
	require.NoError(t, err)

	require.NoError(t, p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "pool-1-agent-1"}, types.Capability{Platform: "linux/amd64", Backend: types.BackendDocker}))

	assert.False(t, created.IPv6)
	assert.Nil(t, created.PublicNetworking, "public_networking must be omitted when the public interface is enabled")
}

func TestDeployAgentPrivateDroplet(t *testing.T) {
	var created dropletCreateBody
	api := newTestAPIServer(t, testAPIHandler{
		regions: []godo.Region{{Slug: "nyc1", Available: true}},
		sizes:   []godo.Size{{Slug: "s-1vcpu-1gb", Regions: []string{"nyc1"}, Available: true}},
		images:  []godo.Image{{ID: 101, Slug: "ubuntu-24-04-x64", Name: "24.04 x64", Distribution: "Ubuntu"}},
		sshKeys: []godo.Key{{Name: autoSSHKeyName, Fingerprint: "aa:bb"}},
		natGateways: []*godo.VPCNATGateway{{
			ID:     "gw-1",
			Name:   "ci-egress",
			Region: "nyc1",
			VPCs: []*godo.IngressVPC{
				{VpcUUID: "vpc-other"},
				{VpcUUID: "vpc-123", DefaultGateway: true},
			},
		}},
		onCreateDroplet: func(_ *testing.T, req dropletCreateBody) {
			created = req
		},
	})

	cmd := newTestCommand(t, ProviderFlags, []string{
		"--digitalocean-api-token=token",
		"--digitalocean-public-ipv4-enable=false",
		"--digitalocean-public-ipv6-enable=false",
		"--digitalocean-nat-gateway=ci-egress",
	})

	p, err := newWithClient(t.Context(), cmd, &config.Config{PoolID: "pool-1"}, newTestClient(t, api))
	require.NoError(t, err)

	require.NoError(t, p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "pool-1-agent-1"}, types.Capability{Platform: "linux/amd64", Backend: types.BackendDocker}))

	assert.False(t, created.IPv6)
	require.NotNil(t, created.PublicNetworking)
	assert.False(t, *created.PublicNetworking)
	assert.Equal(t, "vpc-123", created.VPCUUID, "agents must join the VPC the gateway serves as default gateway")
}

func TestNewNATGatewayValidation(t *testing.T) {
	gateway := &godo.VPCNATGateway{
		ID:     "gw-1",
		Name:   "ci-egress",
		Region: "nyc1",
		VPCs:   []*godo.IngressVPC{{VpcUUID: "vpc-123", DefaultGateway: true}},
	}

	tests := []struct {
		name        string
		args        []string
		natGateways []*godo.VPCNATGateway
		wantErr     error
	}{
		{
			name:        "required when public IPv4 is disabled",
			args:        []string{"--digitalocean-public-ipv4-enable=false", "--digitalocean-public-ipv6-enable=false"},
			natGateways: []*godo.VPCNATGateway{gateway},
			wantErr:     ErrNATGatewayRequired,
		},
		{
			name:        "unknown gateway",
			args:        []string{"--digitalocean-public-ipv4-enable=false", "--digitalocean-public-ipv6-enable=false", "--digitalocean-nat-gateway=missing"},
			natGateways: []*godo.VPCNATGateway{gateway},
			wantErr:     ErrNATGatewayNotFound,
		},
		{
			name: "wrong region",
			args: []string{"--digitalocean-public-ipv4-enable=false", "--digitalocean-public-ipv6-enable=false", "--digitalocean-nat-gateway=gw-1"},
			natGateways: []*godo.VPCNATGateway{{
				ID: "gw-1", Name: "ci-egress", Region: "fra1",
				VPCs: []*godo.IngressVPC{{VpcUUID: "vpc-123", DefaultGateway: true}},
			}},
			wantErr: ErrNATGatewayInvalid,
		},
		{
			name: "no default gateway VPC",
			args: []string{"--digitalocean-public-ipv4-enable=false", "--digitalocean-public-ipv6-enable=false", "--digitalocean-nat-gateway=ci-egress"},
			natGateways: []*godo.VPCNATGateway{{
				ID: "gw-1", Name: "ci-egress", Region: "nyc1",
				VPCs: []*godo.IngressVPC{{VpcUUID: "vpc-123"}},
			}},
			wantErr: ErrNATGatewayInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newTestAPIServer(t, testAPIHandler{
				regions:     []godo.Region{{Slug: "nyc1", Available: true}},
				sizes:       []godo.Size{{Slug: "s-1vcpu-1gb", Regions: []string{"nyc1"}, Available: true}},
				images:      []godo.Image{{ID: 101, Slug: "ubuntu-24-04-x64", Name: "24.04 x64", Distribution: "Ubuntu"}},
				sshKeys:     []godo.Key{{Name: autoSSHKeyName, Fingerprint: "aa:bb"}},
				natGateways: tt.natGateways,
			})

			cmd := newTestCommand(t, ProviderFlags, append([]string{"--digitalocean-api-token=token"}, tt.args...))

			_, err := newWithClient(t.Context(), cmd, &config.Config{PoolID: "pool-1"}, newTestClient(t, api))
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
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

func TestCapabilities(t *testing.T) {
	p := &provider{name: "digitalocean"}

	caps, err := p.Capabilities(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []types.Capability{{Platform: "linux/amd64", Backend: types.BackendDocker}}, caps)
}

func TestDeployAgentRejectsUnsupportedCapability(t *testing.T) {
	p := &provider{name: "digitalocean"}

	err := p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"}, types.Capability{Platform: "linux/arm64", Backend: types.BackendDocker})
	require.ErrorContains(t, err, "linux/amd64")

	err = p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"}, types.Capability{Platform: "linux/amd64", Backend: types.BackendKubernetes})
	require.ErrorContains(t, err, "docker")
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
	onCreateKey     func(*testing.T, godo.KeyCreateRequest)
	natGateways     []*godo.VPCNATGateway
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
		case r.Method == http.MethodPost && r.URL.Path == "/v2/account/keys":
			var req godo.KeyCreateRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			if handler.onCreateKey != nil {
				handler.onCreateKey(t, req)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ssh_key": godo.Key{Name: req.Name, Fingerprint: "ge:ne:ra:te:d"}})
		case r.Method == http.MethodGet && r.URL.Path == "/v2/vpc_nat_gateways":
			_ = json.NewEncoder(w).Encode(map[string]any{"vpc_nat_gateways": handler.natGateways})
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
