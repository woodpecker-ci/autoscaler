package e2e_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// TestMaxAgentsCapsScaleUp checks that demand beyond MaxAgents is not served.
func TestMaxAgentsCapsScaleUp(t *testing.T) {
	t.Parallel()

	h := newHarness(t, testConfig(0, 2), dockerAMD64)
	h.woodpecker.queue.Pending = []woodpecker.Task{
		realWorkflowTask("build-1", "linux/amd64"),
		realWorkflowTask("build-2", "linux/amd64"),
		realWorkflowTask("build-3", "linux/amd64"),
	}

	h.reconcile(t)

	require.Len(t, h.provider.deployed, 2)
}

// TestLimitedCapacityGoesToBusiestBucket checks that a scarce budget is spent
// on the capability with the most pending work.
func TestLimitedCapacityGoesToBusiestBucket(t *testing.T) {
	t.Parallel()

	h := newHarness(t, testConfig(0, 1), dockerAMD64, dockerARM64)
	h.woodpecker.queue.Pending = []woodpecker.Task{
		realWorkflowTask("amd64", "linux/amd64"),
		realWorkflowTask("arm64-1", "linux/arm64"),
		realWorkflowTask("arm64-2", "linux/arm64"),
	}

	h.reconcile(t)

	require.Equal(t, []types.Capability{dockerARM64}, h.provider.deployedCapabilities())
}

// TestEqualDemandGoesToFirstCapability checks the tie-break when buckets
// compete for a scarce budget with the same demand.
func TestEqualDemandGoesToFirstCapability(t *testing.T) {
	t.Parallel()

	h := newHarness(t, testConfig(0, 1), dockerARM64, dockerAMD64)
	h.woodpecker.queue.Pending = []woodpecker.Task{realWorkflowTask("amd", "linux/amd64"), realWorkflowTask("arm", "linux/arm64")}

	h.reconcile(t)

	require.Equal(t, []types.Capability{dockerARM64}, h.provider.deployedCapabilities())
}

// TestWorkflowsPerAgentPacksWorkflows checks that pending demand is divided by
// WorkflowsPerAgent, rounding up, and that invalid values fall back to one.
func TestWorkflowsPerAgentPacksWorkflows(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name                                 string
		workflowsPerAgent, tasks, wantAgents int
	}{
		{name: "exact multiple", workflowsPerAgent: 2, tasks: 4, wantAgents: 2},
		{name: "partial final agent", workflowsPerAgent: 2, tasks: 5, wantAgents: 3},
		{name: "zero falls back to one", tasks: 2, wantAgents: 2},
		{name: "negative falls back to one", workflowsPerAgent: -1, tasks: 2, wantAgents: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cfg := testConfig(0, 5)
			cfg.WorkflowsPerAgent = test.workflowsPerAgent
			h := newHarness(t, cfg, dockerAMD64)
			for i := range test.tasks {
				h.woodpecker.queue.Pending = append(h.woodpecker.queue.Pending, realWorkflowTask(fmt.Sprint(i), "linux/amd64"))
			}

			h.reconcile(t)

			require.Len(t, h.provider.deployed, test.wantAgents)
		})
	}
}

// TestRunningWorkPlusBacklogAddsCapacity checks that pending work on top of a
// busy agent provisions an additional agent.
func TestRunningWorkPlusBacklogAddsCapacity(t *testing.T) {
	t.Parallel()

	h := newHarness(t, testConfig(0, 3), dockerAMD64)
	agent := h.addConnectedAgent(t, "pool-e2e-agent-busy", dockerAMD64)
	h.woodpecker.queue.Pending = []woodpecker.Task{
		realWorkflowTask("pending", "linux/amd64"),
	}
	h.woodpecker.queue.Running = []woodpecker.Task{
		runningOn(realWorkflowTask("running", "linux/amd64"), agent.ID),
	}

	h.reconcile(t)

	require.Len(t, h.provider.deployed, 2)
	require.Len(t, h.woodpecker.agents, 2)
}

// TestMinAgentsKeepsWarmPool checks that an empty queue still provisions
// MinAgents in the first capability and keeps them across cycles.
func TestMinAgentsKeepsWarmPool(t *testing.T) {
	t.Parallel()

	h := newHarness(t, testConfig(1, 3), dockerARM64, dockerAMD64)

	t.Run("empty queue provisions the warm agent", func(t *testing.T) {
		h.reconcile(t)
		require.Equal(t, []types.Capability{dockerARM64}, h.provider.deployedCapabilities())

		t.Run("later reconciliations hold the warm pool steady", func(t *testing.T) {
			h.connectAgents(t)
			h.markIdle()
			h.reconcile(t)
			require.Len(t, h.provider.deployed, 1)
			require.Len(t, h.woodpecker.agents, 1)
		})
	})
}

// TestMinAgentsWarmCapacityGoesToBusiestCapability checks that warm capacity
// above demand joins the bucket that already has work.
func TestMinAgentsWarmCapacityGoesToBusiestCapability(t *testing.T) {
	t.Parallel()

	h := newHarness(t, testConfig(3, 5), dockerAMD64, dockerARM64)
	h.woodpecker.queue.Pending = []woodpecker.Task{realWorkflowTask("arm", "linux/arm64")}

	h.reconcile(t)

	require.ElementsMatch(t, []types.Capability{dockerARM64, dockerARM64, dockerARM64}, h.provider.deployedCapabilities())
}

// TestScaleDownStopsAtMinAgents checks that idle agents are removed only down
// to MinAgents and the survivor stays schedulable.
func TestScaleDownStopsAtMinAgents(t *testing.T) {
	t.Parallel()

	h := newHarness(t, testConfig(1, 3), dockerAMD64)
	for i := range 3 {
		h.addConnectedAgent(t, fmt.Sprintf("pool-e2e-agent-%d", i), dockerAMD64)
	}

	h.reconcile(t)

	require.Len(t, h.provider.deployed, 1)
	require.Len(t, h.woodpecker.agents, 1)
	for _, agent := range h.woodpecker.agents {
		require.False(t, agent.NoSchedule)
	}
}

// TestBusyDrainingAgentOccupiesSlot checks that a draining agent with running
// work still counts against MaxAgents.
func TestBusyDrainingAgentOccupiesSlot(t *testing.T) {
	t.Parallel()

	h := newHarness(t, testConfig(0, 1), dockerAMD64, dockerARM64)
	agent := h.addConnectedAgent(t, "pool-e2e-agent-draining", dockerAMD64)
	agent.NoSchedule = true
	h.woodpecker.put(agent)
	h.woodpecker.queue.Running = []woodpecker.Task{runningOn(realWorkflowTask("busy", "linux/amd64"), agent.ID)}
	h.woodpecker.queue.Pending = []woodpecker.Task{realWorkflowTask("pending", "linux/arm64")}

	h.reconcile(t)

	require.Equal(t, []types.Capability{dockerAMD64}, h.provider.deployedCapabilities())
	require.True(t, h.woodpecker.agentByName(t, agent.Name).NoSchedule)
}

// TestExternalRunningWorkCreatesNoDemand checks that work running on agents
// outside this pool does not provision agents.
func TestExternalRunningWorkCreatesNoDemand(t *testing.T) {
	t.Parallel()

	h := newHarness(t, testConfig(0, 3), dockerAMD64)
	h.woodpecker.queue.Running = []woodpecker.Task{
		runningOn(realWorkflowTask("external", "linux/amd64"), 999),
	}

	h.reconcile(t)

	require.Empty(t, h.provider.deployed)
	require.Empty(t, h.woodpecker.agents)
}
