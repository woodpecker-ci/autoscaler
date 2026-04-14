package engine

import (
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"go.woodpecker-ci.org/autoscaler/config"
	mocks_provider "go.woodpecker-ci.org/autoscaler/engine/provider/mocks"
	"go.woodpecker-ci.org/autoscaler/engine/scheduler"
	mocks_server "go.woodpecker-ci.org/autoscaler/server/mocks"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type MockClient struct {
	workers       int
	running       int
	pending       int
	waitingOnDeps int
	woodpecker.Client
}

func (m MockClient) QueueInfo() (*woodpecker.Info, error) {
	info := &woodpecker.Info{}

	info.Stats.Workers = m.workers
	info.Stats.Running = m.running
	info.Stats.Pending = m.pending
	info.Stats.WaitingOnDeps = m.waitingOnDeps

	info.Pending = []woodpecker.Task{
		{
			Labels: map[string]string{
				"arch": "amd64",
			},
		},
	}
	info.Running = []woodpecker.Task{
		{
			Labels: map[string]string{
				"arch": "amd64",
			},
		},
	}

	return info, nil
}

func Test_calcAgents(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)

	t.Run("should create new agent (MinAgents)", func(t *testing.T) {
		autoscaler := Autoscaler{client: &MockClient{
			pending: 0,
		}, config: &config.Config{
			WorkflowsPerAgent: 1,
			MaxAgents:         2,
			MinAgents:         1,
		}, scheduler: &scheduler.SimpleScheduler{}}

		value, _ := autoscaler.calcAgents(t.Context())
		assert.Equal(t, float64(1), value)
	})

	t.Run("should create single agent", func(t *testing.T) {
		autoscaler := Autoscaler{client: &MockClient{
			pending: 2,
		}, config: &config.Config{
			WorkflowsPerAgent: 5,
			MaxAgents:         3,
		}, scheduler: &scheduler.SimpleScheduler{}}

		value, _ := autoscaler.calcAgents(t.Context())
		assert.Equal(t, float64(1), value)
	})

	t.Run("should create multiple agents", func(t *testing.T) {
		autoscaler := Autoscaler{client: &MockClient{
			pending: 6,
		}, config: &config.Config{
			WorkflowsPerAgent: 5,
			MaxAgents:         3,
		}, scheduler: &scheduler.SimpleScheduler{}}

		value, _ := autoscaler.calcAgents(t.Context())
		assert.Equal(t, float64(2), value)
	})

	t.Run("should create new agent (MaxAgents)", func(t *testing.T) {
		autoscaler := Autoscaler{client: &MockClient{
			pending: 2,
		}, config: &config.Config{
			WorkflowsPerAgent: 1,
			MaxAgents:         2,
		}, agents: []*woodpecker.Agent{
			{Name: "pool-1-agent-1234"},
		}, scheduler: &scheduler.SimpleScheduler{}}

		value, _ := autoscaler.calcAgents(t.Context())
		assert.Equal(t, float64(1), value)
	})

	t.Run("should not create new agent when pool already covers demand", func(t *testing.T) {
		// Previously this tested "availableAgents" from queueInfo.Stats.Workers (free slots
		// reported fleet-wide by the server). That value is no longer passed through the
		// Scheduler interface; free capacity is now derived from the pool agents list.
		// The equivalent test: one pool agent with WorkflowsPerAgent=2 can absorb 2 pending
		// tasks, so no new agent should be created.
		autoscaler := Autoscaler{client: &MockClient{
			pending: 2,
		}, config: &config.Config{
			WorkflowsPerAgent: 2,
			MaxAgents:         2,
		}, agents: []*woodpecker.Agent{
			{Name: "pool-1-agent-1234"},
		}, scheduler: &scheduler.SimpleScheduler{}}

		value, _ := autoscaler.calcAgents(t.Context())
		assert.Equal(t, float64(0), value)
	})
}

func Test_filterTasksByLabel(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)

	tasks := []woodpecker.Task{
		{Labels: map[string]string{"arch": "amd64"}},
		{Labels: map[string]string{"arch": "arm64"}},
	}

	t.Run("should return all tasks when label matches all", func(t *testing.T) {
		result := filterTasksByLabel(tasks, "arch", "amd64")
		assert.Equal(t, 1, len(result))
		assert.Equal(t, "amd64", result[0].Labels["arch"])
	})

	t.Run("should return empty when label matches none", func(t *testing.T) {
		result := filterTasksByLabel(tasks, "arch", "riscv64")
		assert.Equal(t, 0, len(result))
	})

	t.Run("should return all matching tasks", func(t *testing.T) {
		mixed := []woodpecker.Task{
			{Labels: map[string]string{"arch": "amd64"}},
			{Labels: map[string]string{"arch": "amd64"}},
			{Labels: map[string]string{"arch": "arm64"}},
		}
		result := filterTasksByLabel(mixed, "arch", "amd64")
		assert.Equal(t, 2, len(result))
	})
}

