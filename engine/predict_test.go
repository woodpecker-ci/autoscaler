package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

func bucketsForTest(caps []types.Capability, extra map[string]string) []agentBucket {
	out := make([]agentBucket, 0, len(caps))
	for _, c := range caps {
		out = append(out, agentBucket{
			Capability: c,
			Labels:     agentLabelsFor(c, extra),
		})
	}
	return out
}

func Test_routeTaskToBucket(t *testing.T) {
	buckets := bucketsForTest([]types.Capability{dockerAmd64Cap, dockerArm64Cap}, nil)

	t.Run("picks the first matching bucket", func(t *testing.T) {
		idx := routeTaskToBucket(woodpecker.Task{
			Labels: map[string]string{"platform": "linux/amd64"},
		}, buckets)
		assert.Equal(t, 0, idx)
	})

	t.Run("picks the second bucket when only it matches", func(t *testing.T) {
		idx := routeTaskToBucket(woodpecker.Task{
			Labels: map[string]string{"platform": "linux/arm64"},
		}, buckets)
		assert.Equal(t, 1, idx)
	})

	t.Run("returns -1 when no bucket matches", func(t *testing.T) {
		idx := routeTaskToBucket(woodpecker.Task{
			Labels: map[string]string{"backend": "local"},
		}, buckets)
		assert.Equal(t, -1, idx)
	})

	t.Run("an unlabelled task matches the first bucket", func(t *testing.T) {
		// Per the woodpecker rules, an empty workflow label set is
		// satisfied by every agent — so the task is routable to the first
		// bucket we know about.
		idx := routeTaskToBucket(woodpecker.Task{}, buckets)
		assert.Equal(t, 0, idx)
	})
}

func Test_matchAgentToBucket(t *testing.T) {
	buckets := bucketsForTest([]types.Capability{dockerAmd64Cap, dockerArm64Cap}, nil)

	t.Run("matches by reported platform and backend", func(t *testing.T) {
		idx := matchAgentToBucket(&woodpecker.Agent{
			Platform: "linux/arm64",
			Backend:  "docker",
		}, buckets)
		assert.Equal(t, 1, idx)
	})

	t.Run("returns -1 when no bucket matches", func(t *testing.T) {
		idx := matchAgentToBucket(&woodpecker.Agent{
			Platform: "linux/riscv64",
			Backend:  "docker",
		}, buckets)
		assert.Equal(t, -1, idx)
	})

	t.Run("ignores custom labels — match is on (platform, backend) only", func(t *testing.T) {
		// Even if the operator changes ExtraAgentLabels between reconciles
		// and the agent's CustomLabels drift, we still find the right
		// bucket because (platform, backend) is stable.
		idx := matchAgentToBucket(&woodpecker.Agent{
			Platform: "linux/amd64",
			Backend:  "docker",
			CustomLabels: map[string]string{
				"region": "europe", // not in bucket extras
			},
		}, buckets)
		assert.Equal(t, 0, idx)
	})
}

func Test_computeBucketStates(t *testing.T) {
	buckets := bucketsForTest([]types.Capability{dockerAmd64Cap, dockerArm64Cap}, nil)

	pending := []woodpecker.Task{
		{Labels: map[string]string{"platform": "linux/amd64"}},
		{Labels: map[string]string{"platform": "linux/amd64"}},
		{Labels: map[string]string{"platform": "linux/arm64"}},
		{Labels: map[string]string{"backend": "local"}}, // unschedulable
	}
	running := []woodpecker.Task{
		{Labels: map[string]string{"platform": "linux/arm64"}},
	}
	pool := []*woodpecker.Agent{
		{ID: 1, Platform: "linux/amd64", Backend: "docker"},
		{ID: 2, Platform: "linux/arm64", Backend: "docker", NoSchedule: true}, // drained, skipped
		{ID: 3, Platform: "linux/arm64", Backend: "docker"},
	}

	states, unmatchedPending, unmatchedRunning := computeBucketStates(buckets, pending, running, pool)
	assert.Equal(t, 1, unmatchedPending)
	assert.Equal(t, 0, unmatchedRunning)

	// amd64 bucket
	assert.Equal(t, 2, states[0].Pending)
	assert.Equal(t, 0, states[0].Running)
	assert.Equal(t, 1, states[0].PoolAgents)
	// arm64 bucket
	assert.Equal(t, 1, states[1].Pending)
	assert.Equal(t, 1, states[1].Running)
	assert.Equal(t, 1, states[1].PoolAgents) // drained one not counted
}

func Test_rawDelta(t *testing.T) {
	t.Run("scales up when more work than capacity", func(t *testing.T) {
		assert.Equal(t, 2, rawDelta(bucketState{Pending: 4, Running: 0, PoolAgents: 0}, 2))
	})

	t.Run("scales down when overprovisioned", func(t *testing.T) {
		assert.Equal(t, -2, rawDelta(bucketState{Pending: 0, Running: 0, PoolAgents: 2}, 1))
	})

	t.Run("does not scale when capacity matches demand exactly", func(t *testing.T) {
		assert.Equal(t, 0, rawDelta(bucketState{Pending: 2, Running: 0, PoolAgents: 2}, 1))
	})

	t.Run("rounds up partial agent need", func(t *testing.T) {
		// 7 tasks, WPA=5 -> ceil(7/5) = 2.
		assert.Equal(t, 2, rawDelta(bucketState{Pending: 7, PoolAgents: 0}, 5))
	})

	t.Run("treats zero or negative WorkflowsPerAgent as 1", func(t *testing.T) {
		assert.Equal(t, 3, rawDelta(bucketState{Pending: 3}, 0))
	})

	t.Run("counts running tasks against required capacity", func(t *testing.T) {
		// 1 running + 1 pending = 2 tasks, WPA=1, no pool -> need 2.
		assert.Equal(t, 2, rawDelta(bucketState{Pending: 1, Running: 1}, 1))
	})
}
