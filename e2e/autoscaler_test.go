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

func TestAutoscalerHandlesARealWorkflowLifecycle(t *testing.T) {
	environment := newTestEnvironment(t, testConfig(0, 2), dockerAMD64, dockerARM64)
	environment.woodpecker.queue.Pending = []woodpecker.Task{
		realWorkflowTask("build-amd64", "linux/amd64"),
		realWorkflowTask("build-arm64", "linux/arm64"),
	}

	t.Log("pending workflows create one agent for each required platform")
	environment.reconcile(t)
	require.Len(t, environment.provider.deployed, 2)
	require.ElementsMatch(
		t,
		[]types.Capability{dockerAMD64, dockerARM64},
		environment.provider.deployedCapabilities(),
	)
	require.Len(t, environment.woodpecker.agents, 2)

	t.Log("another reconciliation during provisioning does not exceed MaxAgents")
	environment.reconcile(t)
	require.Len(t, environment.provider.deployed, 2)
	require.Len(t, environment.woodpecker.agents, 2)

	t.Log("agents connect and Woodpecker assigns each workflow to its matching platform")
	environment.connectDeployedAgents(t)
	environment.woodpecker.queue.Running = []woodpecker.Task{
		runningTask(
			realWorkflowTask("build-amd64", "linux/amd64"),
			environment.agentIDForPlatform(t, "linux/amd64"),
		),
		runningTask(
			realWorkflowTask("build-arm64", "linux/arm64"),
			environment.agentIDForPlatform(t, "linux/arm64"),
		),
	}
	environment.woodpecker.queue.Pending = nil
	environment.reconcile(t)
	require.Len(t, environment.provider.deployed, 2)

	t.Log("completed workflows leave idle agents that are drained and removed")
	environment.woodpecker.queue.Running = nil
	environment.markAgentsIdle()
	environment.reconcile(t)
	require.Empty(t, environment.provider.deployed)
	require.Empty(t, environment.woodpecker.agents)
}

func TestAutoscalerReplacesAnIdleAgentWithTheRequiredCapability(t *testing.T) {
	environment := newTestEnvironment(t, testConfig(1, 1), dockerAMD64, dockerARM64)
	environment.addConnectedAgent(t, "pool-e2e-agent-existing", dockerAMD64)
	environment.woodpecker.queue.Pending = []woodpecker.Task{
		realWorkflowTask("build-arm64", "linux/arm64"),
	}

	t.Log("the full pool first removes idle capacity with the wrong capability")
	environment.reconcile(t)
	require.Empty(t, environment.provider.deployed)
	require.Empty(t, environment.woodpecker.agents)

	t.Log("the next reconciliation fills the available slot with the required capability")
	environment.reconcile(t)
	require.Equal(t, []types.Capability{dockerARM64}, environment.provider.deployedCapabilities())
	require.Len(t, environment.woodpecker.agents, 1)
}

func realWorkflowTask(id, platform string) woodpecker.Task {
	return woodpecker.Task{
		ID: id,
		Labels: map[string]string{
			"platform":                         platform,
			"backend":                          "docker",
			"org-id":                           "42",
			"repo":                             "acme/api",
			"woodpecker-ci.org/pipeline-event": "push",
		},
	}
}

func runningTask(task woodpecker.Task, agentID int64) woodpecker.Task {
	task.AgentID = agentID
	return task
}