func Test_calcAgents_labelFilter(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)

	// MockClient always returns 1 running amd64 task and 1 pending amd64 task.

	t.Run("should not filter when FilterLabels is empty", func(t *testing.T) {
		autoscaler := Autoscaler{
			client: &MockClient{pending: 2},
			config: &config.Config{
				WorkflowsPerAgent: 1,
				MaxAgents:         3,
			},
			scheduler: &scheduler.SimpleScheduler{},
		}

		value, _ := autoscaler.calcAgents(t.Context())
		// 1 running + 1 pending = 2 tasks, 0 pool agents → needs 2 agents
		assert.Equal(t, float64(2), value)
	})

	t.Run("should filter by label (amd64 matches both tasks)", func(t *testing.T) {
		autoscaler := Autoscaler{
			client: &MockClient{pending: 2},
			config: &config.Config{
				WorkflowsPerAgent: 1,
				MaxAgents:         3,
				FilterLabels:      "arch=amd64",
			},
			scheduler: &scheduler.SimpleScheduler{},
		}

		value, _ := autoscaler.calcAgents(t.Context())
		// 1 amd64 running + 1 amd64 pending = 2 tasks → needs 2 agents
		assert.Equal(t, float64(2), value)
	})

	t.Run("should filter all out by label (arm64 matches nothing)", func(t *testing.T) {
		autoscaler := Autoscaler{
			client: &MockClient{pending: 2},
			config: &config.Config{
				WorkflowsPerAgent: 1,
				MaxAgents:         3,
				FilterLabels:      "arch=arm64",
			},
			scheduler: &scheduler.SimpleScheduler{},
		}

		value, _ := autoscaler.calcAgents(t.Context())
		// 0 matching tasks → 0 agents needed
		assert.Equal(t, float64(0), value)
	})
}

func Test_getPoolAgents(t *testing.T) {
	autoscaler := Autoscaler{
		agents: []*woodpecker.Agent{
			{ID: 1, Name: "pool-1-agent-1", NoSchedule: false},
			{ID: 2, Name: "pool-1-agent-2", NoSchedule: true},
			{ID: 3, Name: "pool-1-agent-3", NoSchedule: false},
		},
	}

	agents := autoscaler.getPoolAgents(false)
	assert.Equal(t, 3, len(agents))

	agents = autoscaler.getPoolAgents(true)
	assert.Equal(t, 2, len(agents))
}

func Test_createAgents(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)

	t.Run("should create a new agent", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_provider.NewMockProvider(t)
		autoscaler := Autoscaler{
			client:   client,
			provider: provider,
			config: &config.Config{
				PoolID: "1",
			},
		}

		client.On("AgentCreate", mock.Anything).Return(&woodpecker.Agent{Name: "pool-1-agent-1"}, nil)
		provider.On("DeployAgent", ctx, mock.Anything, mock.Anything).Return(nil)

		err := autoscaler.createAgents(ctx, 1)
		assert.NoError(t, err)
	})

	t.Run("should reuse an no-schedule agent first before creating a new one", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_provider.NewMockProvider(t)
		autoscaler := Autoscaler{
			client:   client,
			provider: provider,
			agents: []*woodpecker.Agent{
				{
					ID:         1,
					NoSchedule: true,
				},
			},
			config: &config.Config{
				PoolID: "1",
			},
		}

		client.On("AgentUpdate", mock.MatchedBy(func(agent *woodpecker.Agent) bool {
			return agent.ID == 1 && agent.NoSchedule == false
		})).Return(nil, nil)
		client.On("AgentCreate", mock.Anything).Return(&woodpecker.Agent{Name: "pool-1-agent-1"}, nil)
		provider.On("DeployAgent", ctx, mock.Anything, mock.Anything).Return(nil)

		err := autoscaler.createAgents(ctx, 2)
		assert.NoError(t, err)
	})
}

