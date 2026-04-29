package engine

import (
	"strings"

	"go.woodpecker-ci.org/autoscaler/engine/types"
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

// agentLabelSet is the resolved view of what labels a freshly-deployed agent
// would advertise to the woodpecker server. Both the implicit per-capability
// labels (platform, backend) and operator-supplied custom labels are merged
// in, and the "!" prefix on mandatory keys is stripped from the keys (the
// mandatory-ness is tracked separately in Mandatory).
type agentLabelSet struct {
	// Labels is the full key->value map an agent would publish, with any
	// "!" prefix already removed from the keys.
	Labels map[string]string
	// Mandatory is the set of keys that were originally prefixed with "!"
	// in the operator config — those keys MUST be present (and match) on a
	// task for this agent to be allowed to pick it up.
	Mandatory map[string]struct{}
}

// agentLabelsFor constructs the label set a single agent would publish for
// the given provider capability and operator-configured extra labels.
//
// The capability supplies the well-known "platform" and "backend" labels
// (matching exactly what the woodpecker agent self-reports on connect).
// extraAgentLabels are merged on top, with the "!" prefix interpreted as
// "mandatory key" per the woodpecker label rules.
func agentLabelsFor(capability types.Capability, extraAgentLabels map[string]string) agentLabelSet {
	out := agentLabelSet{
		Labels:    make(map[string]string, 2+len(extraAgentLabels)), //nolint:mnd
		Mandatory: make(map[string]struct{}),
	}

	if capability.Platform != "" {
		out.Labels["platform"] = capability.Platform
	}
	if capability.Backend != "" {
		out.Labels["backend"] = string(capability.Backend)
	}

	for k, v := range extraAgentLabels {
		key := k
		if strings.HasPrefix(key, labelMandatoryPrefix) {
			key = strings.TrimPrefix(key, labelMandatoryPrefix)
			out.Mandatory[key] = struct{}{}
		}
		out.Labels[key] = v
	}

	return out
}

// taskMatchesAgent reports whether a workflow task with the given labels
// could be picked up by an agent advertising the given label set.
//
// The matching rules mirror the woodpecker server scheduler:
//   - Empty workflow label values are ignored (per docs).
//   - For every non-empty workflow label, the agent must have a label with
//     the same key, and the agent value must either equal the workflow value
//     or be the wildcard "*".
//   - Every key the agent has marked mandatory ("!" prefix in config) must
//     be present and non-empty in the workflow labels.
func taskMatchesAgent(taskLabels map[string]string, agent agentLabelSet) bool {
	// Mandatory keys: workflow must explicitly set them (with a non-empty
	// value, since empty values are ignored by woodpecker).
	for key := range agent.Mandatory {
		v, ok := taskLabels[key]
		if !ok || v == "" {
			return false
		}
	}

	// Every (non-empty) workflow label must be matched by the agent.
	for key, want := range taskLabels {
		if want == "" {
			continue
		}
		got, ok := agent.Labels[key]
		if !ok {
			return false
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
