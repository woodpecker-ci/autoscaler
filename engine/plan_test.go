package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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

	t.Run("scales up when pool is fully busy with a backlog", func(t *testing.T) {
		// All 3 existing pool agents are busy (running=3) and 2 more
		// workflows are queued. Regression test for 3b14c05: the legacy
		// calcAgents double-subtracted the pool size, which made it
		// return a negative value here (wanting to drain agents)
		// instead of scaling up.
		assert.Equal(t, 2, rawDelta(bucketState{Pending: 2, Running: 3, PoolAgents: 3}, 1))
	})
}

func Test_allocateBudget(t *testing.T) {
	t.Run("fills a MinAgents warm pool without demand", func(t *testing.T) {
		states := []bucketState{{}, {}}
		raw := []int{0, 0}
		final := allocateBudget(states, raw, poolLimits{Min: 2, Max: 5})
		// Two warm agents, placed on the first (busiest-by-tie) bucket.
		assert.Equal(t, []int{2, 0}, final)
	})

	t.Run("drains an excess capability before replacing it at a fixed size", func(t *testing.T) {
		states := []bucketState{
			{Pending: 1},    // wants an agent
			{PoolAgents: 1}, // idle wrong-capability agent holding the only slot
		}
		raw := []int{1, -1}
		final := allocateBudget(states, raw, poolLimits{Footprint: 1, Min: 1, Max: 1})
		// Full pool: drain the idle agent now; the create waits until its slot
		// frees next cycle.
		assert.Equal(t, []int{0, -1}, final)
	})

	t.Run("counts the footprint against MaxAgents", func(t *testing.T) {
		states := []bucketState{
			{Pending: 2, PoolAgents: 0}, // two booting agents don't match yet
		}
		raw := []int{2}
		// Two agents already occupy the two provider slots; create nothing more.
		final := allocateBudget(states, raw, poolLimits{Footprint: 2, Max: 2})
		assert.Equal(t, []int{0}, final)
	})

	t.Run("reactivates without requiring another provider slot", func(t *testing.T) {
		states := []bucketState{{Pending: 1, ReusableAgents: 1}}
		raw := []int{1}

		final := allocateBudget(states, raw, poolLimits{Footprint: 1, Max: 1})

		assert.Equal(t, []int{1}, final)
	})

	t.Run("clamps total scale-up at MaxAgents", func(t *testing.T) {
		// Two buckets, both want 5 agents, total cap is 6.
		states := []bucketState{
			{Pending: 5},
			{Pending: 5},
		}
		raw := []int{5, 5}
		final := allocateBudget(states, raw, poolLimits{Max: 6})
		// Busiest first, tied here, so the first bucket gets a full 5 and the
		// second the remaining 1.
		assert.Equal(t, []int{5, 1}, final)
	})

	t.Run("largest demand is served first", func(t *testing.T) {
		states := []bucketState{
			{Pending: 1}, // small demand
			{Pending: 5}, // large demand
		}
		raw := []int{1, 5}
		final := allocateBudget(states, raw, poolLimits{Max: 4})
		// 4 budget, big bucket gets 4, small gets 0.
		assert.Equal(t, []int{0, 4}, final)
	})

	t.Run("scale-down stops at MinAgents", func(t *testing.T) {
		states := []bucketState{
			{PoolAgents: 3},
		}
		raw := []int{-3}
		// 3 online, MinAgents=1 -> keep the warm floor, drain 2.
		final := allocateBudget(states, raw, poolLimits{Footprint: 3, Min: 1, Max: 5})
		assert.Equal(t, []int{-2}, final)
	})

	t.Run("scale-down can't drain more than the bucket has", func(t *testing.T) {
		states := []bucketState{
			{PoolAgents: 1}, // only 1 online
			{PoolAgents: 5},
		}
		raw := []int{-3, 0} // unreasonable raw; want clamped to 1
		final := allocateBudget(states, raw, poolLimits{Footprint: 6, Max: 10})
		assert.Equal(t, []int{-1, 0}, final)
	})

	t.Run("zero budgets when already at limits", func(t *testing.T) {
		states := []bucketState{
			{Pending: 1, PoolAgents: 5},
		}
		raw := []int{1}
		// 5 online, MaxAgents=5 -> no room to scale up.
		final := allocateBudget(states, raw, poolLimits{Footprint: 5, Max: 5})
		assert.Equal(t, []int{0}, final)
	})

	t.Run("drains a covered bucket fully once the floor is met elsewhere", func(t *testing.T) {
		states := []bucketState{
			{Pending: 2, PoolAgents: 0}, // demand -> will hold the warm floor
			{Pending: 0, PoolAgents: 3}, // idle, no demand
		}
		raw := []int{2, -3}
		// MinAgents=1 is a global floor: the demanded bucket already covers it,
		// so the idle bucket drains fully instead of keeping an unneeded agent.
		final := allocateBudget(states, raw, poolLimits{Footprint: 3, Min: 1, Max: 5})
		assert.Equal(t, []int{2, -3}, final)
	})
}
