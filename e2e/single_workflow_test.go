package e2e_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// TestSingleWorkflowAgentIsReplacedAfterItsWorkflow checks that a one-shot
// agent is torn down once it ran its workflow, and that the next workflow gets
// a fresh agent even when the pool is full.
func TestSingleWorkflowAgentIsReplacedAfterItsWorkflow(t *testing.T) {
	t.Parallel()

	cfg := testConfig(0, 1)
	cfg.AgentSingleWorkflow = true
	h := newHarness(t, cfg, dockerAMD64)
	h.woodpecker.queue.Pending = []woodpecker.Task{realWorkflowTask("build-1", "linux/amd64")}
	h.reconcile(t)
	require.Len(t, h.provider.deployed, 1)
	var first string
	for name := range h.provider.deployed {
		first = name
	}

	t.Run("running workflow keeps the agent", func(t *testing.T) {
		h.connectAgents(t)
		agent := h.woodpecker.agentByName(t, first)
		h.woodpecker.queue.Pending = nil
		h.woodpecker.queue.Running = []woodpecker.Task{runningOn(realWorkflowTask("build-1", "linux/amd64"), agent.ID)}
		h.reconcile(t)
		require.Contains(t, h.provider.deployed, first)

		t.Run("spent agent is replaced for the next workflow", func(t *testing.T) {
			h.woodpecker.queue.Running = nil
			h.woodpecker.queue.Pending = []woodpecker.Task{realWorkflowTask("build-2", "linux/amd64")}
			agent.LastWork = time.Now().Unix()
			h.woodpecker.put(agent)
			h.reconcile(t)
			require.Len(t, h.provider.deployed, 1)
			require.NotContains(t, h.provider.deployed, first)
			require.Len(t, h.woodpecker.agents, 1)
		})
	})
}

// TestSingleWorkflowAgentWaitsForItsWorkflow checks that a connected one-shot
// agent that has not worked yet stays available as capacity.
func TestSingleWorkflowAgentWaitsForItsWorkflow(t *testing.T) {
	t.Parallel()

	cfg := testConfig(1, 1)
	cfg.AgentSingleWorkflow = true
	h := newHarness(t, cfg, dockerAMD64)
	agent := h.addConnectedAgent(t, "pool-e2e-agent-fresh", dockerAMD64)
	agent.LastWork = 0
	h.woodpecker.put(agent)

	h.reconcile(t)

	require.Contains(t, h.provider.deployed, agent.Name)
	require.Len(t, h.woodpecker.agents, 1)
}
