package scheduler

import (
	"math"

	"github.com/rs/zerolog/log"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/provider"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type SimpleScheduler struct{}

func (*SimpleScheduler) Plan(
	_ []provider.Capability,
	running []woodpecker.Task,
	pending []woodpecker.Task,
	agents []*woodpecker.Agent,
	cfg *config.Config,
) ([]ScaleDecision, error) {
	runningTasks := len(running)
	pendingTasks := len(pending)

	log.Debug().Msgf("queue info: runningTasks = %v pendingTasks = %v", runningTasks, pendingTasks)

	availablePoolAgents := len(agents)
	reqAgents := math.Ceil(float64(pendingTasks+runningTasks) / float64(cfg.WorkflowsPerAgent))

	maxUp := float64(cfg.MaxAgents - availablePoolAgents)
	maxDown := float64(availablePoolAgents - cfg.MinAgents)

	reqPoolAgents := math.Ceil(reqAgents - float64(availablePoolAgents))
	reqPoolAgents = math.Max(reqPoolAgents, -maxDown)
	reqPoolAgents = math.Min(reqPoolAgents, maxUp)

	log.Debug().Msgf("capacity info: poolAgents = %v/%v limits = %v/%v", availablePoolAgents, reqPoolAgents, maxUp, maxDown)

	delta := int(reqPoolAgents)
	if delta == 0 {
		return nil, nil
	}

	return []ScaleDecision{
		{
			Capability: provider.Capability{DeployMethod: provider.CloudInit},
			Delta:      delta,
		},
	}, nil
}
