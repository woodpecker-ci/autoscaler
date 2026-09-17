package e2e_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

func testAgentLifecycle(t *testing.T) {
	t.Run("boot recovery", testBootRecovery)
	t.Run("a workflow moves through provisioning, running, and teardown", func(t *testing.T) {
		h := newHarness(t, testConfig(0, 2), dockerAMD64, dockerARM64)
		h.woodpecker.queue.Pending = []woodpecker.Task{
			realWorkflowTask("build-amd64", "linux/amd64"),
			realWorkflowTask("build-arm64", "linux/arm64"),
		}

		t.Run("pending workflows provision matching agents", func(t *testing.T) {
			h.reconcile(t)
			require.ElementsMatch(t, []types.Capability{dockerAMD64, dockerARM64}, h.provider.deployedCapabilities())
			require.Len(t, h.woodpecker.agents, 2)

			t.Run("another cycle does not duplicate booting agents", func(t *testing.T) {
				h.reconcile(t)
				require.Len(t, h.provider.deployed, 2)
				require.Len(t, h.woodpecker.agents, 2)

				t.Run("connected agents keep running workflows alive", func(t *testing.T) {
					h.connectAgents(t)
					h.woodpecker.queue.Pending = nil
					h.woodpecker.queue.Running = []woodpecker.Task{
						runningOn(realWorkflowTask("build-amd64", "linux/amd64"), h.agentIDForPlatform(t, "linux/amd64")),
						runningOn(realWorkflowTask("build-arm64", "linux/arm64"), h.agentIDForPlatform(t, "linux/arm64")),
					}
					h.reconcile(t)
					require.Len(t, h.provider.deployed, 2)

					t.Run("finished workflows let idle agents drain and disappear", func(t *testing.T) {
						h.woodpecker.queue.Running = nil
						h.markIdle()
						h.reconcile(t)
						require.Empty(t, h.provider.deployed)
						require.Empty(t, h.woodpecker.agents)
					})
				})
			})
		})
	})

	t.Run("a full pool replaces an idle agent with the needed capability", func(t *testing.T) {
		h := newHarness(t, testConfig(1, 1), dockerAMD64, dockerARM64)
		h.addConnectedAgent(t, "pool-e2e-agent-existing", dockerAMD64)
		h.woodpecker.queue.Pending = []woodpecker.Task{
			realWorkflowTask("build-arm64", "linux/arm64"),
		}

		t.Run("first cycle drains the wrong capability", func(t *testing.T) {
			h.reconcile(t)
			require.Empty(t, h.provider.deployed)
			require.Empty(t, h.woodpecker.agents)

			t.Run("second cycle fills the freed slot", func(t *testing.T) {
				h.reconcile(t)
				require.Equal(t, []types.Capability{dockerARM64}, h.provider.deployedCapabilities())
				require.Len(t, h.woodpecker.agents, 1)
			})
		})
	})

	t.Run("an agent with stale custom labels is replaced", func(t *testing.T) {
		cfg := testConfig(0, 1)
		cfg.ExtraAgentLabels = map[string]string{"region": "us"}
		h := newHarness(t, cfg, dockerAMD64)
		h.addConnectedAgent(t, "pool-e2e-agent-old-labels", dockerAMD64)

		old := h.woodpecker.agentByName(t, "pool-e2e-agent-old-labels")
		old.CustomLabels = map[string]string{"region": "eu"}
		h.woodpecker.put(old)
		task := realWorkflowTask("build-us", "linux/amd64")
		task.Labels["region"] = "us"
		h.woodpecker.queue.Pending = []woodpecker.Task{task}

		t.Run("first cycle retires the stale agent", func(t *testing.T) {
			h.reconcile(t)
			require.Empty(t, h.provider.deployed)

			t.Run("second cycle provisions the replacement", func(t *testing.T) {
				h.reconcile(t)
				require.Len(t, h.provider.deployed, 1)
				require.NotContains(t, h.provider.deployed, "pool-e2e-agent-old-labels")
			})
		})
	})

	t.Run("booting agents cover demand until they connect", func(t *testing.T) {
		h := newHarness(t, testConfig(0, 10), dockerAMD64)
		h.woodpecker.queue.Pending = []woodpecker.Task{
			realWorkflowTask("build-1", "linux/amd64"),
			realWorkflowTask("build-2", "linux/amd64"),
		}

		t.Run("first cycle provisions capacity", func(t *testing.T) {
			h.reconcile(t)
			require.Len(t, h.provider.deployed, 2)

			t.Run("later cycles do not overprovision", func(t *testing.T) {
				h.reconcile(t)
				h.reconcile(t)
				require.Len(t, h.provider.deployed, 2)
				require.Len(t, h.woodpecker.agents, 2)

				t.Run("connected agents keep the fleet stable", func(t *testing.T) {
					h.connectAgents(t)
					h.reconcile(t)
					require.Len(t, h.provider.deployed, 2)
					require.Len(t, h.woodpecker.agents, 2)
				})
			})
		})
	})

	t.Run("a restart still bounds unattributed booting agents by MaxAgents", func(t *testing.T) {
		cfg := testConfig(0, 2)
		h := newHarness(t, cfg, dockerAMD64)
		h.woodpecker.queue.Pending = []woodpecker.Task{
			realWorkflowTask("build-1", "linux/amd64"),
			realWorkflowTask("build-2", "linux/amd64"),
		}
		h.reconcile(t)
		require.Len(t, h.provider.deployed, 2)

		restarted, err := engine.NewAutoscaler(t.Context(), h.provider, h.woodpecker, cfg)
		require.NoError(t, err)
		h.autoscaler = restarted
		h.reconcile(t)

		require.Len(t, h.provider.deployed, 2)
		require.Len(t, h.woodpecker.agents, 2)
	})

	t.Run("an agent stuck in boot is replaced after its creation timeout", func(t *testing.T) {
		cfg := testConfig(0, 10)
		cfg.AgentCreationTimeout = time.Minute
		h := newHarness(t, cfg, dockerAMD64)
		h.woodpecker.queue.Pending = []woodpecker.Task{
			realWorkflowTask("build", "linux/amd64"),
		}

		h.reconcile(t)
		require.Len(t, h.provider.deployed, 1)
		var stuck string
		for name := range h.provider.deployed {
			stuck = name
		}

		t.Run("inside the timeout the booting agent keeps its slot", func(t *testing.T) {
			h.reconcile(t)
			require.Len(t, h.provider.deployed, 1)

			t.Run("past the timeout the stuck agent is reaped and replaced", func(t *testing.T) {
				for _, agent := range h.woodpecker.agents {
					agent.Created = time.Now().Add(-2 * time.Minute).Unix()
				}
				h.reconcile(t)
				require.Len(t, h.provider.deployed, 1)
				require.Len(t, h.woodpecker.agents, 1)
				require.NotContains(t, h.provider.deployed, stuck)
			})
		})
	})

	t.Run("matching demand reactivates a drained agent at capacity", func(t *testing.T) {
		cfg := testConfig(0, 1)
		cfg.BillingModel = types.BillingHourlyRoundUp
		h := newHarness(t, cfg, dockerAMD64)
		h.addConnectedAgent(t, "pool-e2e-agent-drained", dockerAMD64)

		drained := h.woodpecker.agentByName(t, "pool-e2e-agent-drained")
		drained.NoSchedule = true
		h.woodpecker.put(drained)
		h.woodpecker.queue.Pending = []woodpecker.Task{
			realWorkflowTask("build", "linux/amd64"),
		}

		h.reconcile(t)

		require.False(t, h.woodpecker.agentByName(t, drained.Name).NoSchedule)
		require.Len(t, h.provider.deployed, 1)
	})

	t.Run("a draining agent stays until its running work finishes", func(t *testing.T) {
		h := newHarness(t, testConfig(0, 1), dockerAMD64)
		h.addConnectedAgent(t, "pool-e2e-agent-busy", dockerAMD64)

		agent := h.woodpecker.agentByName(t, "pool-e2e-agent-busy")
		agent.NoSchedule = true
		h.woodpecker.put(agent)
		h.woodpecker.queue.Running = []woodpecker.Task{
			runningOn(realWorkflowTask("build", "linux/amd64"), agent.ID),
		}

		t.Run("running work keeps the agent", func(t *testing.T) {
			h.reconcile(t)
			require.Len(t, h.provider.deployed, 1)
			require.Len(t, h.woodpecker.agents, 1)

			t.Run("finishing the work allows removal", func(t *testing.T) {
				h.woodpecker.queue.Running = nil
				h.reconcile(t)
				require.Empty(t, h.provider.deployed)
				require.Empty(t, h.woodpecker.agents)
			})
		})
	})
}