func Test_cleanupDanglingAgents(t *testing.T) {
	t.Run("should remove agent that is only present on woodpecker (not provider)", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_provider.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: []*woodpecker.Agent{
				{ID: 1, Name: "pool-1-agent-1", NoSchedule: false},
			},
			provider: provider,
			client:   client,
		}

		provider.On("ListDeployedAgentNames", mock.Anything).Return(nil, nil)
		client.On("AgentDelete", int64(1)).Return(nil)

		err := autoscaler.cleanupDanglingAgents(ctx)
		assert.NoError(t, err)
	})

	t.Run("should remove agent that is only present on provider (not woodpecker)", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_provider.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: []*woodpecker.Agent{
				{ID: 1, Name: "pool-1-agent-1", NoSchedule: false},
			},
			provider: provider,
			client:   client,
		}

		provider.On("ListDeployedAgentNames", mock.Anything).Return([]string{"pool-1-agent-1", "pool-1-agent-2"}, nil)
		provider.On("RemoveAgent", mock.Anything, mock.MatchedBy(func(agent *woodpecker.Agent) bool {
			return agent.Name == "pool-1-agent-2"
		})).Return(nil)

		err := autoscaler.cleanupDanglingAgents(ctx)
		assert.NoError(t, err)
	})
}

func Test_cleanupStaleAgents(t *testing.T) {
	t.Run("should remove agent that never connected (last contact = 0) in over 15 minutes", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_provider.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: []*woodpecker.Agent{
				{
					ID:          1,
					Name:        "active agent",
					NoSchedule:  false,
					Created:     time.Now().Add(-time.Minute * 20).Unix(), // created 20 minutes ago
					LastContact: time.Now().Add(-time.Minute * 5).Unix(),  // last contact 5 minutes ago
				},
				{
					ID:          2,
					Name:        "never contacted agent",
					NoSchedule:  false,
					Created:     time.Now().Add(-time.Minute * 20).Unix(), // created 20 minutes ago
					LastContact: 0,                                        // never contacted
				},
			},
			provider: provider,
			client:   client,
			config: &config.Config{
				AgentInactivityTimeout: time.Minute * 15,
			},
		}

		client.On("AgentTasksList", int64(2)).Return(nil, nil)
		client.On("AgentDelete", int64(2)).Return(nil)
		provider.On("RemoveAgent", mock.Anything, mock.MatchedBy(func(agent *woodpecker.Agent) bool {
			return agent.ID == 2
		})).Return(nil)

		err := autoscaler.cleanupStaleAgents(ctx)
		assert.NoError(t, err)
	})

	t.Run("should remove agent that has lost connection for more than 15 minutes", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_provider.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: []*woodpecker.Agent{
				{
					ID:          1,
					Name:        "active agent",
					NoSchedule:  false,
					Created:     time.Now().Add(-time.Minute * 20).Unix(), // created 20 minutes ago
					LastContact: time.Now().Add(-time.Minute * 5).Unix(),  // last contact 5 minutes ago
				},
				{
					ID:          2,
					Name:        "stale agent",
					NoSchedule:  false,
					Created:     time.Now().Add(-time.Minute * 20).Unix(), // created 20 minutes ago
					LastContact: time.Now().Add(-time.Minute * 20).Unix(), // last contact 20 minutes ago
				},
			},
			provider: provider,
			client:   client,
			config: &config.Config{
				AgentInactivityTimeout: time.Minute * 15,
			},
		}

		client.On("AgentTasksList", int64(2)).Return(nil, nil)
		client.On("AgentDelete", int64(2)).Return(nil)
		provider.On("RemoveAgent", mock.Anything, mock.MatchedBy(func(agent *woodpecker.Agent) bool {
			return agent.ID == 2
		})).Return(nil)

		err := autoscaler.cleanupStaleAgents(ctx)
		assert.NoError(t, err)
	})
}

func Test_isAgentIdle(t *testing.T) {
	t.Run("should return false if agent has tasks", func(t *testing.T) {
		client := mocks_server.NewMockClient(t)
		autoscaler := Autoscaler{
			client: client,
			config: &config.Config{
				AgentIdleTimeout: time.Minute * 15,
			},
		}

		client.On("AgentTasksList", int64(1)).Return([]*woodpecker.Task{
			{ID: "1"},
		}, nil)

		idle, err := autoscaler.isAgentIdle(&woodpecker.Agent{
			ID:         1,
			Name:       "pool-1-agent-1",
			NoSchedule: false,
		})
		assert.NoError(t, err)
		assert.False(t, idle)
	})

	t.Run("should return false if agent has done work recently", func(t *testing.T) {
		client := mocks_server.NewMockClient(t)
		autoscaler := Autoscaler{
			client: client,
			config: &config.Config{
				AgentIdleTimeout: time.Minute * 15,
			},
		}

		client.On("AgentTasksList", int64(1)).Return(nil, nil)

		idle, err := autoscaler.isAgentIdle(&woodpecker.Agent{
			ID:         1,
			Name:       "pool-1-agent-1",
			NoSchedule: false,
			LastWork:   time.Now().Add(-time.Minute * 10).Unix(),
		})
		assert.NoError(t, err)
		assert.False(t, idle)
	})

	t.Run("should return true if agent is idle", func(t *testing.T) {
		client := mocks_server.NewMockClient(t)
		autoscaler := Autoscaler{
			client: client,
			config: &config.Config{
				AgentIdleTimeout: time.Minute * 15,
			},
		}

		client.On("AgentTasksList", int64(1)).Return(nil, nil) // no tasks

		idle, err := autoscaler.isAgentIdle(&woodpecker.Agent{
			ID:         1,
			Name:       "pool-1-agent-1",
			NoSchedule: false,
			LastWork:   time.Now().Add(-time.Minute * 20).Unix(), // last work 20 minutes ago
		})
		assert.NoError(t, err)
		assert.True(t, idle)
	})
}

