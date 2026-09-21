package e2e_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// TestUnavailableCapabilityAgentIsRetired checks that an idle agent whose
// capability the provider no longer offers is removed.
func TestUnavailableCapabilityAgentIsRetired(t *testing.T) {
	h := newHarness(t, testConfig(0, 2), dockerAMD64)
	h.addConnectedAgent(t, "pool-e2e-agent-drifted", dockerARM64)

	h.reconcile(t)

	require.Empty(t, h.provider.deployed)
	require.Empty(t, h.woodpecker.agents)
}

// TestNeverConnectedAgentIsReaped checks that an agent which never contacted
// the server is removed after AgentInactivityTimeout.
func TestNeverConnectedAgentIsReaped(t *testing.T) {
	cfg := testConfig(0, 2)
	cfg.AgentInactivityTimeout = time.Minute
	h := newHarness(t, cfg, dockerAMD64)

	name := "pool-e2e-agent-never-connected"
	agent, err := h.woodpecker.AgentCreate(&woodpecker.Agent{Name: name})
	require.NoError(t, err)
	agent.Created = time.Now().Add(-2 * time.Minute).Unix()
	h.woodpecker.put(agent)
	h.provider.deployed[name] = dockerAMD64

	h.reconcile(t)

	require.Empty(t, h.provider.deployed)
	require.Empty(t, h.woodpecker.agents)
}

// TestSilentAgentIsReaped checks that a connected agent that stopped reporting
// is removed, even below MinAgents.
func TestSilentAgentIsReaped(t *testing.T) {
	cfg := testConfig(1, 1)
	cfg.AgentInactivityTimeout = time.Minute
	h := newHarness(t, cfg, dockerAMD64)
	agent := h.addConnectedAgent(t, "pool-e2e-agent-silent", dockerAMD64)
	agent.LastContact = time.Now().Add(-2 * time.Minute).Unix()
	h.woodpecker.put(agent)

	h.reconcile(t)

	require.Empty(t, h.provider.deployed)
	require.Empty(t, h.woodpecker.agents)
}

// TestProviderServerDriftIsReconciled checks that agents known to only one of
// provider or server are cleaned up on both sides.
func TestProviderServerDriftIsReconciled(t *testing.T) {
	h := newHarness(t, testConfig(0, 3), dockerAMD64)
	h.provider.deployed["pool-e2e-agent-provider-only"] = dockerAMD64

	serverOnly, err := h.woodpecker.AgentCreate(&woodpecker.Agent{Name: "pool-e2e-agent-server-only"})
	require.NoError(t, err)
	serverOnly.Platform = dockerAMD64.Platform
	serverOnly.Backend = string(dockerAMD64.Backend)
	serverOnly.LastContact = time.Now().Unix()
	serverOnly.LastWork = time.Now().Unix()
	h.woodpecker.put(serverOnly)

	h.reconcile(t)

	require.Empty(t, h.provider.deployed)
	require.Empty(t, h.woodpecker.agents)
}

// TestCleanupLeavesForeignAgentsUntouched checks that agents of other pools or
// outside any pool are never modified.
func TestCleanupLeavesForeignAgentsUntouched(t *testing.T) {
	h := newHarness(t, testConfig(0, 1), dockerAMD64)
	for _, name := range []string{"pool-e2e-other-agent-1", "pool-other-agent-1", "ordinary-agent", "prefix-pool-e2e-agent-1"} {
		_, err := h.woodpecker.AgentCreate(&woodpecker.Agent{Name: name})
		require.NoError(t, err)
	}
	before, err := h.woodpecker.AgentList()
	require.NoError(t, err)
	h.woodpecker.queue.Pending = []woodpecker.Task{realWorkflowTask("build", "linux/amd64")}

	h.reconcile(t)

	require.Len(t, h.provider.deployed, 1)
	require.Len(t, h.woodpecker.agents, len(before)+1)
	for _, agent := range before {
		require.Equal(t, agent, h.woodpecker.agentByName(t, agent.Name))
	}
}

// TestCleanupRetainsInFlightWork checks that stale or unavailable agents are
// kept while they run work and removed once it completes.
func TestCleanupRetainsInFlightWork(t *testing.T) {
	for _, test := range []struct {
		name       string
		capability types.Capability
		stale      bool
	}{
		{name: "stale", capability: dockerAMD64, stale: true},
		{name: "unavailable capability", capability: dockerARM64},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(t, testConfig(0, 2), dockerAMD64)
			agent := h.addConnectedAgent(t, "pool-e2e-agent-busy", test.capability)
			if test.stale {
				agent.LastContact = time.Now().Add(-2 * time.Hour).Unix()
				h.woodpecker.put(agent)
			}
			h.woodpecker.queue.Running = []woodpecker.Task{runningOn(realWorkflowTask("busy", test.capability.Platform), agent.ID)}
			h.reconcile(t)
			require.Contains(t, h.provider.deployed, agent.Name)
			require.Contains(t, h.woodpecker.agents, agent.ID)

			t.Run("completion allows cleanup", func(t *testing.T) {
				h.woodpecker.queue.Running = nil
				h.reconcile(t)
				require.Empty(t, h.provider.deployed)
				require.Empty(t, h.woodpecker.agents)
			})
		})
	}
}
