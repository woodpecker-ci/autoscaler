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

var (
	dockerAmd64Cap = types.Capability{Platform: "linux/amd64", Backend: types.BackendDocker}
	dockerArm64Cap = types.Capability{Platform: "linux/arm64", Backend: types.BackendDocker}
)

// taskWithLabels is a tiny helper to keep test cases readable.
func taskWithLabels(labels map[string]string) woodpecker.Task {
	return woodpecker.Task{Labels: labels}
}

func Test_planScaling(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)

	t.Run("scales up when pending tasks have a matching bucket", func(t *testing.T) {
		a := Autoscaler{
			providerCapabilities: []types.Capability{dockerAmd64Cap},
			config: &config.Config{
				WorkflowsPerAgent: 1,
				MaxAgents:         5,
				MinAgents:         0,
			},
		}
		decisions := a.planScaling([]woodpecker.Task{
			taskWithLabels(map[string]string{"platform": "linux/amd64"}),
			taskWithLabels(map[string]string{"platform": "linux/amd64"}),
		}, nil)
		assert.Len(t, decisions, 1)
		assert.Equal(t, dockerAmd64Cap, decisions[0].Bucket.Capability)
		assert.Equal(t, 2, decisions[0].Delta)
	})

	t.Run("does not scale for tasks no bucket can serve", func(t *testing.T) {
		// Provider only supports amd64; pending task asks for backend=local
		// which is not in any capability. We must NOT scale up — spinning
		// up amd64 docker agents wouldn't help.
		a := Autoscaler{
			providerCapabilities: []types.Capability{dockerAmd64Cap},
			config: &config.Config{
				WorkflowsPerAgent: 1,
				MaxAgents:         5,
				MinAgents:         0,
			},
		}
		decisions := a.planScaling([]woodpecker.Task{
			taskWithLabels(map[string]string{"backend": "local"}),
		}, nil)
		assert.Empty(t, decisions, "must not scale for unschedulable tasks")
	})

	t.Run("routes tasks to per-platform buckets", func(t *testing.T) {
		a := Autoscaler{
			providerCapabilities: []types.Capability{dockerAmd64Cap, dockerArm64Cap},
			config: &config.Config{
				WorkflowsPerAgent: 1,
				MaxAgents:         5,
				MinAgents:         0,
			},
		}
		decisions := a.planScaling([]woodpecker.Task{
			taskWithLabels(map[string]string{"platform": "linux/amd64"}),
			taskWithLabels(map[string]string{"platform": "linux/arm64"}),
		}, nil)
		assert.Len(t, decisions, 2)
		seen := map[string]int{}
		for _, d := range decisions {
			seen[d.Bucket.Capability.Platform] = d.Delta
		}
		assert.Equal(t, 1, seen["linux/amd64"])
		assert.Equal(t, 1, seen["linux/arm64"])
	})

	t.Run("respects mandatory ! labels", func(t *testing.T) {
		// Operator marks 'gpu' as mandatory on every agent we deploy.
		// A task without 'gpu' is unschedulable on this autoscaler.
		a := Autoscaler{
			providerCapabilities: []types.Capability{dockerAmd64Cap},
			config: &config.Config{
				WorkflowsPerAgent: 1,
				MaxAgents:         5,
				MinAgents:         0,
				ExtraAgentLabels:  map[string]string{"!gpu": "true"},
			},
		}
		// Task that doesn't mention gpu -> not scheduled.
		decisions := a.planScaling([]woodpecker.Task{
			taskWithLabels(map[string]string{"platform": "linux/amd64"}),
		}, nil)
		assert.Empty(t, decisions)

		// Task that explicitly asks for gpu=true -> scheduled.
		decisions = a.planScaling([]woodpecker.Task{
			taskWithLabels(map[string]string{
				"platform": "linux/amd64",
				"gpu":      "true",
			}),
		}, nil)
		assert.Len(t, decisions, 1)
		assert.Equal(t, 1, decisions[0].Delta)
	})

	t.Run("counts existing pool agents in their bucket", func(t *testing.T) {
		// Two pending amd64 tasks, but we already have one online amd64
		// agent — only need one more.
		a := Autoscaler{
			providerCapabilities: []types.Capability{dockerAmd64Cap},
			agents: map[string]*woodpecker.Agent{
				"pool-1-agent-1": {ID: 1, Name: "pool-1-agent-1", Platform: "linux/amd64", Backend: "docker"},
			},
			config: &config.Config{
				WorkflowsPerAgent: 1,
				MaxAgents:         5,
				MinAgents:         0,
			},
		}
		decisions := a.planScaling([]woodpecker.Task{
			taskWithLabels(map[string]string{"platform": "linux/amd64"}),
			taskWithLabels(map[string]string{"platform": "linux/amd64"}),
		}, nil)
		assert.Len(t, decisions, 1)
		assert.Equal(t, 1, decisions[0].Delta)
	})

	t.Run("clamps scale-up to MaxAgents budget", func(t *testing.T) {
		// 10 pending tasks, no agents, but MaxAgents=3 -> only 3 created.
		a := Autoscaler{
			providerCapabilities: []types.Capability{dockerAmd64Cap},
			config: &config.Config{
				WorkflowsPerAgent: 1,
				MaxAgents:         3,
				MinAgents:         0,
			},
		}
		pending := make([]woodpecker.Task, 10)
		for i := range pending {
			pending[i] = taskWithLabels(map[string]string{"platform": "linux/amd64"})
		}
		decisions := a.planScaling(pending, nil)
		assert.Len(t, decisions, 1)
		assert.Equal(t, 3, decisions[0].Delta)
	})

	t.Run("drains idle agents past MinAgents", func(t *testing.T) {
		// No work, three idle online amd64 agents, MinAgents=1 -> ask for
		// two drains.
		a := Autoscaler{
			providerCapabilities: []types.Capability{dockerAmd64Cap},
			agents: map[string]*woodpecker.Agent{
				"pool-1-agent-1": {ID: 1, Name: "pool-1-agent-1", Platform: "linux/amd64", Backend: "docker"},
				"pool-1-agent-2": {ID: 2, Name: "pool-1-agent-2", Platform: "linux/amd64", Backend: "docker"},
				"pool-1-agent-3": {ID: 3, Name: "pool-1-agent-3", Platform: "linux/amd64", Backend: "docker"},
			},
			config: &config.Config{
				WorkflowsPerAgent: 1,
				MaxAgents:         5,
				MinAgents:         1,
			},
		}
		decisions := a.planScaling(nil, nil)
		assert.Len(t, decisions, 1)
		assert.Equal(t, -2, decisions[0].Delta)
	})

	t.Run("returns nothing when no capabilities are known", func(t *testing.T) {
		a := Autoscaler{
			config: &config.Config{
				WorkflowsPerAgent: 1,
				MaxAgents:         5,
				MinAgents:         0,
			},
		}
		decisions := a.planScaling([]woodpecker.Task{taskWithLabels(nil)}, nil)
		assert.Nil(t, decisions)
	})

	t.Run("handles ExtraAgentLabels wildcards", func(t *testing.T) {
		a := Autoscaler{
			providerCapabilities: []types.Capability{dockerAmd64Cap},
			config: &config.Config{
				WorkflowsPerAgent: 1,
				MaxAgents:         5,
				MinAgents:         0,
				ExtraAgentLabels:  map[string]string{"region": "*"},
			},
		}
		decisions := a.planScaling([]woodpecker.Task{
			taskWithLabels(map[string]string{"region": "europe"}),
		}, nil)
		assert.Len(t, decisions, 1)
		assert.Equal(t, 1, decisions[0].Delta)
	})

	t.Run("packs multiple pending tasks per agent at given WorkflowsPerAgent", func(t *testing.T) {
		// 6 pending tasks, WPA=5 -> need ceil(6/5) = 2 agents.
		a := Autoscaler{
			providerCapabilities: []types.Capability{dockerAmd64Cap},
			config: &config.Config{
				WorkflowsPerAgent: 5,
				MaxAgents:         3,
				MinAgents:         0,
			},
		}
		pending := make([]woodpecker.Task, 6)
		for i := range pending {
			pending[i] = taskWithLabels(map[string]string{"platform": "linux/amd64"})
		}
		decisions := a.planScaling(pending, nil)
		assert.Len(t, decisions, 1)
		assert.Equal(t, 2, decisions[0].Delta)
	})
}

