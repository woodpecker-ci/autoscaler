package digitalocean

import (
	"context"
	"testing"

	"github.com/digitalocean/godo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type fakeDroplets struct {
	createReq *godo.DropletCreateRequest
	deletedID int
	pages     [][]godo.Droplet
}

func (f *fakeDroplets) Create(_ context.Context, req *godo.DropletCreateRequest) (*godo.Droplet, *godo.Response, error) {
	f.createReq = req
	return &godo.Droplet{ID: 42}, nil, nil
}

func (f *fakeDroplets) Delete(_ context.Context, dropletID int) (*godo.Response, error) {
	f.deletedID = dropletID
	return nil, nil
}

func (f *fakeDroplets) ListByTag(_ context.Context, _ string, opt *godo.ListOptions) ([]godo.Droplet, *godo.Response, error) {
	page := opt.Page - 1
	if page < 0 || page >= len(f.pages) {
		return nil, nil, nil
	}

	return f.pages[page], responseWithNext(page < len(f.pages)-1), nil
}

type fakeKeys struct {
	keys []godo.Key
}

func (f fakeKeys) List(_ context.Context, _ *godo.ListOptions) ([]godo.Key, *godo.Response, error) {
	return f.keys, nil, nil
}

type fakeImages struct {
	image *godo.Image
}

func (f fakeImages) GetBySlug(_ context.Context, _ string) (*godo.Image, *godo.Response, error) {
	return f.image, nil, nil
}

type fakeRegions struct {
	regions []godo.Region
}

func (f fakeRegions) List(_ context.Context, _ *godo.ListOptions) ([]godo.Region, *godo.Response, error) {
	return f.regions, nil, nil
}

type fakeSizes struct {
	sizes []godo.Size
}

func (f fakeSizes) List(_ context.Context, _ *godo.ListOptions) ([]godo.Size, *godo.Response, error) {
	return f.sizes, nil, nil
}

func TestDeployAgentBuildsDropletRequest(t *testing.T) {
	droplets := &fakeDroplets{}
	provider := &Provider{
		name:       "digitalocean",
		region:     "nyc1",
		size:       "s-1vcpu-1gb",
		image:      "ubuntu-22-04-x64",
		tags:       []string{"wp-autoscaler-pool-main", "custom-tag"},
		sshKeys:    []godo.DropletCreateSSHKey{{ID: 123}},
		enableIPv6: true,
		vpcUUID:    "vpc-uuid",
		config: &config.Config{
			Image:             "woodpeckerci/woodpecker-agent:next",
			GRPCAddress:       "grpc.example.com",
			GRPCSecure:        true,
			WorkflowsPerAgent: 2,
		},
		droplets: droplets,
	}

	err := provider.DeployAgent(t.Context(), &woodpecker.Agent{Name: "agent-1", Token: "secret"})
	require.NoError(t, err)
	require.NotNil(t, droplets.createReq)

	assert.Equal(t, "agent-1", droplets.createReq.Name)
	assert.Equal(t, "nyc1", droplets.createReq.Region)
	assert.Equal(t, "s-1vcpu-1gb", droplets.createReq.Size)
	assert.Equal(t, "ubuntu-22-04-x64", droplets.createReq.Image.Slug)
	assert.Equal(t, []string{"wp-autoscaler-pool-main", "custom-tag"}, droplets.createReq.Tags)
	assert.Equal(t, []godo.DropletCreateSSHKey{{ID: 123}}, droplets.createReq.SSHKeys)
	assert.True(t, droplets.createReq.IPv6)
	assert.Equal(t, "vpc-uuid", droplets.createReq.VPCUUID)
	assert.Contains(t, droplets.createReq.UserData, "WOODPECKER_AGENT_SECRET=secret")
}

func TestRemoveAgentDeletesOnlyPoolDroplet(t *testing.T) {
	droplets := &fakeDroplets{
		pages: [][]godo.Droplet{
			{
				{ID: 7, Name: "agent-1"},
				{ID: 8, Name: "agent-2"},
			},
		},
	}
	provider := &Provider{name: "digitalocean", poolTag: "wp-autoscaler-pool-main", droplets: droplets}

	err := provider.RemoveAgent(t.Context(), &woodpecker.Agent{Name: "agent-2"})
	require.NoError(t, err)
	assert.Equal(t, 8, droplets.deletedID)
}

func TestListDeployedAgentNamesUsesPoolTagPagination(t *testing.T) {
	provider := &Provider{
		name:    "digitalocean",
		poolTag: "wp-autoscaler-pool-main",
		droplets: &fakeDroplets{
			pages: [][]godo.Droplet{
				{{Name: "agent-1"}},
				{{Name: "agent-2"}},
			},
		},
	}

	names, err := provider.ListDeployedAgentNames(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"agent-1", "agent-2"}, names)
}

func TestResolveSSHKeys(t *testing.T) {
	provider := &Provider{
		name: "digitalocean",
		keys: fakeKeys{keys: []godo.Key{
			{ID: 123, Name: "woodpecker", Fingerprint: "fingerprint-123"},
			{ID: 456, Name: "other", Fingerprint: "fingerprint-456"},
		}},
	}

	keys, err := provider.resolveSSHKeys(t.Context(), nil)
	require.NoError(t, err)
	assert.Equal(t, []godo.DropletCreateSSHKey{{ID: 123}}, keys)

	keys, err = provider.resolveSSHKeys(t.Context(), []string{"fingerprint-456"})
	require.NoError(t, err)
	assert.Equal(t, []godo.DropletCreateSSHKey{{ID: 456}}, keys)
}

func TestResolveConfigValidatesImageRegion(t *testing.T) {
	provider := &Provider{
		name:   "digitalocean",
		region: "nyc1",
		size:   "s-1vcpu-1gb",
		image:  "ubuntu-22-04-x64",
		regions: fakeRegions{regions: []godo.Region{
			{Slug: "nyc1", Available: true, Sizes: []string{"s-1vcpu-1gb"}},
		}},
		sizes: fakeSizes{sizes: []godo.Size{
			{Slug: "s-1vcpu-1gb", Available: true, Regions: []string{"nyc1"}},
		}},
		images: fakeImages{image: &godo.Image{Slug: "ubuntu-22-04-x64", Regions: []string{"sfo3"}}},
		keys: fakeKeys{keys: []godo.Key{
			{ID: 123, Name: "woodpecker", Fingerprint: "fingerprint-123"},
		}},
	}

	err := provider.resolveConfig(t.Context(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrImageUnavailableInRegion.Error())
}

func TestDigitalOceanTags(t *testing.T) {
	assert.True(t, validTag("letters:digits-123_under"))
	assert.False(t, validTag("wp.autoscaler/pool=main"))
	assert.Equal(t, "wp-autoscaler-pool-main-default", buildInternalTag("pool", "main/default"))
}

func responseWithNext(next bool) *godo.Response {
	if !next {
		return nil
	}

	return &godo.Response{
		Links: &godo.Links{
			Pages: &godo.Pages{Next: "next"},
		},
	}
}
