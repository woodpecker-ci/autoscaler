package e2e_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// TestCapabilityDiscoveryErrorPreventsStartup checks that the autoscaler does
// not start without knowing what the provider can deploy.
func TestCapabilityDiscoveryErrorPreventsStartup(t *testing.T) {
	provider := newFakeProvider(dockerAMD64)
	provider.capabilitiesErr = errors.New("discovery failed")
	woodpecker := newFakeWoodpecker()

	_, err := engine.NewAutoscaler(t.Context(), provider, woodpecker, testConfig(0, 1))

	require.ErrorContains(t, err, "could not query provider capabilities")
	require.ErrorContains(t, err, "discovery failed")
}

// TestCapabilitiesAreCachedAfterStartup checks that capabilities are queried
// once and later provider changes are not picked up by reconcile.
func TestCapabilitiesAreCachedAfterStartup(t *testing.T) {
	h := newHarness(t, testConfig(0, 1), dockerAMD64)
	require.Equal(t, 1, h.provider.capabilitiesCalls)

	h.provider.capabilities = []types.Capability{dockerARM64}
	h.woodpecker.queue.Pending = []woodpecker.Task{
		realWorkflowTask("needs-arm", "linux/arm64"),
	}
	h.reconcile(t)

	require.Equal(t, 1, h.provider.capabilitiesCalls)
	require.Empty(t, h.provider.deployed)
}

// TestEmptyCapabilitiesHoldFleetSteady checks that without any capability the
// existing fleet is neither grown nor drained.
func TestEmptyCapabilitiesHoldFleetSteady(t *testing.T) {
	h := newHarness(t, testConfig(1, 3))
	h.addConnectedAgent(t, "pool-e2e-agent-standing", dockerAMD64)
	h.woodpecker.queue.Pending = []woodpecker.Task{
		realWorkflowTask("build", "linux/amd64"),
	}

	h.reconcile(t)

	require.Len(t, h.provider.deployed, 1)
	require.Len(t, h.woodpecker.agents, 1)
}
