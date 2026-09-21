package e2e_test

import (
	"context"
	"maps"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/autoscaler/server"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var (
	dockerAMD64 = types.Capability{Platform: "linux/amd64", Backend: types.BackendDocker}
	dockerARM64 = types.Capability{Platform: "linux/arm64", Backend: types.BackendDocker}
)

// harness wires the real engine.Autoscaler to in-memory fakes of the provider
// and the woodpecker server, so tests can drive whole reconcile cycles through
// the public API and assert on the resulting pool.
type harness struct {
	config     *config.Config
	autoscaler *engine.Autoscaler
	provider   *fakeProvider
	woodpecker *fakeWoodpecker
}

func newHarness(t *testing.T, cfg *config.Config, capabilities ...types.Capability) *harness {
	t.Helper()

	provider := newFakeProvider(capabilities...)
	woodpecker := newFakeWoodpecker()

	autoscaler, err := engine.NewAutoscaler(t.Context(), provider, woodpecker, cfg)
	require.NoError(t, err)

	return &harness{config: cfg, autoscaler: autoscaler, provider: provider, woodpecker: woodpecker}
}

func testConfig(minAgents, maxAgents int) *config.Config {
	return &config.Config{
		PoolID:                 "e2e",
		MinAgents:              minAgents,
		MaxAgents:              maxAgents,
		WorkflowsPerAgent:      1,
		AgentIdleTimeout:       time.Minute,
		AgentInactivityTimeout: time.Hour,
		BillingModel:           types.BillingPerSecond,
	}
}

// hourlyConfig is a single-agent pool on an hourly-billed provider with a
// one-minute teardown window at the end of every paid hour.
func hourlyConfig() *config.Config {
	cfg := testConfig(0, 1)
	cfg.BillingModel = types.BillingHourlyRoundUp
	cfg.ReconciliationInterval = time.Minute
	cfg.AgentBillingTeardownMargin = time.Minute
	return cfg
}

func (h *harness) reconcile(t *testing.T) {
	t.Helper()
	require.NoError(t, h.autoscaler.Reconcile(t.Context()))
}

// connectAgents simulates every freshly deployed agent registering with the
// server and reporting the capability it was deployed for.
func (h *harness) connectAgents(t *testing.T) {
	t.Helper()
	for name, capability := range h.provider.deployed {
		agent := h.woodpecker.agentByName(t, name)
		if agent.LastContact != 0 {
			continue
		}
		agent.CustomLabels = maps.Clone(h.config.ExtraAgentLabels)
		agent.Platform = capability.Platform
		agent.Backend = string(capability.Backend)
		agent.LastContact = time.Now().Unix()
		agent.LastWork = time.Now().Unix()
		h.woodpecker.put(agent)
	}
}

// addConnectedAgent seeds an already-registered, idle agent that the provider
// also knows about, as a starting point for replacement scenarios.
func (h *harness) addConnectedAgent(t *testing.T, name string, capability types.Capability) *woodpecker.Agent {
	t.Helper()
	agent, err := h.woodpecker.AgentCreate(&woodpecker.Agent{Name: name})
	require.NoError(t, err)
	agent.Platform = capability.Platform
	agent.Backend = string(capability.Backend)
	agent.LastContact = time.Now().Unix()
	agent.LastWork = time.Now().Add(-2 * time.Minute).Unix()
	agent.CustomLabels = maps.Clone(h.config.ExtraAgentLabels)
	h.woodpecker.put(agent)
	h.provider.deployed[name] = capability
	return agent
}

// markIdle pushes every agent's last-work time past the idle timeout.
func (h *harness) markIdle() {
	for _, agent := range h.woodpecker.agents {
		agent.LastWork = time.Now().Add(-2 * time.Minute).Unix()
	}
}

func (h *harness) agentIDForPlatform(t *testing.T, platform string) int64 {
	t.Helper()
	for _, agent := range h.woodpecker.agents {
		if agent.Platform == platform {
			return agent.ID
		}
	}
	require.FailNowf(t, "no connected agent", "platform %q", platform)
	return 0
}

// realWorkflowTask mirrors what /queue/info returns: the workflow's own labels
// plus the org-id/repo and internal labels the server stamps on every task.
func realWorkflowTask(id, platform string) woodpecker.Task {
	return woodpecker.Task{
		ID: id,
		Labels: map[string]string{
			"platform":                         platform,
			"backend":                          "docker",
			"org-id":                           "42",
			"repo":                             "acme/api",
			"woodpecker-ci.org/pipeline-event": "push",
		},
	}
}

func runningOn(task woodpecker.Task, agentID int64) woodpecker.Task {
	task.AgentID = agentID
	return task
}