func Test_createAgents(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	bucket := agentBucket{
		Capability: dockerAmd64Cap,
		Labels:     agentLabelsFor(dockerAmd64Cap, nil),
	}

	t.Run("creates a new agent for the requested bucket", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_provider.NewMockProvider(t)
		autoscaler := Autoscaler{
			client:   client,
			provider: provider,
			agents:   map[string]*woodpecker.Agent{},
			config:   &config.Config{PoolID: "1"},
		}

		client.On("AgentCreate", mock.Anything).Return(&woodpecker.Agent{Name: "pool-1-agent-1"}, nil)
		// The capability passed to DeployAgent must be the bucket's, not zero.
		provider.On("DeployAgent", ctx, mock.Anything, dockerAmd64Cap).Return(nil)

		err := autoscaler.createAgents(ctx, bucket, 1)
		assert.NoError(t, err)
	})

	t.Run("reactivates a matching no-schedule agent before creating new ones", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_provider.NewMockProvider(t)
		autoscaler := Autoscaler{
			client:   client,
			provider: provider,
			agents: map[string]*woodpecker.Agent{
				"pool-1-agent-1": {
					ID:         1,
					Name:       "pool-1-agent-1",
					Platform:   "linux/amd64",
					Backend:    "docker",
					NoSchedule: true,
				},
			},
			config: &config.Config{PoolID: "1"},
		}

		client.On("AgentUpdate", mock.MatchedBy(func(agent *woodpecker.Agent) bool {
			return agent.ID == 1 && !agent.NoSchedule
		})).Return(nil, nil)
		client.On("AgentCreate", mock.Anything).Return(&woodpecker.Agent{Name: "pool-1-agent-1"}, nil)
		provider.On("DeployAgent", ctx, mock.Anything, dockerAmd64Cap).Return(nil)

		err := autoscaler.createAgents(ctx, bucket, 2)
		assert.NoError(t, err)
	})

	t.Run("does not reactivate an agent from a different bucket", func(t *testing.T) {
		// We have a no-schedule arm64 agent but we're scaling amd64 — the
		// arm64 agent must stay drained, and a fresh amd64 agent must be
		// deployed instead.
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_provider.NewMockProvider(t)
		autoscaler := Autoscaler{
			client:   client,
			provider: provider,
			agents: map[string]*woodpecker.Agent{
				"pool-1-agent-1": {
					ID:         1,
					Name:       "pool-1-agent-1",
					Platform:   "linux/arm64",
					Backend:    "docker",
					NoSchedule: true,
				},
			},
			config: &config.Config{PoolID: "1"},
		}
		client.On("AgentCreate", mock.Anything).Return(&woodpecker.Agent{Name: "pool-1-agent-2"}, nil)
		provider.On("DeployAgent", ctx, mock.Anything, dockerAmd64Cap).Return(nil)
		// AgentUpdate explicitly NOT expected — mock will fail the test
		// if it's called.

		err := autoscaler.createAgents(ctx, bucket, 1)
		assert.NoError(t, err)
	})
}

