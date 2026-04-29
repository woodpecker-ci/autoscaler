package engine

import (
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

// bucketState holds the per-bucket numbers the planner needs: how much
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
