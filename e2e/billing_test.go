package e2e_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// TestHourlyBillingKeepsPaidCapacity checks that an idle hourly agent stays
// schedulable during its paid hour and is removed in the teardown window.
func TestHourlyBillingKeepsPaidCapacity(t *testing.T) {
	t.Parallel()

	h := newHarness(t, hourlyConfig(), dockerAMD64)
	agent := h.addConnectedAgent(t, "pool-e2e-agent-paid", dockerAMD64)
	h.markIdle()

	t.Run("inside the paid hour the idle agent stays schedulable", func(t *testing.T) {
		h.reconcile(t)
		require.Len(t, h.provider.deployed, 1)
		require.False(t, h.woodpecker.agentByName(t, agent.Name).NoSchedule)

		t.Run("inside the teardown window the idle agent is removed", func(t *testing.T) {
			current := h.woodpecker.agentByName(t, agent.Name)
			current.Created = time.Now().Add(-59 * time.Minute).Unix()
			h.woodpecker.put(current)
			h.reconcile(t)
			require.Empty(t, h.provider.deployed)
			require.Empty(t, h.woodpecker.agents)
		})
	})
}

// TestDrainedAgentInFreshPaidHourStaysWarm checks that a drained hourly agent
// that already entered a new paid hour is kept.
func TestDrainedAgentInFreshPaidHourStaysWarm(t *testing.T) {
	t.Parallel()

	h := newHarness(t, hourlyConfig(), dockerAMD64)
	agent := h.addConnectedAgent(t, "pool-e2e-agent-drained", dockerAMD64)
	agent.NoSchedule = true
	agent.Created = time.Now().Add(-61 * time.Minute).Unix()
	h.woodpecker.put(agent)

	h.reconcile(t)

	require.Len(t, h.provider.deployed, 1)
	require.Len(t, h.woodpecker.agents, 1)
}

// TestHourlyTeardownWindows checks that the teardown window repeats every hour
// and that unknown creation times are handled conservatively.
func TestHourlyTeardownWindows(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name                 string
		age                  time.Duration
		margin               time.Duration
		unknown, wantRemoved bool
	}{
		{name: "unknown creation time", unknown: true},
		{name: "future creation time", age: -time.Hour},
		{name: "middle of first hour", age: 30 * time.Minute},
		{name: "end of first hour", age: 59 * time.Minute, wantRemoved: true},
		{name: "start of second hour", age: 61 * time.Minute},
		{name: "end of second hour", age: 119 * time.Minute, wantRemoved: true},
		{name: "window covering an entire hour", age: 30 * time.Minute, margin: time.Hour, wantRemoved: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cfg := hourlyConfig()
			if test.margin != 0 {
				cfg.AgentBillingTeardownMargin = test.margin
			}
			h := newHarness(t, cfg, dockerAMD64)
			agent := h.addConnectedAgent(t, "pool-e2e-agent-paid", dockerAMD64)
			agent.Created = time.Now().Add(-test.age).Unix()
			if test.unknown {
				agent.Created = 0
			}
			// Hourly teardown depends on running work, not the per-second idle timeout.
			agent.LastWork = time.Now().Unix()
			h.woodpecker.put(agent)

			h.reconcile(t)

			if test.wantRemoved {
				require.Empty(t, h.provider.deployed)
				require.Empty(t, h.woodpecker.agents)
			} else {
				require.Contains(t, h.provider.deployed, agent.Name)
				require.False(t, h.woodpecker.agentByName(t, agent.Name).NoSchedule)
			}
		})
	}
}

// TestBusyHourlyAgentSurvivesMissedWindow checks that work running through a
// teardown window buys the agent the next paid hour.
func TestBusyHourlyAgentSurvivesMissedWindow(t *testing.T) {
	t.Parallel()

	h := newHarness(t, hourlyConfig(), dockerAMD64)
	agent := h.addConnectedAgent(t, "pool-e2e-agent-busy", dockerAMD64)
	agent.NoSchedule = true
	agent.Created = time.Now().Add(-59 * time.Minute).Unix()
	h.woodpecker.put(agent)
	h.woodpecker.queue.Running = []woodpecker.Task{runningOn(realWorkflowTask("build", "linux/amd64"), agent.ID)}
	h.reconcile(t)
	require.Contains(t, h.provider.deployed, agent.Name)

	t.Run("finished work retains the newly paid hour", func(t *testing.T) {
		h.woodpecker.queue.Running = nil
		agent.Created = time.Now().Add(-61 * time.Minute).Unix()
		h.woodpecker.put(agent)
		h.reconcile(t)
		require.Contains(t, h.provider.deployed, agent.Name)

		t.Run("the next teardown window removes it", func(t *testing.T) {
			agent.Created = time.Now().Add(-119 * time.Minute).Unix()
			h.woodpecker.put(agent)
			h.reconcile(t)
			require.Empty(t, h.provider.deployed)
			require.Empty(t, h.woodpecker.agents)
		})
	})
}

// TestPerSecondAgentsWaitForIdleTimeout checks that per-second agents are only
// removed after AgentIdleTimeout, whether or not they already drain.
func TestPerSecondAgentsWaitForIdleTimeout(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		draining bool
	}{
		{name: "schedulable"},
		{name: "already draining", draining: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t, testConfig(0, 1), dockerAMD64)
			agent := h.addConnectedAgent(t, "pool-e2e-agent-recent", dockerAMD64)
			agent.NoSchedule = test.draining
			agent.LastWork = time.Now().Unix()
			h.woodpecker.put(agent)
			h.reconcile(t)
			require.Equal(t, agent, h.woodpecker.agentByName(t, agent.Name))
			require.Contains(t, h.provider.deployed, agent.Name)

			t.Run("idle timeout permits removal", func(t *testing.T) {
				h.markIdle()
				h.reconcile(t)
				require.Empty(t, h.provider.deployed)
				require.Empty(t, h.woodpecker.agents)
			})
		})
	}
}
