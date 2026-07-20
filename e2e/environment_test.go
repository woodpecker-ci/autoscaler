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

type testEnvironment struct {
	autoscaler *engine.Autoscaler
	provider   *mockProvider
	woodpecker *mockWoodpeckerServer
}

func newTestEnvironment(t *testing.T, cfg *config.Config, capabilities ...types.Capability) *testEnvironment {
	t.Helper()

	provider := &mockProvider{
		capabilities: capabilities,
		deployed:     make(map[string]types.Capability),
	}
	woodpeckerServer := newMockWoodpeckerServer()
	autoscaler, err := engine.NewAutoscaler(t.Context(), provider, woodpeckerServer, cfg)
	require.NoError(t, err)

	return &testEnvironment{
		autoscaler: autoscaler,
		provider:   provider,
		woodpecker: woodpeckerServer,
	}
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

func (e *testEnvironment) reconcile(t *testing.T) {
	t.Helper()
	require.NoError(t, e.autoscaler.Reconcile(t.Context()))
}

func (e *testEnvironment) connectDeployedAgents(t *testing.T) {
	t.Helper()

	for name, capability := range e.provider.deployed {
		agent := e.woodpecker.agentByName(t, name)
		agent.Platform = capability.Platform
		agent.Backend = string(capability.Backend)
		agent.LastContact = time.Now().Unix()
		agent.LastWork = time.Now().Unix()
		e.woodpecker.agents[agent.ID] = cloneAgent(agent)
	}
}

func (e *testEnvironment) addConnectedAgent(t *testing.T, name string, capability types.Capability) *woodpecker.Agent {
	t.Helper()

	agent, err := e.woodpecker.AgentCreate(&woodpecker.Agent{Name: name})
	require.NoError(t, err)
	agent.Platform = capability.Platform
	agent.Backend = string(capability.Backend)
	agent.LastContact = time.Now().Unix()
	agent.LastWork = time.Now().Add(-2 * time.Minute).Unix()
	e.woodpecker.agents[agent.ID] = cloneAgent(agent)
	e.provider.deployed[name] = capability
	return agent
}

func (e *testEnvironment) agentIDForPlatform(t *testing.T, platform string) int64 {
	t.Helper()

	for _, agent := range e.woodpecker.agents {
		if agent.Platform == platform {
			return agent.ID
		}
	}
	require.FailNow(t, "no connected agent for platform", platform)
	return 0
}

func (e *testEnvironment) markAgentsIdle() {
	for id, agent := range e.woodpecker.agents {
		agent.LastWork = time.Now().Add(-2 * time.Minute).Unix()
		e.woodpecker.agents[id] = cloneAgent(agent)
	}
}

type mockProvider struct {
	capabilities []types.Capability
	deployed     map[string]types.Capability
}

var _ types.Provider = (*mockProvider)(nil)

func (p *mockProvider) Capabilities(context.Context) ([]types.Capability, error) {
	return append([]types.Capability(nil), p.capabilities...), nil
}

func (p *mockProvider) DeployAgent(_ context.Context, agent *woodpecker.Agent, capability types.Capability) error {
	p.deployed[agent.Name] = capability
	return nil
}

func (p *mockProvider) RemoveAgent(_ context.Context, agent *woodpecker.Agent) error {
	delete(p.deployed, agent.Name)
	return nil
}

func (p *mockProvider) ListDeployedAgentNames(context.Context) ([]string, error) {
	names := make([]string, 0, len(p.deployed))
	for name := range p.deployed {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (*mockProvider) BillingModel() types.BillingModel {
	return types.BillingPerSecond
}

func (p *mockProvider) deployedCapabilities() []types.Capability {
	capabilities := make([]types.Capability, 0, len(p.deployed))
	for _, capability := range p.deployed {
		capabilities = append(capabilities, capability)
	}
	return capabilities
}

type mockWoodpeckerServer struct {
	server.Client
	nextAgentID int64
	agents      map[int64]*woodpecker.Agent
	queue       woodpecker.Info
}

var _ server.Client = (*mockWoodpeckerServer)(nil)

func newMockWoodpeckerServer() *mockWoodpeckerServer {
	return &mockWoodpeckerServer{
		nextAgentID: 1,
		agents:      make(map[int64]*woodpecker.Agent),
	}
}

func (s *mockWoodpeckerServer) AgentList() ([]*woodpecker.Agent, error) {
	agents := make([]*woodpecker.Agent, 0, len(s.agents))
	for _, agent := range s.agents {
		agents = append(agents, cloneAgent(agent))
	}
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].ID < agents[j].ID
	})
	return agents, nil
}

func (s *mockWoodpeckerServer) AgentCreate(agent *woodpecker.Agent) (*woodpecker.Agent, error) {
	created := cloneAgent(agent)
	created.ID = s.nextAgentID
	created.Created = time.Now().Unix()
	s.nextAgentID++
	s.agents[created.ID] = cloneAgent(created)
	return cloneAgent(created), nil
}

func (s *mockWoodpeckerServer) AgentUpdate(agent *woodpecker.Agent) (*woodpecker.Agent, error) {
	s.agents[agent.ID] = cloneAgent(agent)
	return cloneAgent(agent), nil
}

func (s *mockWoodpeckerServer) AgentDelete(agentID int64) error {
	delete(s.agents, agentID)
	return nil
}

func (s *mockWoodpeckerServer) AgentTasksList(agentID int64) ([]*woodpecker.Task, error) {
	tasks := make([]*woodpecker.Task, 0)
	for i := range s.queue.Running {
		if s.queue.Running[i].AgentID == agentID {
			task := s.queue.Running[i]
			tasks = append(tasks, &task)
		}
	}
	return tasks, nil
}

func (s *mockWoodpeckerServer) QueueInfo() (*woodpecker.Info, error) {
	queue := s.queue
	queue.Pending = append([]woodpecker.Task(nil), s.queue.Pending...)
	queue.Running = append([]woodpecker.Task(nil), s.queue.Running...)
	return &queue, nil
}

func (s *mockWoodpeckerServer) agentByName(t *testing.T, name string) *woodpecker.Agent {
	t.Helper()

	for _, agent := range s.agents {
		if agent.Name == name {
			return cloneAgent(agent)
		}
	}
	require.FailNow(t, "agent not registered on Woodpecker", name)
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