func Test_cleanupDanglingAgents(t *testing.T) {
	t.Run("should remove agent that is only present on woodpecker (not provider)", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_provider.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: map[string]*woodpecker.Agent{
				"pool-1-agent-1": {
					ID:         1,
					Name:       "pool-1-agent-1",
					Platform:   "linux/amd64",
					Backend:    "docker",
					NoSchedule: false,
				},
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
			agents: map[string]*woodpecker.Agent{
				"pool-1-agent-1": {
					ID:         1,
					Name:       "pool-1-agent-1",
					Platform:   "linux/amd64",
					Backend:    "docker",
					NoSchedule: false,
				},
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
			agents: map[string]*woodpecker.Agent{
				"active agent": {
					ID:          1,
					Name:        "active agent",
					NoSchedule:  false,
					Created:     time.Now().Add(-time.Minute * 20).Unix(),
					LastContact: time.Now().Add(-time.Minute * 5).Unix(),
				},
				"never contacted agent": {
					ID:          2,
					Name:        "never contacted agent",
					NoSchedule:  false,
					Created:     time.Now().Add(-time.Minute * 20).Unix(),
					LastContact: 0,
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
			agents: map[string]*woodpecker.Agent{
				"active agent": {
					ID:          1,
					Name:        "active agent",
					NoSchedule:  false,
					Created:     time.Now().Add(-time.Minute * 20).Unix(),
					LastContact: time.Now().Add(-time.Minute * 5).Unix(),
				},
				"stale agent": {
					ID:          2,
					Name:        "stale agent",
					NoSchedule:  false,
					Created:     time.Now().Add(-time.Minute * 20).Unix(),
					LastContact: time.Now().Add(-time.Minute * 20).Unix(),
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

		client.On("AgentTasksList", int64(1)).Return(nil, nil)

		idle, err := autoscaler.isAgentIdle(&woodpecker.Agent{
			ID:         1,
			Name:       "pool-1-agent-1",
			NoSchedule: false,
			LastWork:   time.Now().Add(-time.Minute * 20).Unix(),
		})
		assert.NoError(t, err)
		assert.True(t, idle)
	})
}

func Test_drainAgents(t *testing.T) {
	bucket := agentBucket{
		Capability: dockerAmd64Cap,
		Labels:     agentLabelsFor(dockerAmd64Cap, nil),
	}

	t.Run("drains matching idle agents only", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_provider.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: map[string]*woodpecker.Agent{
				"pool-1-agent-1": {ID: 1, Name: "pool-1-agent-1", Platform: "linux/amd64", Backend: "docker", LastContact: time.Now().Add(-time.Minute * 2).Unix()},
				"pool-1-agent-2": {ID: 2, Name: "pool-1-agent-2", Platform: "linux/amd64", Backend: "docker", NoSchedule: true, LastContact: time.Now().Add(-time.Minute * 2).Unix()},
				"pool-1-agent-3": {ID: 3, Name: "pool-1-agent-3", Platform: "linux/arm64", Backend: "docker", LastContact: time.Now().Add(-time.Minute * 2).Unix()},
				"pool-1-agent-4": {ID: 4, Name: "pool-1-agent-4", Platform: "linux/amd64", Backend: "docker", LastContact: time.Now().Add(-time.Minute * 2).Unix()},
			},
			provider: provider,
			client:   client,
			config: &config.Config{
				AgentIdleTimeout: time.Minute * 15,
			},
		}

		// Only IDs 1 and 4 should be drained: 2 is already drained, 3 is
		// the wrong bucket.
		client.On("AgentUpdate", mock.MatchedBy(func(agent *woodpecker.Agent) bool {
			return (agent.ID == 1 || agent.ID == 4) && agent.NoSchedule
		})).Return(nil, nil)

		err := autoscaler.drainAgents(ctx, bucket, 2)
		assert.NoError(t, err)
		assert.True(t, autoscaler.agents["pool-1-agent-1"].NoSchedule)
		assert.True(t, autoscaler.agents["pool-1-agent-4"].NoSchedule)
		assert.False(t, autoscaler.agents["pool-1-agent-3"].NoSchedule, "wrong-bucket agent must not be drained")
	})

	t.Run("does not drain an agent that never connected", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_provider.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: map[string]*woodpecker.Agent{
				"pool-1-agent-1": {ID: 1, Name: "pool-1-agent-1", Platform: "linux/amd64", Backend: "docker", LastContact: 0},
			},
			provider: provider,
			client:   client,
			config: &config.Config{
				AgentIdleTimeout: time.Minute * 15,
			},
		}

		err := autoscaler.drainAgents(ctx, bucket, 1)
		assert.NoError(t, err)
		assert.False(t, autoscaler.agents["pool-1-agent-1"].NoSchedule)
	})

	t.Run("does not drain an agent that has recently done work", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_provider.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: map[string]*woodpecker.Agent{
				"pool-1-agent-1": {
					ID:          1,
					Name:        "pool-1-agent-1",
					Platform:    "linux/amd64",
					Backend:     "docker",
					LastContact: time.Now().Add(-time.Minute * 2).Unix(),
					LastWork:    time.Now().Add(-time.Minute * 5).Unix(),
				},
			},
			provider: provider,
			client:   client,
			config: &config.Config{
				AgentIdleTimeout: time.Minute * 15,
			},
		}

		err := autoscaler.drainAgents(ctx, bucket, 1)
		assert.NoError(t, err)
		assert.False(t, autoscaler.agents["pool-1-agent-1"].NoSchedule)
	})
}

func Test_removeDrainedAgents(t *testing.T) {
	t.Run("should remove agent", func(t *testing.T) {
		ctx := t.Context()
		client := mocks_server.NewMockClient(t)
		provider := mocks_provider.NewMockProvider(t)
		autoscaler := Autoscaler{
			agents: map[string]*woodpecker.Agent{
				"pool-1-agent-1": {ID: 1, Name: "pool-1-agent-1", NoSchedule: false},
				"pool-1-agent-2": {ID: 2, Name: "pool-1-agent-2", NoSchedule: true},
				"pool-1-agent-3": {ID: 3, Name: "pool-1-agent-3", NoSchedule: false},
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
			agents: map[string]*woodpecker.Agent{
				"pool-1-agent-1": {ID: 1, Name: "pool-1-agent-1", NoSchedule: false},
				"pool-1-agent-2": {ID: 2, Name: "pool-1-agent-2", NoSchedule: true},
				"pool-1-agent-3": {ID: 3, Name: "pool-1-agent-3", NoSchedule: false},
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
