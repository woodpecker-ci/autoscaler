package engine

import (
	"context"
	"testing"
	"time"

	"github.com/franela/goblin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"go.woodpecker-ci.org/autoscaler/config"
	mocks_engine "go.woodpecker-ci.org/autoscaler/engine/mocks"
	mocks_server "go.woodpecker-ci.org/autoscaler/server/mocks"
	"go.woodpecker-ci.org/woodpecker/v2/woodpecker-go/woodpecker"
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
	g := goblin.Goblin(t)

	g.Describe("Agent creation", func() {
		g.It("Should create new agent (MinAgents)", func() {
			autoscaler := Autoscaler{client: &MockClient{
				pending: 0,
			}, config: &config.Config{
				WorkflowsPerAgent: 1,
				MaxAgents:         2,
				MinAgents:         1,
			}}

			value, _ := autoscaler.calcAgents(context.TODO())
			g.Assert(value).Equal(float64(1))
		})

		g.It("Should create single agent", func() {
			autoscaler := Autoscaler{client: &MockClient{
				pending: 2,
			}, config: &config.Config{
				WorkflowsPerAgent: 5,
				MaxAgents:         3,
			}}

			value, _ := autoscaler.calcAgents(context.TODO())
			g.Assert(value).Equal(float64(1))
		})

		g.It("Should create multiple agents", func() {
			autoscaler := Autoscaler{client: &MockClient{
				pending: 6,
			}, config: &config.Config{
				WorkflowsPerAgent: 5,
				MaxAgents:         3,
			}}

			value, _ := autoscaler.calcAgents(context.TODO())
			g.Assert(value).Equal(float64(2))
		})

		g.It("Should create new agent (MaxAgents)", func() {
			autoscaler := Autoscaler{client: &MockClient{
				pending: 2,
			}, config: &config.Config{
				WorkflowsPerAgent: 1,
				MaxAgents:         2,
			}, agents: []*woodpecker.Agent{
				{Name: "pool-1-agent-1234"},
			}}

			value, _ := autoscaler.calcAgents(context.TODO())
			g.Assert(value).Equal(float64(1))
		})

		g.It("Should not create new agent (availableAgents)", func() {
			autoscaler := Autoscaler{client: &MockClient{
				workers: 2,
				pending: 2,
			}, config: &config.Config{
				WorkflowsPerAgent: 1,
				MaxAgents:         2,
			}}

			value, _ := autoscaler.calcAgents(context.TODO())
			g.Assert(value).Equal(float64(0))
		})
	})
}

func Test_getQueueInfo(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	g := goblin.Goblin(t)

	g.Describe("Queue Info", func() {
		g.It("Should not filter", func() {
			autoscaler := Autoscaler{
				client: &MockClient{
					pending: 2,
				},
				config: &config.Config{},
			}

			free, running, pending, _ := autoscaler.getQueueInfo(context.TODO())
			g.Assert(free).Equal(0)
			g.Assert(running).Equal(0)
			g.Assert(pending).Equal(2)
		})
		g.It("Should filter one by label", func() {
			autoscaler := Autoscaler{
				client: &MockClient{
					pending: 2,
				},
				config: &config.Config{
					FilterLabels: "arch=amd64",
				},
			}

			free, running, pending, _ := autoscaler.getQueueInfo(context.TODO())
			g.Assert(free).Equal(0)
			g.Assert(running).Equal(1)
			g.Assert(pending).Equal(1)
		})
		g.It("Should filter all by label", func() {
			autoscaler := Autoscaler{
				client: &MockClient{
					pending: 2,
				},
				config: &config.Config{
					FilterLabels: "arch=arm64",
				},
			}

			free, running, pending, _ := autoscaler.getQueueInfo(context.TODO())
			g.Assert(free).Equal(0)
			g.Assert(running).Equal(0)
			g.Assert(pending).Equal(0)
		})
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

func Test_cleanupDanglingAgents(t *testing.T) {
	t.Run("should remove agent that is only present on woodpecker (not provider)", func(t *testing.T) {
		ctx := context.Background()
		client := mocks_server.NewMockClient(t)
		provider := mocks_engine.NewMockProvider(t)
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
		ctx := context.Background()
		client := mocks_server.NewMockClient(t)
		provider := mocks_engine.NewMockProvider(t)
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
		ctx := context.Background()
		client := mocks_server.NewMockClient(t)
		provider := mocks_engine.NewMockProvider(t)
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
		ctx := context.Background()
		client := mocks_server.NewMockClient(t)
		provider := mocks_engine.NewMockProvider(t)
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

func Test_removeDrainedAgents(t *testing.T) {
	t.Run("should remove agent", func(t *testing.T) {
		ctx := context.Background()
		client := mocks_server.NewMockClient(t)
		provider := mocks_engine.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: []*woodpecker.Agent{
				{ID: 1, Name: "pool-1-agent-1", NoSchedule: false},
				{ID: 2, Name: "pool-1-agent-2", NoSchedule: true},
				{ID: 3, Name: "pool-1-agent-3", NoSchedule: false},
			},
			provider: provider,
			client:   client,
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
		ctx := context.Background()
		client := mocks_server.NewMockClient(t)
		provider := mocks_engine.NewMockProvider(t)
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
