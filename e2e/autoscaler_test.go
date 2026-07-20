package e2e_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var (
	dockerAMD64 = types.Capability{Platform: "linux/amd64", Backend: types.BackendDocker}
	dockerARM64 = types.Capability{Platform: "linux/arm64", Backend: types.BackendDocker}
)

// A full workflow lifecycle: real queue labels route to per-platform agents,
// reconciling during provisioning respects MaxAgents, running work is tracked
// by agent, and idle agents are drained and removed.
func TestAutoscalerHandlesAWorkflowLifecycle(t *testing.T) {
	h := newHarness(t, testConfig(0, 2), dockerAMD64, dockerARM64)
	h.woodpecker.queue.Pending = []woodpecker.Task{
		realWorkflowTask("build-amd64", "linux/amd64"),
		realWorkflowTask("build-arm64", "linux/arm64"),
	}

	t.Log("pending workflows create one agent per required platform")
	h.reconcile(t)
	require.ElementsMatch(t, []types.Capability{dockerAMD64, dockerARM64}, h.provider.deployedCapabilities())
	require.Len(t, h.woodpecker.agents, 2)

	t.Log("reconciling again during provisioning stays within MaxAgents")
	h.reconcile(t)
	require.Len(t, h.provider.deployed, 2)
	require.Len(t, h.woodpecker.agents, 2)

	t.Log("agents connect and each workflow runs on its matching agent")
	h.connectAgents(t)
	h.woodpecker.queue.Pending = nil
	h.woodpecker.queue.Running = []woodpecker.Task{
		runningOn(realWorkflowTask("build-amd64", "linux/amd64"), h.agentIDForPlatform(t, "linux/amd64")),
		runningOn(realWorkflowTask("build-arm64", "linux/arm64"), h.agentIDForPlatform(t, "linux/arm64")),
	}
	h.reconcile(t)
	require.Len(t, h.provider.deployed, 2, "busy agents are kept")

	t.Log("finished workflows leave idle agents that are drained and removed")
	h.woodpecker.queue.Running = nil
	h.markIdle()
	h.reconcile(t)
	require.Empty(t, h.provider.deployed)
	require.Empty(t, h.woodpecker.agents)
}

// A fixed-size pool holding the wrong capability drains it before it can be
// replaced with the one that is actually needed.
func TestAutoscalerReplacesAnIdleAgentWithTheRequiredCapability(t *testing.T) {
	h := newHarness(t, testConfig(1, 1), dockerAMD64, dockerARM64)
	h.addConnectedAgent(t, "pool-e2e-agent-existing", dockerAMD64)
	h.woodpecker.queue.Pending = []woodpecker.Task{
		realWorkflowTask("build-arm64", "linux/arm64"),
	}

	t.Log("the full pool first drains the idle wrong-capability agent")
	h.reconcile(t)
	require.Empty(t, h.provider.deployed)
	require.Empty(t, h.woodpecker.agents)

	t.Log("the freed slot is then filled with the required capability")
	h.reconcile(t)
	require.Equal(t, []types.Capability{dockerARM64}, h.provider.deployedCapabilities())
	require.Len(t, h.woodpecker.agents, 1)
}

// MinAgents keeps a warm agent without any queue demand, and reconciling again
// does not pile on more.
func TestAutoscalerMaintainsAWarmPool(t *testing.T) {
	h := newHarness(t, testConfig(1, 3), dockerAMD64)

	t.Log("an empty queue still provisions the MinAgents warm pool")
	h.reconcile(t)
	require.Equal(t, []types.Capability{dockerAMD64}, h.provider.deployedCapabilities())

	t.Log("the warm agent connects and the pool holds steady")
	h.connectAgents(t)
	h.markIdle()
	h.reconcile(t)
	require.Len(t, h.provider.deployed, 1)
	require.Len(t, h.woodpecker.agents, 1)
}

// Work running on agents outside this pool (e.g. static admin agents) is not
// this pool's demand and must not trigger scale-up.
func TestAutoscalerIgnoresWorkOutsideThePool(t *testing.T) {
	h := newHarness(t, testConfig(0, 3), dockerAMD64)
	h.woodpecker.queue.Running = []woodpecker.Task{
		runningOn(realWorkflowTask("external", "linux/amd64"), 999),
	}

	h.reconcile(t)
	require.Empty(t, h.provider.deployed, "work on a non-pool agent must not scale the pool")
	require.Empty(t, h.woodpecker.agents)
}

// An idle agent whose capability the provider no longer offers is retired so it
// stops holding a slot.
func TestAutoscalerRetiresAgentsWithUnavailableCapabilities(t *testing.T) {
	h := newHarness(t, testConfig(0, 2), dockerAMD64)
	h.addConnectedAgent(t, "pool-e2e-agent-drifted", dockerARM64)

	h.reconcile(t)
	require.Empty(t, h.provider.deployed, "the drifted agent is drained and removed")
	require.Empty(t, h.woodpecker.agents)
}

// WorkflowsPerAgent lets one agent take several queued workflows, so the pool
// scales to ceil(load/WPA) agents rather than one per task.
func TestAutoscalerPacksWorkflowsPerAgent(t *testing.T) {
	cfg := testConfig(0, 5)
	cfg.WorkflowsPerAgent = 2
	h := newHarness(t, cfg, dockerAMD64)
	h.woodpecker.queue.Pending = []woodpecker.Task{
		realWorkflowTask("build-1", "linux/amd64"),
		realWorkflowTask("build-2", "linux/amd64"),
		realWorkflowTask("build-3", "linux/amd64"),
	}

	t.Log("three pending workflows at two-per-agent provision two agents, not three")
	h.reconcile(t)
	require.Len(t, h.provider.deployed, 2)
	require.Len(t, h.woodpecker.agents, 2)
}
