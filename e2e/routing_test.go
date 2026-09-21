package e2e_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// TestRealQueueLabelsRouteToMatchingPlatforms checks that tasks carrying the
// labels the server stamps on real workflows provision agents per platform.
func TestRealQueueLabelsRouteToMatchingPlatforms(t *testing.T) {
	h := newHarness(t, testConfig(0, 2), dockerAMD64, dockerARM64)
	amd64 := realWorkflowTask("build-amd64", "linux/amd64")
	amd64.Labels["empty-value-is-ignored"] = ""
	arm64 := realWorkflowTask("build-arm64", "linux/arm64")
	h.woodpecker.queue.Pending = []woodpecker.Task{amd64, arm64}

	h.reconcile(t)

	require.ElementsMatch(t, []types.Capability{dockerAMD64, dockerARM64}, h.provider.deployedCapabilities())
}

// TestTaskWithoutPlatformUsesFirstCapability checks that an unconstrained
// task is served by the first capability the provider advertises.
func TestTaskWithoutPlatformUsesFirstCapability(t *testing.T) {
	h := newHarness(t, testConfig(0, 1), dockerARM64, dockerAMD64)
	h.woodpecker.queue.Pending = []woodpecker.Task{
		realWorkflowTask("unconstrained", ""),
	}

	h.reconcile(t)

	require.Equal(t, []types.Capability{dockerARM64}, h.provider.deployedCapabilities())
}

// TestMandatoryLabelsRequireExactValue checks that a "!" agent label is only
// satisfied by a workflow setting exactly that value.
func TestMandatoryLabelsRequireExactValue(t *testing.T) {
	for _, test := range []struct {
		name       string
		region     string
		wantAgents int
	}{
		{name: "missing", wantAgents: 0},
		{name: "different", region: "us", wantAgents: 0},
		{name: "matching", region: "eu", wantAgents: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := testConfig(0, 1)
			cfg.ExtraAgentLabels = map[string]string{"!region": "eu"}
			h := newHarness(t, cfg, dockerAMD64)
			task := realWorkflowTask("build", "linux/amd64")
			if test.region != "" {
				task.Labels["region"] = test.region
			}
			h.woodpecker.queue.Pending = []woodpecker.Task{task}

			h.reconcile(t)

			require.Len(t, h.provider.deployed, test.wantAgents)
		})
	}
}

// TestWildcardExtraLabelAcceptsAnyValue checks that a "*" agent label matches
// whatever value a workflow requests.
func TestWildcardExtraLabelAcceptsAnyValue(t *testing.T) {
	cfg := testConfig(0, 1)
	cfg.ExtraAgentLabels = map[string]string{"region": "*"}
	h := newHarness(t, cfg, dockerAMD64)
	task := realWorkflowTask("build", "linux/amd64")
	task.Labels["region"] = "eu"
	h.woodpecker.queue.Pending = []woodpecker.Task{task}

	h.reconcile(t)

	require.Equal(t, []types.Capability{dockerAMD64}, h.provider.deployedCapabilities())
}

// TestUnschedulablePendingCreatesNoAgent checks that work no capability can
// serve does not provision an agent that could never pick it up.
func TestUnschedulablePendingCreatesNoAgent(t *testing.T) {
	h := newHarness(t, testConfig(0, 3), dockerAMD64)
	h.woodpecker.queue.Pending = []woodpecker.Task{
		realWorkflowTask("needs-arm", "linux/arm64"),
	}

	h.reconcile(t)

	require.Empty(t, h.provider.deployed)
	require.Empty(t, h.woodpecker.agents)
}

