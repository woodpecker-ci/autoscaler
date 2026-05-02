package engine

import (
	"math"
	"sort"

	"github.com/rs/zerolog/log"

	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// rawDelta computes the unbounded scaling delta for one bucket: how many
// agents this bucket needs *if* there were no Min/Max caps.
//
// We need enough agents to chew through (Pending + Running) at
// WorkflowsPerAgent parallelism. We already have PoolAgents online. The
// delta is the difference, rounded up so partial agents become a full one.
//
// We deliberately do NOT subtract a separately-counted "free workers"
// term: each pool agent already provides WPA slots, so each online pool
// agent in the bucket already accounts for WPA workflows of capacity.
// The legacy formula double-counted free workers against pool agents.
func rawDelta(s bucketState, workflowsPerAgent int) int {
	if workflowsPerAgent <= 0 {
		workflowsPerAgent = 1
	}
	required := int(math.Ceil(float64(s.Pending+s.Running) / float64(workflowsPerAgent)))
	return required - s.PoolAgents
}

// planScaling produces the list of per-bucket scale up/down deltas the
// Reconcile loop should act on. Buckets with a zero delta are omitted.
//
// When no provider capabilities are available (e.g. the provider couldn't
// be queried) planScaling returns nil and logs — there is no safe way to
// scale without knowing what we can deploy.
func (a *Autoscaler) planScaling(pending, running []woodpecker.Task) []bucketDecision {
	buckets := a.agentBuckets()
	if len(buckets) == 0 {
		log.Warn().Msg("no provider capabilities known; skipping scale decision")
		return nil
	}

	states, unmatchedPending, unmatchedRunning := computeBucketStates(
		buckets, pending, running, a.agents,
	)

	if unmatchedPending > 0 {
		log.Warn().Int("count", unmatchedPending).Msg(
			"pending tasks have labels no configured agent can satisfy; not scaling for them")
	}
	if unmatchedRunning > 0 {
		log.Debug().Int("count", unmatchedRunning).Msg(
			"running tasks didn't match any configured bucket")
	}

	rawDeltas := make([]int, len(states))
	for i := range states {
		rawDeltas[i] = rawDelta(states[i], a.config.WorkflowsPerAgent)
	}
	finalDeltas := allocateBudget(states, rawDeltas, a.config.MinAgents, a.config.MaxAgents)

	out := make([]bucketDecision, 0, len(states))
	for i, s := range states {
		if finalDeltas[i] == 0 {
			continue
		}
		out = append(out, bucketDecision{Bucket: s.Bucket, Delta: finalDeltas[i]})
		log.Debug().
			Str("platform", s.Bucket.Capability.Platform).
			Str("backend", string(s.Bucket.Capability.Backend)).
			Int("pending", s.Pending).
			Int("running", s.Running).
			Int("pool", s.PoolAgents).
			Int("delta", finalDeltas[i]).
			Msg("bucket plan")
	}
	return out
}

// allocateBudget clamps each bucket's raw delta to the global Min/Max
// caps, distributing the remaining scale-up/scale-down budget greedily,
// largest absolute demand first, so the most-loaded buckets are served
// first. Returns one final delta per bucket, in the same order as states.
func allocateBudget(states []bucketState, rawDeltas []int, minAgents, maxAgents int) []int {
	totalOnline := 0
	for _, s := range states {
		totalOnline += s.PoolAgents
	}
	upBudget := max(maxAgents-totalOnline, 0)
	downBudget := max(totalOnline-minAgents, 0)

	order := make([]int, len(states))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		ai, aj := rawDeltas[order[i]], rawDeltas[order[j]]
		if ai < 0 {
			ai = -ai
		}
		if aj < 0 {
			aj = -aj
		}
		return ai > aj
	})

	finalDeltas := make([]int, len(states))
	for _, idx := range order {
		raw := rawDeltas[idx]
		switch {
		case raw > 0:
			grant := min(raw, upBudget)
			finalDeltas[idx] = grant
			upBudget -= grant
		case raw < 0:
			// Drain at most |raw| agents from this bucket, at most this
			// bucket's own PoolAgents (can't drain what isn't there),
			// and at most the global downBudget.
			want := min(-raw, states[idx].PoolAgents, downBudget)
			finalDeltas[idx] = -want
			downBudget -= want
		}
	}
	return finalDeltas
}
