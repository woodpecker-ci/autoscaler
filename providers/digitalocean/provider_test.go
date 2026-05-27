package digitalocean

import (
	"context"
	"errors"
	"testing"
	"text/template"

	"github.com/digitalocean/godo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// --- mocks ---

type mockDroplets struct{ mock.Mock }

func (m *mockDroplets) Create(ctx context.Context, req *godo.DropletCreateRequest) (*godo.Droplet, *godo.Response, error) {
	args := m.Called(ctx, req)
	d, _ := args.Get(0).(*godo.Droplet)
	r, _ := args.Get(1).(*godo.Response)
	return d, r, args.Error(2)
}

func (m *mockDroplets) Delete(ctx context.Context, id int) (*godo.Response, error) {
	args := m.Called(ctx, id)
	r, _ := args.Get(0).(*godo.Response)
	return r, args.Error(1)
}

func (m *mockDroplets) ListByTag(ctx context.Context, tag string, opts *godo.ListOptions) ([]godo.Droplet, *godo.Response, error) {
	args := m.Called(ctx, tag, opts)
	d, _ := args.Get(0).([]godo.Droplet)
	r, _ := args.Get(1).(*godo.Response)
	return d, r, args.Error(2)
}

type mockKeys struct{ mock.Mock }

func (m *mockKeys) List(ctx context.Context, opts *godo.ListOptions) ([]godo.Key, *godo.Response, error) {
	args := m.Called(ctx, opts)
	k, _ := args.Get(0).([]godo.Key)
	r, _ := args.Get(1).(*godo.Response)
	return k, r, args.Error(2)
}

// --- helpers ---

func lastPageResp() *godo.Response {
	return &godo.Response{Links: &godo.Links{}}
}

func newProvider(d dropletsService, k keysService) *Provider {
	return &Provider{
		name:     "digitalocean",
		region:   "nyc3",
		size:     "s-1vcpu-1gb",
		image:    "ubuntu-22-04-x64",
		poolTag:  "woodpecker-pool:test-pool",
		tags:     []string{"woodpecker-pool:test-pool"},
		config:   &config.Config{},
		droplets: d,
		keys:     k,
	}
}

// --- DeployAgent ---

func TestDeployAgent_InvalidUserData(t *testing.T) {
	p := newProvider(&mockDroplets{}, &mockKeys{})
	p.userDataTemplate = template.Must(template.New("").Parse("{{.InvalidField}}"))

	err := p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "RenderUserDataTemplate")
}

func TestDeployAgent_CreateError(t *testing.T) {
	d := &mockDroplets{}
	d.On("Create", mock.Anything, mock.Anything).Return(nil, lastPageResp(), errors.New("api error"))
	t.Cleanup(func() { d.AssertExpectations(t) })

	p := newProvider(d, &mockKeys{})
	err := p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Droplets.Create")
}

func TestDeployAgent_Success(t *testing.T) {
	d := &mockDroplets{}
	d.On("Create", mock.Anything, mock.MatchedBy(func(req *godo.DropletCreateRequest) bool {
		return req.Name == "agent-1" &&
			req.Region == "nyc3" &&
			req.Size == "s-1vcpu-1gb" &&
			len(req.Tags) == 1 && req.Tags[0] == "woodpecker-pool:test-pool"
	})).Return(&godo.Droplet{ID: 1}, lastPageResp(), nil)
	t.Cleanup(func() { d.AssertExpectations(t) })

	p := newProvider(d, &mockKeys{})
	err := p.DeployAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	assert.NoError(t, err)
}

// --- RemoveAgent ---

func TestRemoveAgent_NotFound(t *testing.T) {
	d := &mockDroplets{}
	d.On("ListByTag", mock.Anything, "woodpecker-pool:test-pool", mock.Anything).
		Return([]godo.Droplet{}, lastPageResp(), nil)
	t.Cleanup(func() { d.AssertExpectations(t) })

	p := newProvider(d, &mockKeys{})
	err := p.RemoveAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	assert.NoError(t, err)
	d.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}

func TestRemoveAgent_DeleteError(t *testing.T) {
	d := &mockDroplets{}
	d.On("ListByTag", mock.Anything, "woodpecker-pool:test-pool", mock.Anything).
		Return([]godo.Droplet{{ID: 42, Name: "agent-1"}}, lastPageResp(), nil)
	d.On("Delete", mock.Anything, 42).Return(nil, errors.New("api error"))
	t.Cleanup(func() { d.AssertExpectations(t) })

	p := newProvider(d, &mockKeys{})
	err := p.RemoveAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Droplets.Delete")
}

