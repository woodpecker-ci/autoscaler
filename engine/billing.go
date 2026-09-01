package engine

import (
	"time"

	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// This file collects the billing-model-dependent teardown policy in one
// place. The engine keeps idle agents around differently depending on how
// the provider charges (see types.BillingModel): per-second billing tears an
// idle agent down as soon as it has been idle long enough, while
// hourly-round-up keeps a paid-for hour warm and only drains inside a small
// window before each hour boundary.

// inTeardownWindow reports whether the agent is currently within the teardown
// window before one of its paid-hour boundaries (anchored at its creation
// time). Agents that have not reported a creation time are never in the window.
func (a *Autoscaler) inTeardownWindow(agent *woodpecker.Agent) bool {
	if agent.Created == 0 {
		return false
	}

	age := time.Since(time.Unix(agent.Created, 0))
	if age < 0 {
		return false
	}

	window := a.config.AgentBillingTeardownMargin + a.config.ReconciliationInterval
	// A window covering a whole hour (or more) means every moment qualifies.
	if window >= time.Hour {
		return true
	}

	return age%time.Hour >= time.Hour-window
}

// idleLongEnough reports whether the agent has gone without work for at least
// the configured idle timeout. It is the per-second-billing eligibility check
// shared by the drain and removal paths.
func (a *Autoscaler) idleLongEnough(agent *woodpecker.Agent) bool {
	return time.Since(time.Unix(agent.LastWork, 0)) >= a.config.AgentIdleTimeout
}
