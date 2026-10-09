package engine

import (
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/pipeline"
)

// poolScope models the labels the woodpecker server enforces on every agent in
// this autoscaler's pool (see model.Agent.GetServerLabels). Keeping the access
// scope separate from provider capabilities and operator labels lets the pool's
// scope evolve — e.g. towards organization- or user-scoped pools — without
// touching task matching or the planner: only the enforced labels change.
type poolScope struct {
	// enforcedLabels are merged onto every agent's advertised labels. The
	// server applies these itself, so the autoscaler mirrors them to keep real
	// queue tasks routable.
	enforcedLabels map[string]string
}

// instanceAdminPoolScope is the scope of a global, instance-admin pool: the
// server matches its agents against every organization by injecting
// "org-id=*". Organization- or user-scoped pools would instead enforce a
// concrete "org-id" here, leaving the rest of the engine unchanged.
func instanceAdminPoolScope() poolScope {
	return poolScope{
		enforcedLabels: map[string]string{
			pipeline.LabelFilterOrg: labelWildcard,
		},
	}
}

// agentLabelsFor renders the labels an agent in this scope advertises for the
// given capability, with the server-enforced scope labels applied on top of
// the provider/operator labels.
func (s poolScope) agentLabelsFor(capability types.Capability, configuredLabels map[string]string) agentLabelSet {
	labels := agentLabelsFor(capability, configuredLabels)
	for key, value := range s.enforcedLabels {
		labels.Labels[key] = value
	}
	return labels
}
