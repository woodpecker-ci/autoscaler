package e2e_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

func testFailureRecovery(t *testing.T) {
	t.Run("failed reads preserve the fleet and can be retried", func(t *testing.T) {
		for _, operation := range []string{"agent list", "queue", "provider inventory"} {
			t.Run(operation, func(t *testing.T) {
				h := newHarness(t, testConfig(1, 1), dockerAMD64)
				agent := h.addConnectedAgent(t, "pool-e2e-agent-existing", dockerAMD64)
				fault := &h.woodpecker.listErr
				stage := "loading agents failed"
				switch operation {
				case "queue":
					fault, stage = &h.woodpecker.queueErr, "loading queue info failed"
				case "provider inventory":
					fault, stage = &h.provider.listErr, "cleaning up dangling agents failed"
				}
				failure := errors.New("service unavailable")
				*fault = failure
				err := h.autoscaler.Reconcile(t.Context())
				require.ErrorIs(t, err, failure)
				require.ErrorContains(t, err, stage)
				require.Equal(t, agent, h.woodpecker.agentByName(t, agent.Name))
				require.Contains(t, h.provider.deployed, agent.Name)

				t.Run("recovery", func(t *testing.T) {
					*fault = nil
					h.reconcile(t)
					require.Equal(t, agent, h.woodpecker.agentByName(t, agent.Name))
					require.Len(t, h.provider.deployed, 1)
				})
			})
		}
	})

	t.Run("registration failure leaves no machine and retries demand", func(t *testing.T) {
		h := newHarness(t, testConfig(0, 1), dockerAMD64)
		h.woodpecker.queue.Pending = []woodpecker.Task{realWorkflowTask("build", "linux/amd64")}
		failure := errors.New("registration failed")
		h.woodpecker.createErr = failure
		require.ErrorIs(t, h.autoscaler.Reconcile(t.Context()), failure)
		require.Empty(t, h.provider.deployed)
		require.Empty(t, h.woodpecker.agents)

		t.Run("recovery", func(t *testing.T) {
			h.woodpecker.createErr = nil
			h.reconcile(t)
			require.Len(t, h.provider.deployed, 1)
			require.Len(t, h.woodpecker.agents, 1)
		})
	})

	t.Run("failed deployment is deregistered before retrying at capacity", func(t *testing.T) {
		h := newHarness(t, testConfig(0, 1), dockerAMD64)
		h.woodpecker.queue.Pending = []woodpecker.Task{realWorkflowTask("build", "linux/amd64")}
		failure := errors.New("deployment failed")
		h.provider.deployErr = failure
		require.ErrorIs(t, h.autoscaler.Reconcile(t.Context()), failure)
		require.Empty(t, h.provider.deployed)
		require.Len(t, h.woodpecker.agents, 1)

		t.Run("recovery removes the orphan registration", func(t *testing.T) {
			h.provider.deployErr = nil
			h.reconcile(t)
			require.Empty(t, h.woodpecker.agents)
			require.Empty(t, h.provider.deployed)
			t.Run("next cycle deploys successfully", func(t *testing.T) {
				h.reconcile(t)
				require.Len(t, h.provider.deployed, 1)
				require.Len(t, h.woodpecker.agents, 1)
			})
		})
	})

	t.Run("failed scheduling updates preserve server state", func(t *testing.T) {
		for _, operation := range []string{"drain excess", "drain unavailable capability", "reactivate"} {
			t.Run(operation, func(t *testing.T) {
				h := newHarness(t, testConfig(0, 1), dockerAMD64)
				capability := dockerAMD64
				if operation == "drain unavailable capability" {
					capability = dockerARM64
				}
				agent := h.addConnectedAgent(t, "pool-e2e-agent-existing", capability)
				if operation == "reactivate" {
					agent.NoSchedule = true
					h.woodpecker.put(agent)
					h.woodpecker.queue.Pending = []woodpecker.Task{realWorkflowTask("build", "linux/amd64")}
				}
				failure := errors.New("update failed")
				h.woodpecker.updateErr = failure
				require.ErrorIs(t, h.autoscaler.Reconcile(t.Context()), failure)
				require.Equal(t, agent, h.woodpecker.agentByName(t, agent.Name))
				require.Contains(t, h.provider.deployed, agent.Name)

				t.Run("recovery", func(t *testing.T) {
					h.woodpecker.updateErr = nil
					h.reconcile(t)
					if operation == "reactivate" {
						require.False(t, h.woodpecker.agentByName(t, agent.Name).NoSchedule)
						require.Len(t, h.provider.deployed, 1)
					} else {
						require.Empty(t, h.provider.deployed)
						require.Empty(t, h.woodpecker.agents)
					}
				})
			})
		}
	})

	t.Run("failed teardown preserves enough state for retry", func(t *testing.T) {
		for _, state := range []string{"drained", "stale"} {
			t.Run(state, func(t *testing.T) {
				for _, operation := range []string{"task lookup", "provider removal", "server deletion"} {
					t.Run(operation, func(t *testing.T) {
						h := newHarness(t, testConfig(1, 1), dockerAMD64)
						agent := h.addConnectedAgent(t, "pool-e2e-agent-existing", dockerAMD64)
						if state == "drained" {
							// Avoid warm-pool reactivation while exercising drained cleanup.
							h.config.MinAgents = 0
							agent.NoSchedule = true
						} else {
							agent.LastContact = time.Now().Add(-2 * time.Hour).Unix()
						}
						h.woodpecker.put(agent)
						fault := &h.woodpecker.tasksErr
						switch operation {
						case "provider removal":
							fault = &h.provider.removeErr
						case "server deletion":
							fault = &h.woodpecker.deleteErr
						}
						failure := errors.New("teardown failed")
						*fault = failure
						err := h.autoscaler.Reconcile(t.Context())
						require.ErrorIs(t, err, failure)
						require.ErrorContains(t, err, state+" agents failed")
						require.Contains(t, h.woodpecker.agents, agent.ID)
						if operation == "server deletion" {
							require.Empty(t, h.provider.deployed)
						} else {
							require.Contains(t, h.provider.deployed, agent.Name)
						}

						t.Run("recovery", func(t *testing.T) {
							*fault = nil
							h.reconcile(t)
							require.Empty(t, h.provider.deployed)
							require.Empty(t, h.woodpecker.agents)
						})
					})
				}
			})
		}
	})

	t.Run("failed drift cleanup retries either source of truth", func(t *testing.T) {
		for _, side := range []string{"provider only", "server only"} {
			t.Run(side, func(t *testing.T) {
				h := newHarness(t, testConfig(0, 1), dockerAMD64)
				const name = "pool-e2e-agent-orphan"
				fault := &h.provider.removeErr
				if side == "provider only" {
					h.provider.deployed[name] = dockerAMD64
				} else {
					_, err := h.woodpecker.AgentCreate(&woodpecker.Agent{Name: name})
					require.NoError(t, err)
					fault = &h.woodpecker.deleteErr
				}
				failure := errors.New("cleanup failed")
				*fault = failure
				err := h.autoscaler.Reconcile(t.Context())
				require.ErrorIs(t, err, failure)
				require.ErrorContains(t, err, "cleaning up dangling agents failed")
				if side == "provider only" {
					require.Contains(t, h.provider.deployed, name)
				} else {
					require.Len(t, h.woodpecker.agents, 1)
				}

				t.Run("recovery", func(t *testing.T) {
					*fault = nil
					h.reconcile(t)
					require.Empty(t, h.provider.deployed)
					require.Empty(t, h.woodpecker.agents)
				})
			})
		}
	})
}
