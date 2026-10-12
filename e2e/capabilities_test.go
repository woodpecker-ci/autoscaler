package e2e_test

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// TestCapabilityDiscoveryErrorPreventsStartup checks that the autoscaler does
// not start without knowing what the provider can deploy.
func TestCapabilityDiscoveryErrorPreventsStartup(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

	h := newHarness(t, testConfig(1, 3))
	h.addConnectedAgent(t, "pool-e2e-agent-standing", dockerAMD64)
	h.woodpecker.queue.Pending = []woodpecker.Task{
		realWorkflowTask("build", "linux/amd64"),
	}

	h.reconcile(t)

	require.Len(t, h.provider.deployed, 1)
	require.Len(t, h.woodpecker.agents, 1)
}

// TestCapabilityWithoutPlatform checks that a provider which cannot tell its
// architecture keeps the agents it deployed, whatever platform they report on
// connect, and only scales for workflows that do not ask for a platform.
func TestCapabilityWithoutPlatform(t *testing.T) {
	t.Parallel()

	dockerAnyPlatform := types.Capability{Backend: types.BackendDocker}
	h := newHarness(t, testConfig(0, 3), dockerAnyPlatform)

	h.woodpecker.queue.Pending = []woodpecker.Task{realWorkflowTask("build", "")}
	h.reconcile(t)
	require.Equal(t, []types.Capability{dockerAnyPlatform}, h.provider.deployedCapabilities())

	// the agent connects with its real platform, idle and ready for the workflow
	h.connectAgents(t)
	agent := h.woodpecker.agentByName(t, slices.Collect(maps.Keys(h.provider.deployed))[0])
	agent.Platform = dockerAMD64.Platform
	h.woodpecker.put(agent)
	h.markIdle()

	t.Run("connected agent counts as capacity", func(t *testing.T) {
		h.reconcile(t)
		require.Equal(t, []string{agent.Name}, slices.Collect(maps.Keys(h.provider.deployed)))
		require.False(t, h.woodpecker.agentByName(t, agent.Name).NoSchedule)

		t.Run("workflow asking for a platform does not scale", func(t *testing.T) {
			h.woodpecker.queue.Pending = []woodpecker.Task{realWorkflowTask("pinned", "linux/amd64")}
			h.reconcile(t)
			for name := range h.provider.deployed {
				require.Equal(t, agent.Name, name)
			}
		})
	})
}

// TestOverlappingCapabilitiesPreventStartup checks that the autoscaler does
// not start when an agent could belong to more than one capability.
func TestOverlappingCapabilitiesPreventStartup(t *testing.T) {
	t.Parallel()

	dockerAnyPlatform := types.Capability{Backend: types.BackendDocker}
	amd64AnyBackend := types.Capability{Platform: "linux/amd64"}
	kubernetesAMD64 := types.Capability{Platform: "linux/amd64", Backend: types.BackendKubernetes}

	for _, test := range []struct {
		name         string
		capabilities []types.Capability
		wantErr      bool
	}{
		{name: "distinct platforms", capabilities: []types.Capability{dockerAMD64, dockerARM64}},
		{name: "distinct backends", capabilities: []types.Capability{dockerAMD64, kubernetesAMD64}},
		{name: "single open capability", capabilities: []types.Capability{dockerAnyPlatform}},
		{name: "duplicate", capabilities: []types.Capability{dockerAMD64, dockerAMD64}, wantErr: true},
		{name: "open platform next to a platform", capabilities: []types.Capability{dockerARM64, dockerAnyPlatform}, wantErr: true},
		{name: "open backend next to a backend", capabilities: []types.Capability{amd64AnyBackend, dockerAMD64}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := engine.NewAutoscaler(t.Context(), newFakeProvider(test.capabilities...), newFakeWoodpecker(), testConfig(0, 1))
			if test.wantErr {
				require.ErrorIs(t, err, engine.ErrOverlappingCapabilities)
				return
			}
			require.NoError(t, err)
		})
	}
}