// TestLabelMatching mirrors the server's label matching between workflow
// labels and the extra labels the autoscaler gives its agents.
func TestLabelMatching(t *testing.T) {
	for _, test := range []struct {
		name        string
		agentLabels map[string]string
		taskLabels  map[string]string
		wantAgents  int
	}{
		{name: "unknown label", taskLabels: map[string]string{"gpu": "yes"}},
		{name: "wrong backend", taskLabels: map[string]string{"backend": "local"}},
		{name: "different normal label", agentLabels: map[string]string{"region": "eu"}, taskLabels: map[string]string{"region": "us"}},
		{name: "matching normal label", agentLabels: map[string]string{"region": "eu"}, taskLabels: map[string]string{"region": "eu"}, wantAgents: 1},
		{name: "normal label need not be requested", agentLabels: map[string]string{"region": "eu"}, wantAgents: 1},
		{name: "empty mandatory task value", agentLabels: map[string]string{"!region": "eu"}, taskLabels: map[string]string{"region": ""}},
		{name: "mandatory wildcard rejects arbitrary values", agentLabels: map[string]string{"!region": "*"}, taskLabels: map[string]string{"region": "eu"}},
		{name: "mandatory wildcard accepts literal star", agentLabels: map[string]string{"!region": "*"}, taskLabels: map[string]string{"region": "*"}, wantAgents: 1},
		{name: "repository restriction rejects other repositories", agentLabels: map[string]string{"repo": "acme/other"}},
		{name: "repository restriction accepts matching repository", agentLabels: map[string]string{"repo": "acme/api"}, wantAgents: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := testConfig(0, 1)
			cfg.ExtraAgentLabels = test.agentLabels
			h := newHarness(t, cfg, dockerAMD64)
			task := realWorkflowTask("build", "linux/amd64")
			for key, value := range test.taskLabels {
				task.Labels[key] = value
			}
			h.woodpecker.queue.Pending = []woodpecker.Task{task}
			h.reconcile(t)
			require.Len(t, h.provider.deployed, test.wantAgents)
			require.Len(t, h.woodpecker.agents, test.wantAgents)
		})
	}
}

// TestAggregateWorkerCountsDoNotHidePendingDemand checks that demand comes
// from the pending task list, not from the server's aggregate worker stats.
func TestAggregateWorkerCountsDoNotHidePendingDemand(t *testing.T) {
	h := newHarness(t, testConfig(0, 2), dockerAMD64)
	h.woodpecker.queue.Stats = woodpecker.QueueStats{Workers: 100, Pending: 1}
	h.woodpecker.queue.Pending = []woodpecker.Task{realWorkflowTask("build", "linux/amd64")}

	h.reconcile(t)

	require.Equal(t, []types.Capability{dockerAMD64}, h.provider.deployedCapabilities())
}

// TestWaitingOnDepsCreatesNoCapacity checks that work blocked on other
// workflows does not provision agents yet.
func TestWaitingOnDepsCreatesNoCapacity(t *testing.T) {
	h := newHarness(t, testConfig(0, 2), dockerAMD64)
	h.woodpecker.queue.WaitingOnDeps = []woodpecker.Task{realWorkflowTask("blocked", "linux/amd64")}
	h.woodpecker.queue.Stats.WaitingOnDeps = 1

	h.reconcile(t)

	require.Empty(t, h.provider.deployed)
	require.Empty(t, h.woodpecker.agents)
}

// TestUnschedulableWorkDoesNotConsumeCapacity checks that unservable tasks do
// not use up the budget needed by servable ones.
func TestUnschedulableWorkDoesNotConsumeCapacity(t *testing.T) {
	h := newHarness(t, testConfig(0, 2), dockerAMD64)
	h.woodpecker.queue.Pending = []woodpecker.Task{
		realWorkflowTask("unsupported-1", "linux/arm64"),
		realWorkflowTask("unsupported-2", "linux/arm64"),
		realWorkflowTask("supported", "linux/amd64"),
	}

	h.reconcile(t)

	require.Equal(t, []types.Capability{dockerAMD64}, h.provider.deployedCapabilities())
}

// TestRunningWorkBelongsToItsAgent checks that running tasks are attributed
// by agent ID, even when they carry no platform label.
func TestRunningWorkBelongsToItsAgent(t *testing.T) {
	h := newHarness(t, testConfig(0, 3), dockerAMD64, dockerARM64)
	agent := h.addConnectedAgent(t, "pool-e2e-agent-arm", dockerARM64)
	h.woodpecker.queue.Running = []woodpecker.Task{runningOn(realWorkflowTask("running", ""), agent.ID)}

	h.reconcile(t)

	require.Equal(t, []types.Capability{dockerARM64}, h.provider.deployedCapabilities())
	require.False(t, h.woodpecker.agentByName(t, agent.Name).NoSchedule)
}

// TestBackendDistinguishesCapabilities checks that two capabilities sharing a
// platform are told apart by their backend.
func TestBackendDistinguishesCapabilities(t *testing.T) {
	local := types.Capability{Platform: dockerAMD64.Platform, Backend: types.BackendLocal}
	h := newHarness(t, testConfig(0, 2), dockerAMD64, local)
	task := realWorkflowTask("local", "linux/amd64")
	task.Labels["backend"] = "local"
	h.woodpecker.queue.Pending = []woodpecker.Task{task}

	h.reconcile(t)

	require.Equal(t, []types.Capability{local}, h.provider.deployedCapabilities())
}