func TestRemoveAgent_Success(t *testing.T) {
	d := &mockDroplets{}
	d.On("ListByTag", mock.Anything, "woodpecker-pool:test-pool", mock.Anything).
		Return([]godo.Droplet{{ID: 42, Name: "agent-1"}}, lastPageResp(), nil)
	d.On("Delete", mock.Anything, 42).Return(lastPageResp(), nil)
	t.Cleanup(func() { d.AssertExpectations(t) })

	p := newProvider(d, &mockKeys{})
	err := p.RemoveAgent(t.Context(), &woodpecker.Agent{Name: "agent-1"})
	assert.NoError(t, err)
}

// --- ListDeployedAgentNames ---

func TestListDeployedAgentNames_Empty(t *testing.T) {
	d := &mockDroplets{}
	d.On("ListByTag", mock.Anything, "woodpecker-pool:test-pool", mock.Anything).
		Return([]godo.Droplet{}, lastPageResp(), nil)
	t.Cleanup(func() { d.AssertExpectations(t) })

	p := newProvider(d, &mockKeys{})
	names, err := p.ListDeployedAgentNames(t.Context())
	assert.NoError(t, err)
	assert.Empty(t, names)
}

func TestListDeployedAgentNames_ReturnsNames(t *testing.T) {
	d := &mockDroplets{}
	d.On("ListByTag", mock.Anything, "woodpecker-pool:test-pool", mock.Anything).
		Return([]godo.Droplet{{Name: "agent-1"}, {Name: "agent-2"}}, lastPageResp(), nil)
	t.Cleanup(func() { d.AssertExpectations(t) })

	p := newProvider(d, &mockKeys{})
	names, err := p.ListDeployedAgentNames(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, []string{"agent-1", "agent-2"}, names)
}

func TestListDeployedAgentNames_Pagination(t *testing.T) {
	page1Resp := &godo.Response{Links: &godo.Links{Pages: &godo.Pages{Next: "http://example.com?page=2"}}}
	d := &mockDroplets{}
	d.On("ListByTag", mock.Anything, "woodpecker-pool:test-pool", &godo.ListOptions{PerPage: perPage}).
		Return([]godo.Droplet{{Name: "agent-1"}}, page1Resp, nil)
	d.On("ListByTag", mock.Anything, "woodpecker-pool:test-pool", &godo.ListOptions{PerPage: perPage, Page: 2}).
		Return([]godo.Droplet{{Name: "agent-2"}}, lastPageResp(), nil)
	t.Cleanup(func() { d.AssertExpectations(t) })

	p := newProvider(d, &mockKeys{})
	names, err := p.ListDeployedAgentNames(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, []string{"agent-1", "agent-2"}, names)
}

// --- setupKeypair ---

func TestSetupKeypair_NoKeys(t *testing.T) {
	k := &mockKeys{}
	k.On("List", mock.Anything, mock.Anything).Return([]godo.Key{}, lastPageResp(), nil)
	t.Cleanup(func() { k.AssertExpectations(t) })

	p := newProvider(&mockDroplets{}, k)
	err := p.setupKeypair(t.Context(), "")
	assert.ErrorIs(t, err, ErrSSHKeyNotFound)
}

func TestSetupKeypair_PreferredName(t *testing.T) {
	k := &mockKeys{}
	k.On("List", mock.Anything, mock.Anything).Return([]godo.Key{
		{ID: 1, Name: "other"},
		{ID: 2, Name: "mykey"},
	}, lastPageResp(), nil)
	t.Cleanup(func() { k.AssertExpectations(t) })

	p := newProvider(&mockDroplets{}, k)
	err := p.setupKeypair(t.Context(), "mykey")
	assert.NoError(t, err)
	assert.Equal(t, 2, p.sshKeyID)
}

func TestSetupKeypair_DefaultCandidates(t *testing.T) {
	k := &mockKeys{}
	k.On("List", mock.Anything, mock.Anything).Return([]godo.Key{
		{ID: 5, Name: "some-other-key"},
		{ID: 7, Name: "woodpecker"},
	}, lastPageResp(), nil)
	t.Cleanup(func() { k.AssertExpectations(t) })

	p := newProvider(&mockDroplets{}, k)
	err := p.setupKeypair(t.Context(), "")
	assert.NoError(t, err)
	assert.Equal(t, 7, p.sshKeyID)
}

func TestSetupKeypair_FallbackToFirst(t *testing.T) {
	k := &mockKeys{}
	k.On("List", mock.Anything, mock.Anything).Return([]godo.Key{
		{ID: 7, Name: "some-key"},
	}, lastPageResp(), nil)
	t.Cleanup(func() { k.AssertExpectations(t) })

	p := newProvider(&mockDroplets{}, k)
	err := p.setupKeypair(t.Context(), "")
	assert.NoError(t, err)
	assert.Equal(t, 7, p.sshKeyID)
}
