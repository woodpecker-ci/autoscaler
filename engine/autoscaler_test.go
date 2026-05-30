package engine

import (
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	mocks_provider "go.woodpecker-ci.org/autoscaler/engine/types/mocks"
	mocks_server "go.woodpecker-ci.org/autoscaler/server/mocks"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// fixedClock returns a clock function pinned to t for deterministic
// billing-window tests.
func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

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
		}}

		value, _ := autoscaler.calcAgents(t.Context())
		assert.Equal(t, float64(1), value)
	})

	t.Run("should create single agent", func(t *testing.T) {
		autoscaler := Autoscaler{client: &MockClient{
			pending: 2,
		}, config: &config.Config{
			WorkflowsPerAgent: 5,
			MaxAgents:         3,
		}}

		value, _ := autoscaler.calcAgents(t.Context())
		assert.Equal(t, float64(1), value)
	})

	t.Run("should create multiple agents", func(t *testing.T) {
		autoscaler := Autoscaler{client: &MockClient{
			pending: 6,
		}, config: &config.Config{
			WorkflowsPerAgent: 5,
			MaxAgents:         3,
		}}

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
		}}

		value, _ := autoscaler.calcAgents(t.Context())
		assert.Equal(t, float64(1), value)
	})

	t.Run("should not create new agent (availableAgents)", func(t *testing.T) {
		autoscaler := Autoscaler{client: &MockClient{
			workers: 2,
			pending: 2,
		}, config: &config.Config{
			WorkflowsPerAgent: 1,
			MaxAgents:         2,
		}}

		value, _ := autoscaler.calcAgents(t.Context())
		assert.Equal(t, float64(0), value)
	})
}

func Test_getQueueInfo(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	t.Run("should not filter", func(t *testing.T) {
		autoscaler := Autoscaler{
			client: &MockClient{
				pending: 2,
			},
			config: &config.Config{},
		}

		free, running, pending, _ := autoscaler.getQueueInfo(t.Context())
		assert.Equal(t, 0, free)
		assert.Equal(t, 0, running)
		assert.Equal(t, 2, pending)
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
		provider.On("DeployAgent", ctx, mock.Anything).Return(nil)

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
		provider.On("DeployAgent", ctx, mock.Anything).Return(nil)

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
			config:   &config.Config{},
		}

		client.On("AgentTasksList", int64(2)).Return([]*woodpecker.Task{
			{ID: "1"},
		}, nil)

		err := autoscaler.removeDrainedAgents(ctx)
		assert.NoError(t, err)
	})
}

func Test_inTeardownWindow(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	// margin 2m + reconciliation interval 1m => 3m window at the end of each hour
	cfg := &config.Config{
		AgentBillingTeardownMargin: 2 * time.Minute,
		ReconciliationInterval:     time.Minute,
	}
	autoscaler := Autoscaler{config: cfg, clock: fixedClock(now)}

	createdAgo := func(d time.Duration) *woodpecker.Agent {
		return &woodpecker.Agent{Created: now.Add(-d).Unix()}
	}

	t.Run("inside the window of the first paid hour", func(t *testing.T) {
		assert.True(t, autoscaler.inTeardownWindow(createdAgo(58*time.Minute)))
	})

	t.Run("just before the window opens", func(t *testing.T) {
		assert.False(t, autoscaler.inTeardownWindow(createdAgo(56*time.Minute)))
	})

	t.Run("early in the paid hour stays warm", func(t *testing.T) {
		assert.False(t, autoscaler.inTeardownWindow(createdAgo(5*time.Minute)))
	})

	t.Run("window recurs every hour", func(t *testing.T) {
		assert.True(t, autoscaler.inTeardownWindow(createdAgo(118*time.Minute)))
		assert.False(t, autoscaler.inTeardownWindow(createdAgo(90*time.Minute)))
	})

	t.Run("agent without a creation time is never in the window", func(t *testing.T) {
		assert.False(t, autoscaler.inTeardownWindow(&woodpecker.Agent{Created: 0}))
	})

	t.Run("clock skew (negative age) is never in the window", func(t *testing.T) {
		assert.False(t, autoscaler.inTeardownWindow(createdAgo(-5*time.Minute)))
	})
}

