package e2e_test

import (
	"testing"
	"time"

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

// A pending workflow whose labels no provider capability can satisfy is
// unschedulable here: spinning up agents that still cannot run it wouldn't
// help, so the pool must stay put.
func TestAutoscalerIgnoresUnschedulablePending(t *testing.T) {
	h := newHarness(t, testConfig(0, 3), dockerAMD64)
	h.woodpecker.queue.Pending = []woodpecker.Task{
		realWorkflowTask("needs-arm", "linux/arm64"),
	}

	h.reconcile(t)
	require.Empty(t, h.provider.deployed, "no bucket can serve arm64; do not scale")
	require.Empty(t, h.woodpecker.agents)
}

// Agent startup can take longer than the reconciliation interval: agents
// deployed last cycle have not connected yet (and thus report no platform or
// backend) when the next reconcile runs. The still-pending demand must be
// attributed to those booting agents instead of provisioning another round
// every cycle until they come up.
func TestAutoscalerDoesNotOverprovisionWhileAgentsBoot(t *testing.T) {
	h := newHarness(t, testConfig(0, 10), dockerAMD64)
	h.woodpecker.queue.Pending = []woodpecker.Task{
		realWorkflowTask("build-1", "linux/amd64"),
		realWorkflowTask("build-2", "linux/amd64"),
	}

	t.Log("the first cycle provisions one agent per pending workflow")
	h.reconcile(t)
	require.Len(t, h.provider.deployed, 2)
	require.Len(t, h.woodpecker.agents, 2)

	t.Log("later cycles see the demand covered by the still-booting agents")
	h.reconcile(t)
	h.reconcile(t)
	require.Len(t, h.provider.deployed, 2, "booting agents already cover the demand")
	require.Len(t, h.woodpecker.agents, 2)

	t.Log("once connected the pool serves the work without further changes")
	h.connectAgents(t)
	h.reconcile(t)
	require.Len(t, h.provider.deployed, 2)
	require.Len(t, h.woodpecker.agents, 2)
}

// An agent whose boot exceeds the creation timeout must not hold its bucket
// slot forever: the planner stops counting it as capacity, provisions a
// replacement, and the stale-cleanup path reaps the stuck machine.
func TestAutoscalerReplacesAgentsStuckInBoot(t *testing.T) {
	cfg := testConfig(0, 10)
	cfg.AgentCreationTimeout = time.Minute
	h := newHarness(t, cfg, dockerAMD64)
	h.woodpecker.queue.Pending = []woodpecker.Task{
		realWorkflowTask("build-1", "linux/amd64"),
	}

	t.Log("the first cycle provisions one agent for the pending workflow")
	h.reconcile(t)
	require.Len(t, h.provider.deployed, 1)
	var stuck string
	for name := range h.provider.deployed {
		stuck = name
	}

	t.Log("within the creation timeout the booting agent covers the demand")
	h.reconcile(t)
	require.Len(t, h.provider.deployed, 1)

	t.Log("past the creation timeout the stuck agent is replaced and reaped")
	for _, agent := range h.woodpecker.agents {
		agent.Created = time.Now().Add(-2 * time.Minute).Unix()
	}
	h.reconcile(t)
	require.Len(t, h.provider.deployed, 1)
	require.Len(t, h.woodpecker.agents, 1)
	require.NotContains(t, h.provider.deployed, stuck, "the stuck boot must be torn down")
}

// A provisioned agent that registered but never phoned home, older than the
// inactivity timeout, is a boot that silently failed. It holds a provider slot
// forever unless reaped, so the stale-cleanup path must tear it down.
func TestAutoscalerReapsAgentsThatNeverConnect(t *testing.T) {
	cfg := testConfig(0, 2)
	cfg.AgentInactivityTimeout = time.Minute
	h := newHarness(t, cfg, dockerAMD64)

	name := "pool-e2e-agent-stuck"
	agent, err := h.woodpecker.AgentCreate(&woodpecker.Agent{Name: name})
	require.NoError(t, err)
	agent.Created = time.Now().Add(-2 * time.Minute).Unix()
	agent.LastContact = 0
	h.woodpecker.put(agent)
	h.provider.deployed[name] = dockerAMD64

	h.reconcile(t)
	require.Empty(t, h.provider.deployed, "the stuck agent is torn down on the provider")
	require.Empty(t, h.woodpecker.agents, "and deregistered on the server")
}

// Under hourly-round-up billing the paid hour is kept warm: an idle agent well
// inside its hour is not drained (per-second billing would tear it down), and
// only once it enters the teardown window is it drained and removed.
func TestAutoscalerHourlyBillingKeepsPaidAgentsWarm(t *testing.T) {
	cfg := testConfig(0, 1)
	cfg.BillingModel = types.BillingHourlyRoundUp
	cfg.ReconciliationInterval = time.Minute
	cfg.AgentBillingTeardownMargin = time.Minute // window ~2m: a fresh agent sits mid-hour, far from a boundary
	h := newHarness(t, cfg, dockerAMD64)
	h.addConnectedAgent(t, "pool-e2e-agent-paid", dockerAMD64)
	h.markIdle()

	t.Log("an idle agent inside its paid hour is kept schedulable, not drained")
	h.reconcile(t)
	require.Len(t, h.provider.deployed, 1, "hourly billing keeps the paid hour warm")
	require.Len(t, h.woodpecker.agents, 1)
	require.False(t, h.woodpecker.agentByName(t, "pool-e2e-agent-paid").NoSchedule)

	t.Log("once inside the teardown window the idle agent is drained and removed")
	cfg.AgentBillingTeardownMargin = time.Hour // window >= 1h: every moment counts as the teardown window
	h.reconcile(t)
	require.Empty(t, h.provider.deployed)
	require.Empty(t, h.woodpecker.agents)
}

// A drained agent already occupies a provider slot, but reactivating it does
// not consume another slot. Demand should therefore bring it back immediately
// even when the pool is at MaxAgents.
func TestAutoscalerReactivatesADrainedAgentAtCapacity(t *testing.T) {
	cfg := testConfig(0, 1)
	cfg.BillingModel = types.BillingHourlyRoundUp
	h := newHarness(t, cfg, dockerAMD64)
	h.addConnectedAgent(t, "pool-e2e-agent-drained", dockerAMD64)

	drained := h.woodpecker.agentByName(t, "pool-e2e-agent-drained")
	drained.NoSchedule = true
	h.woodpecker.put(drained)
	h.woodpecker.queue.Pending = []woodpecker.Task{
		realWorkflowTask("build-amd64", "linux/amd64"),
	}

	h.reconcile(t)
	require.False(t, h.woodpecker.agentByName(t, drained.Name).NoSchedule)
	require.Len(t, h.provider.deployed, 1, "reactivation must not deploy another agent")
}

// A drained agent that is still finishing a task must not be torn down: the
// removal path has to notice the in-flight work and keep the agent until it is
// truly idle.
func TestAutoscalerKeepsADrainingAgentThatStillRunsWork(t *testing.T) {
	h := newHarness(t, testConfig(0, 1), dockerAMD64)
	h.addConnectedAgent(t, "pool-e2e-agent-busy", dockerAMD64)

	agent := h.woodpecker.agentByName(t, "pool-e2e-agent-busy")
	agent.NoSchedule = true // already draining
	h.woodpecker.put(agent)
	h.woodpecker.queue.Running = []woodpecker.Task{
		runningOn(realWorkflowTask("build-amd64", "linux/amd64"), agent.ID),
	}

	t.Log("a draining agent with an in-flight task is left running")
	h.reconcile(t)
	require.Len(t, h.provider.deployed, 1, "must not remove an agent mid-task")
	require.Len(t, h.woodpecker.agents, 1)

	t.Log("once the task finishes the drained agent is removed")
	h.woodpecker.queue.Running = nil
	h.reconcile(t)
	require.Empty(t, h.provider.deployed)
	require.Empty(t, h.woodpecker.agents)
}

// Provider and server can disagree about which agents exist. The cleanup pass
// reconciles both directions: an agent only the provider knows about is torn
// down, and an agent only the server knows about is deregistered.
func TestAutoscalerReconcilesProviderServerDrift(t *testing.T) {
	h := newHarness(t, testConfig(0, 3), dockerAMD64)

	// Only on the provider: a leaked machine the server never registered.
	h.provider.deployed["pool-e2e-agent-ghost"] = dockerAMD64

	// Only on the server: a registration with no backing machine.
	orphan, err := h.woodpecker.AgentCreate(&woodpecker.Agent{Name: "pool-e2e-agent-orphan"})
	require.NoError(t, err)
	orphan.Platform = dockerAMD64.Platform
	orphan.Backend = string(dockerAMD64.Backend)
	orphan.LastContact = time.Now().Unix()
	orphan.LastWork = time.Now().Unix()
	h.woodpecker.put(orphan)

	h.reconcile(t)
	require.Empty(t, h.provider.deployed, "the provider-only ghost is torn down")
	require.Empty(t, h.woodpecker.agents, "the server-only orphan is deregistered")
}

// When the provider reports no capabilities (e.g. its API call failed) the
// planner has no safe basis to act: it must not scale for queued work and must
// not drain the agents already running, treating "unknown" as "hold".
func TestAutoscalerHoldsSteadyWithoutProviderCapabilities(t *testing.T) {
	h := newHarness(t, testConfig(1, 3)) // no capabilities offered
	h.addConnectedAgent(t, "pool-e2e-agent-standing", dockerAMD64)
	h.woodpecker.queue.Pending = []woodpecker.Task{
		realWorkflowTask("build-amd64", "linux/amd64"),
	}

	h.reconcile(t)
	require.Len(t, h.provider.deployed, 1, "unknown capabilities must not drain the fleet")
	require.Len(t, h.woodpecker.agents, 1, "and must not scale for demand it cannot place")
}
