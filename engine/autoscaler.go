package engine

import (
	"context"
	"fmt"
	"math"
	"regexp"

	"github.com/rs/zerolog/log"
	"github.com/woodpecker-ci/autoscaler/config"
	"github.com/woodpecker-ci/woodpecker/woodpecker-go/woodpecker"
)

type Autoscaler struct {
	client   woodpecker.Client
	agents   []*woodpecker.Agent
	config   *config.Config
	provider Provider
}

func NewAutoscaler(provider Provider, client woodpecker.Client, config *config.Config) Autoscaler {
	return Autoscaler{
		provider: provider,
		client:   client,
		config:   config,
	}
}

func (a *Autoscaler) getQueueInfo(ctx context.Context) (freeTasks, runningTasks, pendingTasks int, err error) {
	info, err := a.client.QueueInfo()
	if err != nil {
		return -1, -1, -1, err
	}

	return info.Stats.Workers, info.Stats.Running, info.Stats.Pending + info.Stats.WaitingOnDeps, nil
}

func (a *Autoscaler) loadAgents(ctx context.Context) error {
	a.agents = []*woodpecker.Agent{}

	agents, err := a.client.AgentList()
	if err != nil {
		return err
	}
	r, _ := regexp.Compile(fmt.Sprintf("pool-%s-agent-.*?", a.config.PoolID))

	for _, agent := range agents {
		if r.MatchString(agent.Name) {
			a.agents = append(a.agents, agent)
		}
	}

	return nil
}

func (a *Autoscaler) getPoolAgents() []*woodpecker.Agent {
	agents := make([]*woodpecker.Agent, 0)
	for _, agent := range a.agents {
		if !agent.NoSchedule {
			agents = append(agents, agent)
		}
	}
	return agents
}

func (a *Autoscaler) createAgents(ctx context.Context, amount int) error {
	for i := 0; i < amount; i++ {
		agent, err := a.client.AgentCreate(&woodpecker.Agent{
			Name: fmt.Sprintf("pool-%s-agent-%s", a.config.PoolID, RandomString(4)),
		})
		if err != nil {
			return err
		}

		log.Info().Msgf("deploying agent: %s", agent.Name)

		err = a.provider.DeployAgent(ctx, agent)
		if err != nil {
			return err
		}

		a.agents = append(a.agents, agent)
	}

	return nil
}

func (a *Autoscaler) drainAgents(ctx context.Context, amount int) error {
	for i := 0; i < amount; i++ {
		for _, agent := range a.agents {
			if !agent.NoSchedule {
				agent.NoSchedule = true
				_, err := a.client.AgentUpdate(agent)
				if err != nil {
					return err
				}
				log.Info().Msgf("draining agent: %s", agent.Name)
				break
			}
		}
	}

	return nil
}

func (a *Autoscaler) AgentIdle(agent *woodpecker.Agent) (bool, error) {
	tasks, err := a.client.AgentTasksList(agent.ID)
	if err != nil {
		return false, err
	}

	return len(tasks) == 0, nil
}

func (a *Autoscaler) removeDrainedAgents(ctx context.Context) error {
	for _, agent := range a.agents {
		if agent.NoSchedule {
			isIdle, err := a.AgentIdle(agent)
			if err != nil {
				return err
			}
			if !isIdle {
				log.Info().Msgf("agent is still executing workflows: %s", agent.Name)
				continue
			}

			log.Info().Msgf("removing agent: %s", agent.Name)

			err = a.provider.RemoveAgent(ctx, agent)
			if err != nil {
				return err
			}

			err = a.client.AgentDelete(agent.ID)
			if err != nil {
				return err
			}

			log.Info().Msgf("removed agent: %s", agent.Name)

			a.agents = append(a.agents[:0], a.agents[1:]...)
		}
	}

	return nil
}

func (a *Autoscaler) cleanupAgents(ctx context.Context) error {
	registeredAgents := a.getPoolAgents()
	deployedAgentNames, err := a.provider.ListDeployedAgentNames(ctx)
	if err != nil {
		return err
	}

	// remove agents which do not exist on the provider anymore
	for _, agentName := range deployedAgentNames {
		found := false
		for _, agent := range registeredAgents {
			if agent.Name == agentName {
				found = true
				break
			}
		}

		if !found {
			log.Info().Msgf("removing agent: %s", agentName)
			err = a.provider.RemoveAgent(ctx, &woodpecker.Agent{Name: agentName})
			if err != nil {
				return err
			}
			log.Info().Msgf("removed agent: %s", agentName)
		}
	}

	// remove agents which are not in the agent list anymore
	for _, agent := range registeredAgents {
		found := false
		for _, agentName := range deployedAgentNames {
			if agent.Name == agentName {
				found = true
				break
			}
		}

		if !found {
			log.Info().Msgf("removing agent: %s", agent.Name)
			err = a.client.AgentDelete(agent.ID)
			if err != nil {
				return err
			}
			log.Info().Msgf("removed agent: %s", agent.Name)
		}
	}

	// TODO: remove stale agents

	return nil
}

func (a *Autoscaler) calcAgents(ctx context.Context) (float64, error) {
	freeTasks, runningTasks, pendingTasks, err := a.getQueueInfo(ctx)
	if err != nil {
		return 0, err
	}

	log.Debug().Msgf("freeTasks: %v runningTasks: %v pendingTasks: %v", freeTasks, runningTasks, pendingTasks)
	availableAgents := math.Ceil(float64(freeTasks+runningTasks) / float64((a.config.WorkflowsPerAgent)))
	reqAgents := math.Ceil(float64(pendingTasks) / float64(a.config.WorkflowsPerAgent))

	availablePoolAgents := len(a.getPoolAgents())
	maxUp := float64(a.config.MaxAgents - availablePoolAgents)
	maxDown := float64(availablePoolAgents - a.config.MinAgents)

	reqPoolAgents := math.Ceil(reqAgents - (availableAgents + float64(availablePoolAgents)))
	reqPoolAgents = math.Max(reqPoolAgents, -maxDown)
	reqPoolAgents = math.Min(reqPoolAgents, maxUp)

	log.Debug().Msgf("availableAgents: %v reqAgents: %v availablePoolAgents: %v maxUp: %v maxDown: %v", availableAgents, reqAgents, availablePoolAgents, maxUp, maxDown)

	return reqPoolAgents, nil
}

func (a *Autoscaler) Reconcile(ctx context.Context) error {
	err := a.loadAgents(ctx)
	if err != nil {
		return err
	}

	reqPoolAgents, err := a.calcAgents(ctx)
	if err != nil {
		return err
	}

	if reqPoolAgents > 0 {
		log.Info().Msgf("starting additional agents: %v", reqPoolAgents)
		return a.createAgents(ctx, int(reqPoolAgents))
	}

	if reqPoolAgents < 0 {
		drainAgents := int(math.Abs(float64(reqPoolAgents)))
		log.Info().Msgf("stopping agents: %v", drainAgents)
		err := a.drainAgents(ctx, drainAgents)
		if err != nil {
			return err
		}
	}

	err = a.cleanupAgents(ctx)
	if err != nil {
		return err
	}

	return a.removeDrainedAgents(ctx)
}