func testBootRecovery(t *testing.T) {
	t.Run("expired boot at MaxAgents frees a slot before replacement", func(t *testing.T) {
		cfg := testConfig(0, 1)
		cfg.AgentCreationTimeout = time.Minute
		h := newHarness(t, cfg, dockerAMD64)
		h.woodpecker.queue.Pending = []woodpecker.Task{realWorkflowTask("build", "linux/amd64")}
		h.reconcile(t)
		require.Len(t, h.provider.deployed, 1)
		var originalID int64
		for _, agent := range h.woodpecker.agents {
			originalID = agent.ID
			agent.Created = time.Now().Add(-2 * time.Minute).Unix()
		}
		h.reconcile(t)
		require.Empty(t, h.provider.deployed)
		require.Empty(t, h.woodpecker.agents)
		t.Run("next cycle replaces the expired registration", func(t *testing.T) {
			h.reconcile(t)
			require.Len(t, h.provider.deployed, 1)
			require.Len(t, h.woodpecker.agents, 1)
			require.NotContains(t, h.woodpecker.agents, originalID)
		})
	})

	t.Run("a vanished boot registration no longer covers demand", func(t *testing.T) {
		h := newHarness(t, testConfig(0, 1), dockerAMD64)
		h.woodpecker.queue.Pending = []woodpecker.Task{realWorkflowTask("build", "linux/amd64")}
		h.reconcile(t)
		for id := range h.woodpecker.agents {
			require.NoError(t, h.woodpecker.AgentDelete(id))
		}
		h.reconcile(t)
		require.Len(t, h.provider.deployed, 1)
		require.Len(t, h.woodpecker.agents, 1)
		for _, agent := range h.woodpecker.agents {
			require.Greater(t, agent.ID, int64(1))
			require.Contains(t, h.provider.deployed, agent.Name)
		}
	})

	t.Run("connected custom labels keep the provisioned agent usable", func(t *testing.T) {
		cfg := testConfig(0, 2)
		cfg.ExtraAgentLabels = map[string]string{"!region": "eu"}
		h := newHarness(t, cfg, dockerAMD64)
		task := realWorkflowTask("build", "linux/amd64")
		task.Labels["region"] = "eu"
		h.woodpecker.queue.Pending = []woodpecker.Task{task}
		h.reconcile(t)
		require.Len(t, h.provider.deployed, 1)
		h.reconcile(t)
		require.Len(t, h.provider.deployed, 1)
		t.Run("reported labels take over from boot tracking", func(t *testing.T) {
			h.connectAgents(t)
			h.markIdle()
			h.reconcile(t)
			require.Len(t, h.provider.deployed, 1)
			for _, agent := range h.woodpecker.agents {
				require.Equal(t, int64(1), agent.ID)
				require.Equal(t, cfg.ExtraAgentLabels, agent.CustomLabels)
				require.False(t, agent.NoSchedule)
			}
		})
	})
}
