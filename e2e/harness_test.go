package e2e_test

import (
	"maps"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var (
	dockerAMD64 = types.Capability{Platform: "linux/amd64", Backend: types.BackendDocker}
	dockerARM64 = types.Capability{Platform: "linux/arm64", Backend: types.BackendDocker}
)

// harness wires the real engine.Autoscaler to in-memory fakes of the provider
// and the woodpecker server, so tests can drive whole reconcile cycles through
// the public API and assert on the resulting pool. A harness is not safe for
// concurrent use: subtests that continue on a parent's harness must not call
// t.Parallel.
type harness struct {
	config     *config.Config
	autoscaler *engine.Autoscaler
	provider   *fakeProvider
	woodpecker *fakeWoodpecker
}

func newHarness(t *testing.T, cfg *config.Config, capabilities ...types.Capability) *harness {
	t.Helper()

	provider := newFakeProvider(capabilities...)
	woodpecker := newFakeWoodpecker()

	autoscaler, err := engine.NewAutoscaler(t.Context(), provider, woodpecker, cfg)
	require.NoError(t, err)

	return &harness{config: cfg, autoscaler: autoscaler, provider: provider, woodpecker: woodpecker}
}

func testConfig(minAgents, maxAgents int) *config.Config {
	return &config.Config{
		PoolID:                 "e2e",
		MinAgents:              minAgents,
		MaxAgents:              maxAgents,
		WorkflowsPerAgent:      1,
		AgentIdleTimeout:       time.Minute,
		AgentInactivityTimeout: time.Hour,
		BillingModel:           types.BillingPerSecond,
	}
}

// hourlyConfig is a single-agent pool on an hourly-billed provider with a
// one-minute teardown window at the end of every paid hour.
func hourlyConfig() *config.Config {
	cfg := testConfig(0, 1)
	cfg.BillingModel = types.BillingHourlyRoundUp
	cfg.ReconciliationInterval = time.Minute
	cfg.AgentBillingTeardownMargin = time.Minute
	return cfg
}

func (h *harness) reconcile(t *testing.T) {
	t.Helper()
	require.NoError(t, h.autoscaler.Reconcile(t.Context()))
}

// connectAgents simulates every freshly deployed agent registering with the
// server and reporting the capability it was deployed for.
func (h *harness) connectAgents(t *testing.T) {
	t.Helper()
	for name, capability := range h.provider.deployed {
		agent := h.woodpecker.agentByName(t, name)
		if agent.LastContact != 0 {
			continue
		}
		agent.CustomLabels = maps.Clone(h.config.ExtraAgentLabels)
		agent.Platform = capability.Platform
		agent.Backend = string(capability.Backend)
		agent.LastContact = time.Now().Unix()
		agent.LastWork = time.Now().Unix()
		h.woodpecker.put(agent)
	}
}

// addConnectedAgent seeds an already-registered, idle agent that the provider
// also knows about, as a starting point for replacement scenarios.
func (h *harness) addConnectedAgent(t *testing.T, name string, capability types.Capability) *woodpecker.Agent {
	t.Helper()
	agent, err := h.woodpecker.AgentCreate(&woodpecker.Agent{Name: name})
	require.NoError(t, err)
	agent.Platform = capability.Platform
	agent.Backend = string(capability.Backend)
	agent.LastContact = time.Now().Unix()
	agent.LastWork = time.Now().Add(-2 * time.Minute).Unix()
	agent.CustomLabels = maps.Clone(h.config.ExtraAgentLabels)
	h.woodpecker.put(agent)
	h.provider.deployed[name] = capability
	return agent
}

// markIdle pushes every agent's last-work time past the idle timeout.
func (h *harness) markIdle() {
	for _, agent := range h.woodpecker.agents {
		agent.LastWork = time.Now().Add(-2 * time.Minute).Unix()
	}
}

func (h *harness) agentIDForPlatform(t *testing.T, platform string) int64 {
	t.Helper()
	for _, agent := range h.woodpecker.agents {
		if agent.Platform == platform {
			return agent.ID
		}
	}
	require.FailNowf(t, "no connected agent", "platform %q", platform)
	return 0
}

// realWorkflowTask mirrors what /queue/info returns: the workflow's own labels
// plus the org-id/repo and internal labels the server stamps on every task.
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

func runningOn(task woodpecker.Task, agentID int64) woodpecker.Task {
	task.AgentID = agentID
	return task
}
