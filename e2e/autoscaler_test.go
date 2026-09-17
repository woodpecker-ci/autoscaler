package e2e_test

import (
	"testing"

	"go.woodpecker-ci.org/autoscaler/engine/types"
)

var (
	dockerAMD64 = types.Capability{Platform: "linux/amd64", Backend: types.BackendDocker}
	dockerARM64 = types.Capability{Platform: "linux/arm64", Backend: types.BackendDocker}
)

// TestAutoscaler is the behavior map for the complete reconciliation loop.
// Each group lives in a focused file, while nested subtests expose every case
// in `go test -v` output and allow related lifecycle steps to share a harness.
// Dependent steps are nested inside their prerequisite, so selecting a leaf
// also runs its setup.
func TestAutoscaler(t *testing.T) {
	t.Run("task routing", testTaskRouting)
	t.Run("pool limits", testPoolLimits)
	t.Run("agent lifecycle", testAgentLifecycle)
	t.Run("billing", testBilling)
	t.Run("cleanup", testCleanup)
	t.Run("failure recovery", testFailureRecovery)
	t.Run("provider capability discovery", testProviderCapabilityDiscovery)
}
