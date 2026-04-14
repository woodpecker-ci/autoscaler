package scheduler

import (
	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/provider"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// ScaleDecision describes how many agents of a specific capability to
// add (positive) or drain (negative).
type ScaleDecision struct {
	Capability provider.Capability
	Delta      int // >0 = create, <0 = drain
}

// Scheduler decides how to scale the pool given current state.
// Pluggable so future heuristic strategies can be added without
// changing the engine.
type Scheduler interface {
	Plan(
		caps []provider.Capability,
		running []woodpecker.Task,
		pending []woodpecker.Task,
		agents []*woodpecker.Agent,
		cfg *config.Config,
	) ([]ScaleDecision, error)
}
