package e2e_test

import (
	"context"
	"maps"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/autoscaler/server"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

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

// AgentListWithOpts pages the agents like the server does: 50 per page,
// ordered by id.
func (s *fakeWoodpecker) AgentListWithOpts(opt woodpecker.AgentListOptions) ([]*woodpecker.Agent, error) {
	const perPage = 50
	agents, err := s.AgentList()
	if err != nil {
		return nil, err
	}
	start := min(max(opt.Page-1, 0)*perPage, len(agents))
	return agents[start:min(start+perPage, len(agents))], nil
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
