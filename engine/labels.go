package engine

import (
	"strings"

	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/pipeline"
)

// labelMandatoryPrefix is the prefix configured operators use on a key in
// ExtraAgentLabels to mark the label as mandatory: such an agent will only
// pick up workflows that explicitly set that label.
//
// See https://woodpecker-ci.org/docs/administration/configuration/agent#agent_labels
const labelMandatoryPrefix = "!"

// labelWildcard is the value an agent can declare for a label to indicate
// that it matches any workflow value for that key.
const labelWildcard = "*"

// agentLabelSet is the resolved view of labels used to match tasks against a
// freshly deployed agent. Normal and mandatory labels are kept apart so a
// pool scope can add server-enforced labels without conflating the two: the
// server can enforce a normal label with the same key as a mandatory one.
type agentLabelSet struct {
	// Labels are the agent's normal labels, with the "!" prefix removed.
	Labels map[string]string
	// Mandatory are labels configured with a "!" prefix, mapped to their
	// required value. A task must carry each key with exactly this value.
	Mandatory map[string]string
}

// agentLabelsFor constructs the label set a single agent would publish for
// the given provider capability and operator-configured extra labels.
//
// The capability supplies the well-known "platform" and "backend" labels
// (matching what a woodpecker agent self-reports on connect), and every agent
// advertises "repo=*" by default — without it, real queue tasks, which always
// carry a concrete "repo" label, would never match. extraAgentLabels are
// merged on top, with the "!" prefix interpreted as a mandatory key per the
// woodpecker label rules.
func agentLabelsFor(capability types.Capability, extraAgentLabels map[string]string) agentLabelSet {
	out := agentLabelSet{
		Labels:    make(map[string]string, 3+len(extraAgentLabels)), //nolint:mnd
		Mandatory: make(map[string]string),
	}

	if capability.Platform != "" {
		out.Labels[pipeline.LabelFilterPlatform] = capability.Platform
	}
	if capability.Backend != "" {
		out.Labels[pipeline.LabelFilterBackend] = string(capability.Backend)
	}
	// Woodpecker agents allow all repos by default (see cmd/agent in
	// woodpecker); the server-side repo label on every task is matched by it.
	out.Labels[pipeline.LabelFilterRepo] = labelWildcard

	for k, v := range extraAgentLabels {
		if strings.HasPrefix(k, labelMandatoryPrefix) {
			out.Mandatory[strings.TrimPrefix(k, labelMandatoryPrefix)] = v
			continue
		}
		out.Labels[k] = v
	}

	return out
}

// taskMatchesAgent reports whether a workflow task with the given labels could
// be picked up by an agent advertising the given label set. It mirrors the
// woodpecker server scheduler (server/scheduler/filter.go):
//   - Every mandatory ("!"-prefixed) key must be present with the exact
//     configured value; this is checked first.
//   - Internal "woodpecker-ci.org/*" labels are ignored.
//   - Empty workflow label values are ignored.
//   - For every remaining workflow label, the agent must advertise the same
//     key (a mandatory key counts too) with a matching value or the wildcard.
//
// Scope labels the server enforces (e.g. "org-id=*" for a global agent) are
// already baked into the agent label set by poolScope.agentLabelsFor, so this
// matcher stays scope-agnostic.
func taskMatchesAgent(taskLabels map[string]string, agent agentLabelSet) bool {
	for key, required := range agent.Mandatory {
		if v, ok := taskLabels[key]; !ok || v != required {
			return false
		}
	}

	for key, want := range taskLabels {
		if strings.HasPrefix(key, pipeline.InternalLabelPrefix) {
			continue
		}
		if want == "" {
			continue
		}
		got, ok := agent.Labels[key]
		if !ok {
			// A mandatory key is matchable too (mirrors the server's "!"+key
			// lookup), so an agent requiring it can still take the task.
			got, ok = agent.Mandatory[key]
			if !ok {
				return false
			}
		}
		if got == labelWildcard {
			continue
		}
		if got != want {
			return false
		}
	}

	return true
}
