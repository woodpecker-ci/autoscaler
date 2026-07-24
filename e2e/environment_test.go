package e2e_test

import (
	"context"
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

// harness wires the real engine.Autoscaler to in-memory fakes of the provider
// and the woodpecker server, so tests can drive whole reconcile cycles through
// the public API and assert on the resulting pool.
type harness struct {
	autoscaler *engine.Autoscaler
	provider   *fakeProvider
	woodpecker *fakeWoodpecker
}

func newHarness(t *testing.T, cfg *config.Config, capabilities ...types.Capability) *harness {
	t.Helper()

	provider := &fakeProvider{
		capabilities: capabilities,
		deployed:     map[string]types.Capability{},
	}
	woodpecker := &fakeWoodpecker{nextID: 1, agents: map[int64]*woodpecker.Agent{}}

	autoscaler, err := engine.NewAutoscaler(t.Context(), provider, woodpecker, cfg)
	require.NoError(t, err)

	return &harness{autoscaler: autoscaler, provider: provider, woodpecker: woodpecker}
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
		agent.Platform = capability.Platform
		agent.Backend = string(capability.Backend)
		agent.LastContact = time.Now().Unix()
		agent.LastWork = time.Now().Unix()
		h.woodpecker.put(agent)
	}
}

// addConnectedAgent seeds an already-registered, idle agent that the provider
// also knows about — the starting point for replacement scenarios.
func (h *harness) addConnectedAgent(t *testing.T, name string, capability types.Capability) {
	t.Helper()
	agent, err := h.woodpecker.AgentCreate(&woodpecker.Agent{Name: name})
	require.NoError(t, err)
	agent.Platform = capability.Platform
	agent.Backend = string(capability.Backend)
	agent.LastContact = time.Now().Unix()
	agent.LastWork = time.Now().Add(-2 * time.Minute).Unix()
	h.woodpecker.put(agent)
	h.provider.deployed[name] = capability
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
	capabilities []types.Capability
	deployed     map[string]types.Capability
}

var _ types.Provider = (*fakeProvider)(nil)

func (p *fakeProvider) Capabilities(context.Context) ([]types.Capability, error) {
	return append([]types.Capability(nil), p.capabilities...), nil
}

func (p *fakeProvider) DeployAgent(_ context.Context, agent *woodpecker.Agent, capability types.Capability) error {
	p.deployed[agent.Name] = capability
	return nil
}

func (p *fakeProvider) RemoveAgent(_ context.Context, agent *woodpecker.Agent) error {
	delete(p.deployed, agent.Name)
	return nil
}

func (p *fakeProvider) ListDeployedAgentNames(context.Context) ([]string, error) {
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
	nextID int64
	agents map[int64]*woodpecker.Agent
	queue  woodpecker.Info
}

var _ server.Client = (*fakeWoodpecker)(nil)

func (s *fakeWoodpecker) put(agent *woodpecker.Agent) {
	s.agents[agent.ID] = cloneAgent(agent)
}

func (s *fakeWoodpecker) AgentList() ([]*woodpecker.Agent, error) {
	agents := make([]*woodpecker.Agent, 0, len(s.agents))
	for _, agent := range s.agents {
		agents = append(agents, cloneAgent(agent))
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].ID < agents[j].ID })
	return agents, nil
}

func (s *fakeWoodpecker) AgentCreate(agent *woodpecker.Agent) (*woodpecker.Agent, error) {
	created := cloneAgent(agent)
	created.ID = s.nextID
	created.Created = time.Now().Unix()
	s.nextID++
	s.put(created)
	return cloneAgent(created), nil
}

func (s *fakeWoodpecker) AgentUpdate(agent *woodpecker.Agent) (*woodpecker.Agent, error) {
	s.put(agent)
	return cloneAgent(agent), nil
}

func (s *fakeWoodpecker) AgentDelete(agentID int64) error {
	delete(s.agents, agentID)
	return nil
}

func (s *fakeWoodpecker) AgentTasksList(agentID int64) ([]*woodpecker.Task, error) {
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
	clone.CustomLabels = make(map[string]string, len(agent.CustomLabels))
	for key, value := range agent.CustomLabels {
		clone.CustomLabels[key] = value
	}
	return &clone
}
