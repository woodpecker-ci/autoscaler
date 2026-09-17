package e2e_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

func testTaskRouting(t *testing.T) {
	t.Run("label matching", testLabelMatching)
	t.Run("queue attribution", testQueueAttribution)
	t.Run("real queue labels route work to matching platforms", func(t *testing.T) {
		h := newHarness(t, testConfig(0, 2), dockerAMD64, dockerARM64)
		amd64 := realWorkflowTask("build-amd64", "linux/amd64")
		amd64.Labels["empty-value-is-ignored"] = ""
		arm64 := realWorkflowTask("build-arm64", "linux/arm64")
		h.woodpecker.queue.Pending = []woodpecker.Task{amd64, arm64}

		h.reconcile(t)

		require.ElementsMatch(t, []types.Capability{dockerAMD64, dockerARM64}, h.provider.deployedCapabilities())
	})

	t.Run("a task without a platform uses the first matching capability", func(t *testing.T) {
		h := newHarness(t, testConfig(0, 1), dockerARM64, dockerAMD64)
		h.woodpecker.queue.Pending = []woodpecker.Task{
			realWorkflowTask("unconstrained", ""),
		}

		h.reconcile(t)

		require.Equal(t, []types.Capability{dockerARM64}, h.provider.deployedCapabilities())
	})

	t.Run("mandatory labels require an explicit exact value", func(t *testing.T) {
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
	})

	t.Run("a wildcard extra label accepts any workflow value", func(t *testing.T) {
		cfg := testConfig(0, 1)
		cfg.ExtraAgentLabels = map[string]string{"region": "*"}
		h := newHarness(t, cfg, dockerAMD64)
		task := realWorkflowTask("build", "linux/amd64")
		task.Labels["region"] = "eu"
		h.woodpecker.queue.Pending = []woodpecker.Task{task}

		h.reconcile(t)

		require.Equal(t, []types.Capability{dockerAMD64}, h.provider.deployedCapabilities())
	})

	t.Run("unschedulable pending work does not create an unusable agent", func(t *testing.T) {
		h := newHarness(t, testConfig(0, 3), dockerAMD64)
		h.woodpecker.queue.Pending = []woodpecker.Task{
			realWorkflowTask("needs-arm", "linux/arm64"),
		}

		h.reconcile(t)

		require.Empty(t, h.provider.deployed)
		require.Empty(t, h.woodpecker.agents)
	})
}

func testLabelMatching(t *testing.T) {
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

func testQueueAttribution(t *testing.T) {
	t.Run("aggregate worker counts do not hide pending demand", func(t *testing.T) {
		h := newHarness(t, testConfig(0, 2), dockerAMD64)
		h.woodpecker.queue.Stats = woodpecker.QueueStats{Workers: 100, Pending: 1}
		h.woodpecker.queue.Pending = []woodpecker.Task{realWorkflowTask("build", "linux/amd64")}
		h.reconcile(t)
		require.Equal(t, []types.Capability{dockerAMD64}, h.provider.deployedCapabilities())
	})

	t.Run("work waiting on dependencies creates no capacity yet", func(t *testing.T) {
		h := newHarness(t, testConfig(0, 2), dockerAMD64)
		h.woodpecker.queue.WaitingOnDeps = []woodpecker.Task{realWorkflowTask("blocked", "linux/amd64")}
		h.woodpecker.queue.Stats.WaitingOnDeps = 1
		h.reconcile(t)
		require.Empty(t, h.provider.deployed)
		require.Empty(t, h.woodpecker.agents)
	})

	t.Run("unschedulable work does not consume capacity for matching work", func(t *testing.T) {
		h := newHarness(t, testConfig(0, 2), dockerAMD64)
		h.woodpecker.queue.Pending = []woodpecker.Task{
			realWorkflowTask("unsupported-1", "linux/arm64"),
			realWorkflowTask("unsupported-2", "linux/arm64"),
			realWorkflowTask("supported", "linux/amd64"),
		}
		h.reconcile(t)
		require.Equal(t, []types.Capability{dockerAMD64}, h.provider.deployedCapabilities())
	})

	t.Run("running work belongs to its agent even without platform labels", func(t *testing.T) {
		h := newHarness(t, testConfig(0, 3), dockerAMD64, dockerARM64)
		agent := h.addConnectedAgent(t, "pool-e2e-agent-arm", dockerARM64)
		h.woodpecker.queue.Running = []woodpecker.Task{runningOn(realWorkflowTask("running", ""), agent.ID)}
		h.reconcile(t)
		require.Equal(t, []types.Capability{dockerARM64}, h.provider.deployedCapabilities())
		require.False(t, h.woodpecker.agentByName(t, agent.Name).NoSchedule)
	})

	t.Run("backend distinguishes capabilities with the same platform", func(t *testing.T) {
		local := types.Capability{Platform: dockerAMD64.Platform, Backend: types.BackendLocal}
		h := newHarness(t, testConfig(0, 2), dockerAMD64, local)
		task := realWorkflowTask("local", "linux/amd64")
		task.Labels["backend"] = "local"
		h.woodpecker.queue.Pending = []woodpecker.Task{task}
		h.reconcile(t)
		require.Equal(t, []types.Capability{local}, h.provider.deployedCapabilities())
	})
}