// fakeProvider records the agents deployed to and removed from it.
type fakeProvider struct {
	capabilities      []types.Capability
	capabilitiesCalls int
	capabilitiesErr   error
	deployed          map[string]types.Capability
	deployErr         error
	removeErr         error
	listErr           error
}

var _ types.Provider = (*fakeProvider)(nil)

func newFakeProvider(capabilities ...types.Capability) *fakeProvider {
	return &fakeProvider{
		capabilities: capabilities,
		deployed:     map[string]types.Capability{},
	}
}

func (p *fakeProvider) Capabilities(context.Context) ([]types.Capability, error) {
	p.capabilitiesCalls++
	return append([]types.Capability(nil), p.capabilities...), p.capabilitiesErr
}

func (p *fakeProvider) DeployAgent(_ context.Context, agent *woodpecker.Agent, capability types.Capability) error {
	if p.deployErr != nil {
		return p.deployErr
	}
	p.deployed[agent.Name] = capability
	return nil
}

func (p *fakeProvider) RemoveAgent(_ context.Context, agent *woodpecker.Agent) error {
	if p.removeErr != nil {
		return p.removeErr
	}
	delete(p.deployed, agent.Name)
	return nil
}

func (p *fakeProvider) ListDeployedAgentNames(context.Context) ([]string, error) {
	if p.listErr != nil {
		return nil, p.listErr
	}
	names := make([]string, 0, len(p.deployed))
	for name := range p.deployed {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (*fakeProvider) BillingModel() types.BillingModel {
	return types.BillingPerSecond
}

func (p *fakeProvider) deployedCapabilities() []types.Capability {
	capabilities := make([]types.Capability, 0, len(p.deployed))
	for _, capability := range p.deployed {
		capabilities = append(capabilities, capability)
	}
	return capabilities
}

// fakeWoodpecker is an in-memory stand-in for the woodpecker server: it tracks
// registered agents and the queue snapshot. Only the handful of client methods
// the engine calls are implemented; the embedded interface satisfies the rest.
type fakeWoodpecker struct {
	server.Client
	nextID    int64
	agents    map[int64]*woodpecker.Agent
	queue     woodpecker.Info
	listErr   error
	createErr error
	updateErr error
	deleteErr error
	tasksErr  error
	queueErr  error
}

var _ server.Client = (*fakeWoodpecker)(nil)

func newFakeWoodpecker() *fakeWoodpecker {
	return &fakeWoodpecker{nextID: 1, agents: map[int64]*woodpecker.Agent{}}
}

func (s *fakeWoodpecker) put(agent *woodpecker.Agent) {
	s.agents[agent.ID] = cloneAgent(agent)
}

func (s *fakeWoodpecker) AgentList() ([]*woodpecker.Agent, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	agents := make([]*woodpecker.Agent, 0, len(s.agents))
	for _, agent := range s.agents {
		agents = append(agents, cloneAgent(agent))
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].ID < agents[j].ID })
	return agents, nil
}

func (s *fakeWoodpecker) AgentCreate(agent *woodpecker.Agent) (*woodpecker.Agent, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	created := cloneAgent(agent)
	created.ID = s.nextID
	created.Created = time.Now().Unix()
	s.nextID++
	s.put(created)
	return cloneAgent(created), nil
}

func (s *fakeWoodpecker) AgentUpdate(agent *woodpecker.Agent) (*woodpecker.Agent, error) {
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	s.put(agent)
	return cloneAgent(agent), nil
}

func (s *fakeWoodpecker) AgentDelete(agentID int64) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.agents, agentID)
	return nil
}

func (s *fakeWoodpecker) AgentTasksList(agentID int64) ([]*woodpecker.Task, error) {
	if s.tasksErr != nil {
		return nil, s.tasksErr
	}
	tasks := make([]*woodpecker.Task, 0)
	for i := range s.queue.Running {
		if s.queue.Running[i].AgentID == agentID {
			task := s.queue.Running[i]
			tasks = append(tasks, &task)
		}
	}
	return tasks, nil
}

func (s *fakeWoodpecker) QueueInfo() (*woodpecker.Info, error) {
	if s.queueErr != nil {
		return nil, s.queueErr
	}
	queue := s.queue
	queue.Pending = append([]woodpecker.Task(nil), s.queue.Pending...)
	queue.Running = append([]woodpecker.Task(nil), s.queue.Running...)
	return &queue, nil
}

func (s *fakeWoodpecker) agentByName(t *testing.T, name string) *woodpecker.Agent {
	t.Helper()
	for _, agent := range s.agents {
		if agent.Name == name {
			return cloneAgent(agent)
		}
	}
	require.FailNowf(t, "agent not registered", "name %q", name)
	return nil
}

func cloneAgent(agent *woodpecker.Agent) *woodpecker.Agent {
	clone := *agent
	clone.CustomLabels = maps.Clone(agent.CustomLabels)
	return &clone
}
