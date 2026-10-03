package e2e_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// fault selects the injected error field of a fake that a test fails.
type fault func(h *harness) *error

var (
	agentListFault      fault = func(h *harness) *error { return &h.woodpecker.listErr }
	queueFault          fault = func(h *harness) *error { return &h.woodpecker.queueErr }
	agentTasksFault     fault = func(h *harness) *error { return &h.woodpecker.tasksErr }
	agentDeleteFault    fault = func(h *harness) *error { return &h.woodpecker.deleteErr }
	providerListFault   fault = func(h *harness) *error { return &h.provider.listErr }
	providerRemoveFault fault = func(h *harness) *error { return &h.provider.removeErr }
)

// TestFailedReadsPreserveFleet checks that a failing read aborts the cycle
// without touching the fleet and that the next cycle recovers.
func TestFailedReadsPreserveFleet(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		fault fault
		stage string
	}{
		{name: "agent list", fault: agentListFault, stage: "loading agents failed"},
		{name: "queue", fault: queueFault, stage: "loading queue info failed"},
		{name: "provider inventory", fault: providerListFault, stage: "cleaning up dangling agents failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t, testConfig(1, 1), dockerAMD64)
			agent := h.addConnectedAgent(t, "pool-e2e-agent-existing", dockerAMD64)
			failure := errors.New("service unavailable")
			*test.fault(h) = failure

			err := h.autoscaler.Reconcile(t.Context())
			require.ErrorIs(t, err, failure)
			require.ErrorContains(t, err, test.stage)
			require.Equal(t, agent, h.woodpecker.agentByName(t, agent.Name))
			require.Contains(t, h.provider.deployed, agent.Name)

			t.Run("recovery", func(t *testing.T) {
				*test.fault(h) = nil
				h.reconcile(t)
				require.Equal(t, agent, h.woodpecker.agentByName(t, agent.Name))
				require.Len(t, h.provider.deployed, 1)
			})
		})
	}
}

// TestRegistrationFailureRetriesDemand checks that a failed server
// registration deploys no machine and the demand is served next cycle.
func TestRegistrationFailureRetriesDemand(t *testing.T) {
	t.Parallel()

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
}

// TestFailedDeploymentIsDeregistered checks that a registration left behind by
// a failed deployment is removed before the slot is reused.
func TestFailedDeploymentIsDeregistered(t *testing.T) {
	t.Parallel()

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
}

// TestFailedSchedulingUpdatesPreserveServerState checks that a failing
// NoSchedule update leaves the agent unchanged and is retried.
func TestFailedSchedulingUpdatesPreserveServerState(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		capability types.Capability
		reactivate bool
	}{
		{name: "drain excess", capability: dockerAMD64},
		{name: "drain unavailable capability", capability: dockerARM64},
		{name: "reactivate", capability: dockerAMD64, reactivate: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t, testConfig(0, 1), dockerAMD64)
			agent := h.addConnectedAgent(t, "pool-e2e-agent-existing", test.capability)
			if test.reactivate {
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
				if test.reactivate {
					require.False(t, h.woodpecker.agentByName(t, agent.Name).NoSchedule)
					require.Len(t, h.provider.deployed, 1)
				} else {
					require.Empty(t, h.provider.deployed)
					require.Empty(t, h.woodpecker.agents)
				}
			})
		})
	}
}

// TestFailedTeardownCanBeRetried checks that every teardown step of a drained
// or stale agent can fail without losing the state needed to retry.
func TestFailedTeardownCanBeRetried(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		stale bool
		fault fault
		// removedFromProvider is true when the failing step runs after the
		// machine is already gone.
		removedFromProvider bool
	}{
		{name: "drained task lookup", fault: agentTasksFault},
		{name: "drained provider removal", fault: providerRemoveFault},
		{name: "drained server deletion", fault: agentDeleteFault, removedFromProvider: true},
		{name: "stale task lookup", stale: true, fault: agentTasksFault},
		{name: "stale provider removal", stale: true, fault: providerRemoveFault},
		{name: "stale server deletion", stale: true, fault: agentDeleteFault, removedFromProvider: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t, testConfig(1, 1), dockerAMD64)
			agent := h.addConnectedAgent(t, "pool-e2e-agent-existing", dockerAMD64)
			stage := "drained agents failed"
			if test.stale {
				stage = "stale agents failed"
				agent.LastContact = time.Now().Add(-2 * time.Hour).Unix()
			} else {
				// Avoid warm-pool reactivation while exercising drained cleanup.
				h.config.MinAgents = 0
				agent.NoSchedule = true
			}
			h.woodpecker.put(agent)
			failure := errors.New("teardown failed")
			*test.fault(h) = failure

			err := h.autoscaler.Reconcile(t.Context())
			require.ErrorIs(t, err, failure)
			require.ErrorContains(t, err, stage)
			require.Contains(t, h.woodpecker.agents, agent.ID)
			if test.removedFromProvider {
				require.Empty(t, h.provider.deployed)
			} else {
				require.Contains(t, h.provider.deployed, agent.Name)
			}

			t.Run("recovery", func(t *testing.T) {
				*test.fault(h) = nil
				h.reconcile(t)
				require.Empty(t, h.provider.deployed)
				require.Empty(t, h.woodpecker.agents)
			})
		})
	}
}

// TestFailedDriftCleanupCanBeRetried checks that an orphan on either side of
// provider and server survives a failed removal and is removed on retry.
func TestFailedDriftCleanupCanBeRetried(t *testing.T) {
	t.Parallel()

	const name = "pool-e2e-agent-orphan"
	for _, test := range []struct {
		name         string
		providerOnly bool
		fault        fault
	}{
		{name: "provider only", providerOnly: true, fault: providerRemoveFault},
		{name: "server only", fault: agentDeleteFault},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t, testConfig(0, 1), dockerAMD64)
			if test.providerOnly {
				h.provider.deployed[name] = dockerAMD64
			} else {
				_, err := h.woodpecker.AgentCreate(&woodpecker.Agent{Name: name})
				require.NoError(t, err)
			}
			failure := errors.New("cleanup failed")
			*test.fault(h) = failure

			err := h.autoscaler.Reconcile(t.Context())
			require.ErrorIs(t, err, failure)
			require.ErrorContains(t, err, "cleaning up dangling agents failed")
			if test.providerOnly {
				require.Contains(t, h.provider.deployed, name)
			} else {
				require.Len(t, h.woodpecker.agents, 1)
			}

			t.Run("recovery", func(t *testing.T) {
				*test.fault(h) = nil
				h.reconcile(t)
				require.Empty(t, h.provider.deployed)
				require.Empty(t, h.woodpecker.agents)
			})
		})
	}
}