func Test_drainAgents_hourlyRoundUp(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	t.Run("only drains agents inside their teardown window", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_provider.NewMockProvider(t)
		autoscaler := Autoscaler{
			clock: fixedClock(now),
			agents: []*woodpecker.Agent{
				// idle but mid-hour => kept warm, even though it has no recent work
				{ID: 1, Name: "pool-1-agent-1", LastContact: now.Add(-time.Minute).Unix(), LastWork: now.Add(-30 * time.Minute).Unix(), Created: now.Add(-30 * time.Minute).Unix()},
				// in the teardown window => eligible to drain
				{ID: 2, Name: "pool-1-agent-2", LastContact: now.Add(-time.Minute).Unix(), LastWork: now.Add(-time.Minute).Unix(), Created: now.Add(-58 * time.Minute).Unix()},
			},
			provider: provider,
			client:   client,
			config: &config.Config{
				BillingModel:               types.BillingHourlyRoundUp,
				AgentBillingTeardownMargin: 2 * time.Minute,
				ReconciliationInterval:     time.Minute,
			},
		}

		client.On("AgentUpdate", mock.MatchedBy(func(agent *woodpecker.Agent) bool {
			return agent.ID == 2 && agent.NoSchedule
		})).Return(nil, nil)

		err := autoscaler.drainAgents(ctx, 2)
		assert.NoError(t, err)
		assert.False(t, autoscaler.agents[0].NoSchedule)
		assert.True(t, autoscaler.agents[1].NoSchedule)
	})
}

func Test_removeDrainedAgents_hourlyRoundUp(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := func() *config.Config {
		return &config.Config{
			BillingModel:               types.BillingHourlyRoundUp,
			AgentBillingTeardownMargin: 2 * time.Minute,
			ReconciliationInterval:     time.Minute,
		}
	}

	t.Run("removes a drained agent inside its teardown window even if it just worked", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_provider.NewMockProvider(t)
		autoscaler := Autoscaler{
			clock: fixedClock(now),
			agents: []*woodpecker.Agent{
				{ID: 2, Name: "pool-1-agent-2", NoSchedule: true, LastWork: now.Add(-time.Minute).Unix(), Created: now.Add(-58 * time.Minute).Unix()},
			},
			provider: provider,
			client:   client,
			config:   cfg(),
		}

		client.On("AgentTasksList", int64(2)).Return(nil, nil)
		provider.On("RemoveAgent", mock.Anything, mock.MatchedBy(func(agent *woodpecker.Agent) bool {
			return agent.ID == 2
		})).Return(nil)
		client.On("AgentDelete", int64(2)).Return(nil)

		err := autoscaler.removeDrainedAgents(ctx)
		assert.NoError(t, err)
	})

	t.Run("keeps a drained agent that rolled into a fresh paid hour", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_provider.NewMockProvider(t)
		autoscaler := Autoscaler{
			clock: fixedClock(now),
			agents: []*woodpecker.Agent{
				// drained while busy near the boundary; now idle 5m into a new paid hour
				{ID: 2, Name: "pool-1-agent-2", NoSchedule: true, LastWork: now.Add(-time.Minute).Unix(), Created: now.Add(-65 * time.Minute).Unix()},
			},
			provider: provider,
			client:   client,
			config:   cfg(),
		}

		err := autoscaler.removeDrainedAgents(ctx)
		assert.NoError(t, err)
	})
}

func Test_isAgentIdle_hourlyRoundUp(t *testing.T) {
	t.Run("recent work does not keep an hourly agent busy", func(t *testing.T) {
		client := mocks_server.NewMockClient(t)
		autoscaler := Autoscaler{
			client: client,
			config: &config.Config{
				BillingModel:     types.BillingHourlyRoundUp,
				AgentIdleTimeout: time.Minute * 15,
			},
		}

		client.On("AgentTasksList", int64(1)).Return(nil, nil) // no tasks

		idle, err := autoscaler.isAgentIdle(&woodpecker.Agent{
			ID:       1,
			Name:     "pool-1-agent-1",
			LastWork: time.Now().Add(-time.Minute).Unix(), // worked one minute ago
		})
		assert.NoError(t, err)
		assert.True(t, idle)
	})

	t.Run("an hourly agent with an in-flight task is still busy", func(t *testing.T) {
		client := mocks_server.NewMockClient(t)
		autoscaler := Autoscaler{
			client: client,
			config: &config.Config{BillingModel: types.BillingHourlyRoundUp},
		}

		client.On("AgentTasksList", int64(1)).Return([]*woodpecker.Task{{ID: "1"}}, nil)

		idle, err := autoscaler.isAgentIdle(&woodpecker.Agent{ID: 1, Name: "pool-1-agent-1"})
		assert.NoError(t, err)
		assert.False(t, idle)
	})
}
