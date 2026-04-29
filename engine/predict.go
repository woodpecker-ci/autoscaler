package engine

import (
	"math"
	"sort"

	"github.com/rs/zerolog/log"

	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// agentBucket is one (capability, label-set) slot the autoscaler can deploy
// into. Each bucket is rendered from one provider capability merged with
// the global ExtraAgentLabels — agents we deploy for a bucket all advertise
// the same labels, so they pick up the same set of pending tasks.
type agentBucket struct {
	Capability types.Capability
	Labels     agentLabelSet
}

// bucketState holds the per-bucket numbers planScaling needs: how much
// work is queued for it and how many agents it currently has.
type bucketState struct {
	Bucket     agentBucket
	Pending    int
	Running    int
	PoolAgents int // online (NoSchedule=false) pool agents matching this bucket
}

// bucketDecision is the per-bucket scaling decision Reconcile acts on.
type bucketDecision struct {
	Bucket agentBucket
	Delta  int // positive = scale up, negative = drain
}

// agentBuckets renders the buckets the autoscaler can deploy into, in
// stable order: capabilities first as advertised by the provider, each
// merged with the configured ExtraAgentLabels.
func (a *Autoscaler) agentBuckets() []agentBucket {
	if len(a.providerCapabilities) == 0 {
		return nil
	}
	out := make([]agentBucket, 0, len(a.providerCapabilities))
	for _, c := range a.providerCapabilities {
		out = append(out, agentBucket{
			Capability: c,
			Labels:     agentLabelsFor(c, a.config.ExtraAgentLabels),
		})
	}
	return out
}

// routeTaskToBucket returns the index of the first bucket that could pick
// the task up, or -1 if no configured bucket can run it. Tasks routed to
// -1 are unschedulable on this autoscaler — there is no point spinning up
// agents for them.
func routeTaskToBucket(task woodpecker.Task, buckets []agentBucket) int {
	for i, b := range buckets {
		if taskMatchesAgent(task.Labels, b.Labels) {
			return i
		}
	}
	return -1
}

// matchAgentToBucket returns the index of the bucket whose capability
// matches an existing agent's reported (platform, backend) pair, or -1
// if none. We match on woodpecker-reported (platform, backend) rather
// than custom labels because that pair is a property of the machine and
// stable across config changes; CustomLabels can drift if the operator
// edits ExtraAgentLabels between reconciles.
func matchAgentToBucket(agent *woodpecker.Agent, buckets []agentBucket) int {
	for i, b := range buckets {
		if agent.Platform == b.Capability.Platform &&
			agent.Backend == string(b.Capability.Backend) {
			return i
		}
	}
	return -1
}

// computeBucketStates routes pending tasks, running tasks, and existing
// pool agents to their buckets. Tasks that match no bucket are returned
// in unmatchedPending/unmatchedRunning and don't influence scaling.
func computeBucketStates(
	buckets []agentBucket,
	pending, running []woodpecker.Task,
	poolAgents map[string]*woodpecker.Agent,
) (states []bucketState, unmatchedPending, unmatchedRunning int) {
	states = make([]bucketState, len(buckets))
	for i, b := range buckets {
		states[i].Bucket = b
	}

	for _, t := range pending {
		i := routeTaskToBucket(t, buckets)
		if i < 0 {
			unmatchedPending++
			continue
		}
		states[i].Pending++
	}
	for _, t := range running {
		i := routeTaskToBucket(t, buckets)
		if i < 0 {
			unmatchedRunning++
			continue
		}
		states[i].Running++
	}
	for _, ag := range poolAgents {
		if ag.NoSchedule {
			continue
		}
		i := matchAgentToBucket(ag, buckets)
		if i < 0 {
			// Agent doesn't match any current bucket (e.g. config change).
			// Leave it alone — the regular idle/stale paths will drain it.
			continue
		}
		states[i].PoolAgents++
	}
	return states, unmatchedPending, unmatchedRunning
}

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

	// Compute the unbounded per-bucket deltas first.
	rawDeltas := make([]int, len(states))
	for i := range states {
		rawDeltas[i] = rawDelta(states[i], a.config.WorkflowsPerAgent)
	}

	// Apply the global Min/Max caps. Distribute the remaining
	// scale-up/scale-down budget across buckets, prioritizing the ones
	// with the largest absolute demand so the most-loaded buckets are
	// served first.
	totalOnline := 0
	for _, s := range states {
		totalOnline += s.PoolAgents
	}
	upBudget := a.config.MaxAgents - totalOnline
	if upBudget < 0 {
		upBudget = 0
	}
	downBudget := totalOnline - a.config.MinAgents
	if downBudget < 0 {
		downBudget = 0
	}

	order := make([]int, len(states))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		return abs(rawDeltas[order[i]]) > abs(rawDeltas[order[j]])
	})

	finalDeltas := make([]int, len(states))
	for _, idx := range order {
		raw := rawDeltas[idx]
		switch {
		case raw > 0:
			grant := raw
			if grant > upBudget {
				grant = upBudget
			}
			finalDeltas[idx] = grant
			upBudget -= grant
		case raw < 0:
			// We can drain at most |raw| agents from this bucket, at most
			// this bucket's own PoolAgents (can't drain what isn't there),
			// and at most the global downBudget.
			want := -raw
			if want > states[idx].PoolAgents {
				want = states[idx].PoolAgents
			}
			if want > downBudget {
				want = downBudget
			}
			finalDeltas[idx] = -want
			downBudget -= want
		}
	}

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

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
