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
			"pending tasks have labels no configured agent can satisfy; not scaling for them",
		)
	}
	if unmatchedRunning > 0 {
		log.Debug().Int("count", unmatchedRunning).Msg(
			"running tasks didn't match any configured bucket",
		)
	}

	rawDeltas := make([]int, len(states))
	for i := range states {
		rawDeltas[i] = rawDelta(states[i], a.config.WorkflowsPerAgent)
	}
	finalDeltas := allocateBudget(states, rawDeltas, poolLimits{
		Footprint: len(a.agents),
		Min:       a.config.MinAgents,
		Max:       a.config.MaxAgents,
	})

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
			Int("reusable", s.ReusableAgents).
			Int("delta", finalDeltas[i]).
			Msg("bucket plan")
	}
	return out
}

// poolLimits bounds the whole pool for one planning cycle.
type poolLimits struct {
	// Footprint is the number of pool agents currently holding a provider
	// slot — every tracked agent, including booting, draining and
	// config-drifted ones. New agents may only be created up to Max minus this,
	// so slow or stale registrations cannot bypass MaxAgents.
	Footprint int
	Min       int
	Max       int
}

// allocateBudget turns each bucket's raw delta into a bounded final delta.
//
// It reasons in terms of a target fleet: the number of agents each bucket
// should have once the global Min/Max caps are applied. Draining always
// proceeds — it frees a provider slot for the next cycle — while scale-up is
// throttled by how many slots MaxAgents still allows given the current
// footprint. That upholds three invariants the earlier greedy budget missed:
//   - MinAgents stays a warm-pool floor even without queue demand;
//   - a full, fixed-size pool drains an idle wrong-capability agent so a
//     demanded capability can take its slot on a later cycle;
//   - booting/draining/drifted agents count against MaxAgents, so slow or
//     stale registrations never push the pool past its ceiling.
//
// Returns one final delta per bucket, in the same order as states.
func allocateBudget(states []bucketState, rawDeltas []int, limits poolLimits) []int {
	n := len(states)

	// required[i] is the agent count this bucket's load demands
	// (ceil(load/WPA)); never negative.
	required := make([]int, n)
	for i := range states {
		required[i] = max(states[i].PoolAgents+rawDeltas[i], 0)
	}

	// desired[i] is the target agent count per bucket: demand first, then the
	// global floor and ceiling applied against the total.
	desired := make([]int, n)
	copy(desired, required)
	total := 0
	for _, d := range desired {
		total += d
	}

	// Serve the busiest buckets first; ties keep provider order (stable).
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		return required[order[i]] > required[order[j]]
	})

	// MinAgents warm-pool floor: top the target up even without demand, adding
	// warm agents to the busiest bucket (the first bucket when the pool idles).
	for n > 0 && total < limits.Min {
		desired[order[0]]++
		total++
	}

	// MaxAgents ceiling: trim the target from the least-busy buckets first.
	for total > limits.Max {
		trimmed := false
		for i := n - 1; i >= 0; i-- {
			idx := order[i]
			if desired[idx] > 0 {
				desired[idx]--
				total--
				trimmed = true
				break
			}
		}
		if !trimmed {
			break
		}
	}

	// Turn targets into deltas. Drains apply in full. Reactivating a matching
	// NoSchedule agent reuses its existing provider slot, while fresh creates
	// share the slots MaxAgents still allows, busiest bucket first.
	createBudget := max(limits.Max-limits.Footprint, 0)
	final := make([]int, n)
	for _, idx := range order {
		switch delta := desired[idx] - states[idx].PoolAgents; {
		case delta < 0:
			final[idx] = delta
		case delta > 0:
			reactivate := min(delta, states[idx].ReusableAgents)
			create := min(delta-reactivate, createBudget)
			final[idx] = reactivate + create
			createBudget -= create
		}
	}
	return final
}