func Test_drainAgents(t *testing.T) {
	t.Run("should drain agents and skip no-schedule ones", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_provider.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: []*woodpecker.Agent{
				{ID: 1, Name: "pool-1-agent-1", NoSchedule: false, LastContact: time.Now().Add(-time.Minute * 2).Unix()},
				{ID: 2, Name: "pool-1-agent-2", NoSchedule: true, LastContact: time.Now().Add(-time.Minute * 2).Unix()},
				{ID: 3, Name: "pool-1-agent-3", NoSchedule: true, LastContact: time.Now().Add(-time.Minute * 2).Unix()},
				{ID: 4, Name: "pool-1-agent-4", NoSchedule: false, LastContact: time.Now().Add(-time.Minute * 2).Unix()},
			},
			provider: provider,
			client:   client,
			config: &config.Config{
				AgentIdleTimeout: time.Minute * 15,
			},
		}

		client.On("AgentUpdate", mock.MatchedBy(func(agent *woodpecker.Agent) bool {
			return (agent.ID == 1 || agent.ID == 4) && agent.NoSchedule == true
		})).Return(nil, nil)

		err := autoscaler.drainAgents(ctx, 2)
		assert.NoError(t, err)
		assert.True(t, autoscaler.agents[0].NoSchedule)
		assert.True(t, autoscaler.agents[3].NoSchedule)
	})

	t.Run("should not remove an agent that never connected", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_provider.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: []*woodpecker.Agent{
				{ID: 1, Name: "pool-1-agent-1", NoSchedule: false, LastContact: 0},
			},
			provider: provider,
			client:   client,
			config: &config.Config{
				AgentIdleTimeout: time.Minute * 15,
			},
		}

		err := autoscaler.drainAgents(ctx, 1)
		assert.NoError(t, err)
		assert.False(t, autoscaler.agents[0].NoSchedule)
	})

	t.Run("should not remove an agent that has recently done some work", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_provider.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: []*woodpecker.Agent{
				{
					ID:          1,
					Name:        "pool-1-agent-1",
					NoSchedule:  false,
					LastContact: time.Now().Add(-time.Minute * 2).Unix(), // last contact 2 minutes ago
					LastWork:    time.Now().Add(-time.Minute * 5).Unix(), // last work 5 minutes ago
				},
			},
			provider: provider,
			client:   client,
			config: &config.Config{
				AgentIdleTimeout: time.Minute * 15,
			},
		}

		err := autoscaler.drainAgents(ctx, 1)
		assert.NoError(t, err)
		assert.False(t, autoscaler.agents[0].NoSchedule)
	})
}

func Test_removeDrainedAgents(t *testing.T) {
	t.Run("should remove agent", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_provider.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: []*woodpecker.Agent{
				{ID: 1, Name: "pool-1-agent-1", NoSchedule: false},
				{ID: 2, Name: "pool-1-agent-2", NoSchedule: true},
				{ID: 3, Name: "pool-1-agent-3", NoSchedule: false},
			},
			provider: provider,
			client:   client,
			config: &config.Config{
				AgentIdleTimeout: time.Minute * 15,
			},
		}

		client.On("AgentTasksList", int64(2)).Return(nil, nil)
		provider.On("RemoveAgent", mock.Anything, mock.MatchedBy(func(agent *woodpecker.Agent) bool {
			return agent.ID == 2
		})).Return(nil)
		client.On("AgentDelete", int64(2)).Return(nil)

		err := autoscaler.removeDrainedAgents(ctx)
		assert.NoError(t, err)
	})

	t.Run("should not remove agent with tasks", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_provider.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: []*woodpecker.Agent{
				{ID: 1, Name: "pool-1-agent-1", NoSchedule: false},
				{ID: 2, Name: "pool-1-agent-2", NoSchedule: true},
				{ID: 3, Name: "pool-1-agent-3", NoSchedule: false},
			},
			provider: provider,
			client:   client,
		}

		client.On("AgentTasksList", int64(2)).Return([]*woodpecker.Task{
			{ID: "1"},
		}, nil)

		err := autoscaler.removeDrainedAgents(ctx)
		assert.NoError(t, err)
	})
}
