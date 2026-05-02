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
}

func Test_allocateBudget(t *testing.T) {
	t.Run("clamps total scale-up at MaxAgents", func(t *testing.T) {
		// Two buckets, both want 5 agents, total cap is 6.
		states := []bucketState{
			{Pending: 5},
			{Pending: 5},
		}
		raw := []int{5, 5}
		final := allocateBudget(states, raw, 0, 6)
		// Greedy by demand magnitude — tied here, so first-come gets full
		// 5, second gets remaining 1.
		assert.Equal(t, []int{5, 1}, final)
	})

	t.Run("largest demand is served first", func(t *testing.T) {
		states := []bucketState{
			{Pending: 1}, // small demand
			{Pending: 5}, // large demand
		}
		raw := []int{1, 5}
		final := allocateBudget(states, raw, 0, 4)
		// 4 budget, big bucket gets 4, small gets 0.
		assert.Equal(t, []int{0, 4}, final)
	})

	t.Run("scale-down stops at MinAgents", func(t *testing.T) {
		states := []bucketState{
			{PoolAgents: 3},
		}
		raw := []int{-3}
		final := allocateBudget(states, raw, 1, 5)
		// 3 online, MinAgents=1 -> downBudget = 2, can only drain 2.
		assert.Equal(t, []int{-2}, final)
	})

	t.Run("scale-down can't drain more than the bucket has", func(t *testing.T) {
		states := []bucketState{
			{PoolAgents: 1}, // only 1 online
			{PoolAgents: 5},
		}
		raw := []int{-3, 0} // unreasonable raw; want clamped to 1
		final := allocateBudget(states, raw, 0, 10)
		assert.Equal(t, []int{-1, 0}, final)
	})

	t.Run("zero budgets when already at limits", func(t *testing.T) {
		states := []bucketState{
			{Pending: 1, PoolAgents: 5},
		}
		raw := []int{1}
		// totalOnline=5, MaxAgents=5 -> upBudget=0, can't scale up.
		final := allocateBudget(states, raw, 0, 5)
		assert.Equal(t, []int{0}, final)
	})

	t.Run("up and down across buckets are independent", func(t *testing.T) {
		states := []bucketState{
			{Pending: 2, PoolAgents: 0},
			{Pending: 0, PoolAgents: 3},
		}
		raw := []int{2, -3}
		// MinAgents=1, MaxAgents=5, totalOnline=3.
		// upBudget = 5-3 = 2, downBudget = 3-1 = 2.
		final := allocateBudget(states, raw, 1, 5)
		assert.Equal(t, []int{2, -2}, final)
	})
}
