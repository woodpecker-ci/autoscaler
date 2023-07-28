package engine

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"time"

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

// nolint:revive
func (a *Autoscaler) getQueueInfo(ctx context.Context) (freeTasks, runningTasks, pendingTasks int, err error) {
	info, err := a.client.QueueInfo()
	if err != nil {
		return -1, -1, -1, fmt.Errorf("client.QueueInfo: %w", err)
	}

	return info.Stats.Workers, info.Stats.Running, info.Stats.Pending + info.Stats.WaitingOnDeps, nil
}

// nolint:revive
func (a *Autoscaler) loadAgents(ctx context.Context) error {
	a.agents = []*woodpecker.Agent{}

	agents, err := a.client.AgentList()
	if err != nil {
		return fmt.Errorf("client.AgentList: %w", err)
	}
	r, _ := regexp.Compile(fmt.Sprintf("pool-%s-agent-.*?", a.config.PoolID))

	for _, agent := range agents {
		if r.MatchString(agent.Name) {
			a.agents = append(a.agents, agent)
		}
	}

	return nil
}

func (a *Autoscaler) getPoolAgents(excludeDrained bool) []*woodpecker.Agent {
	agents := make([]*woodpecker.Agent, 0)
	for _, agent := range a.agents {
		if excludeDrained && agent.NoSchedule {
			continue
		}
		agents = append(agents, agent)
	}
	return agents
}

func (a *Autoscaler) createAgents(ctx context.Context, amount int) error {
	for i := 0; i < amount; i++ {
		agent, err := a.client.AgentCreate(&woodpecker.Agent{
			Name: fmt.Sprintf("pool-%s-agent-%s", a.config.PoolID, RandomString(4)),
		})
		if err != nil {
			return fmt.Errorf("client.AgentCreate: %w", err)
		}

		log.Info().Str("agent", agent.Name).Msg("deploy agent")

		err = a.provider.DeployAgent(ctx, agent)
		if err != nil {
			return err
		}

		a.agents = append(a.agents, agent)
	}

	return nil
}

// nolint:revive
func (a *Autoscaler) drainAgents(ctx context.Context, amount int) error {
	now := time.Now()

	for i := 0; i < amount; i++ {
		for _, agent := range a.agents {
			agentStartupExpectedUntil := time.Unix(agent.Created, 0).Add(a.config.AgentAllowedStartupTime)

			if !agent.NoSchedule && agentStartupExpectedUntil.Before(now) {
				log.Info().Str("agent", agent.Name).Msg("drain agent")
				agent.NoSchedule = true
				_, err := a.client.AgentUpdate(agent)
				if err != nil {
					return fmt.Errorf("client.AgentUpdate: %w", err)
				}
				break
			}
		}
	}

	return nil
}

func (a *Autoscaler) AgentIdle(agent *woodpecker.Agent) (bool, error) {
	tasks, err := a.client.AgentTasksList(agent.ID)
	if err != nil {
		return false, fmt.Errorf("client.AgentTasksList: %w", err)
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
				log.Info().Str("agent", agent.Name).Msg("agent is processing workload")
				continue
			}

			log.Info().Str("agent", agent.Name).Msgf("remove agent")

			err = a.provider.RemoveAgent(ctx, agent)
			if err != nil {
				return err
			}

			err = a.client.AgentDelete(agent.ID)
			if err != nil {
				return fmt.Errorf("client.AgentDelete: %w", err)
			}

			a.agents = append(a.agents[:0], a.agents[1:]...)
		}
	}

	return nil
}

func (a *Autoscaler) removeDetachedAgents(ctx context.Context) error {
	registeredAgents := a.getPoolAgents(false)
	deployedAgentNames, err := a.provider.ListDeployedAgentNames(ctx)
	if err != nil {
		return err
	}

	// remove agents which are not in the agent list anymore
	for _, agentName := range deployedAgentNames {
		found := false
		for _, agent := range registeredAgents {
			if agent.Name == agentName {
				found = true
				break
			}
		}

		if !found {
			log.Info().Str("agent", agentName).Str("reason", "not found on woodpecker").Msg("remove agent")
			if err := a.provider.RemoveAgent(ctx, &woodpecker.Agent{Name: agentName}); err != nil {
				return err
			}
		}
	}

	// remove agents which do not exist on the provider anymore
	for _, agent := range registeredAgents {
		found := false
		for _, agentName := range deployedAgentNames {
			if agent.Name == agentName {
				found = true
				break
			}
		}

		if !found {
			log.Info().Str("agent", agent.Name).Str("reason", "not found on provider").Msg("remove agent")
			if err = a.client.AgentDelete(agent.ID); err != nil {
				return fmt.Errorf("client.AgentDelete: %w", err)
			}
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

	log.Debug().Msgf("queue info: freeTasks = %v runningTasks = %v pendingTasks = %v", freeTasks, runningTasks, pendingTasks)
	availableAgents := math.Ceil(float64(freeTasks+runningTasks) / float64((a.config.WorkflowsPerAgent)))
	reqAgents := math.Ceil(float64(pendingTasks+runningTasks) / float64(a.config.WorkflowsPerAgent))

	availablePoolAgents := len(a.getPoolAgents(true))
	maxUp := float64(a.config.MaxAgents - availablePoolAgents)
	maxDown := float64(availablePoolAgents - a.config.MinAgents)

	reqPoolAgents := math.Ceil(reqAgents - (availableAgents + float64(availablePoolAgents)))
	reqPoolAgents = math.Max(reqPoolAgents, -maxDown)
	reqPoolAgents = math.Min(reqPoolAgents, maxUp)

	log.Debug().Msgf("capacity info: agents = %v/%v pool = %v/%v limits = %v/%v", availableAgents, reqAgents, availablePoolAgents, reqPoolAgents, maxUp, maxDown)

	return reqPoolAgents, nil
}

func (a *Autoscaler) Reconcile(ctx context.Context) error {
	if err := a.loadAgents(ctx); err != nil {
		log.Error().Err(err).Msg("load agents failed")
		return nil
	}

	reqPoolAgents, err := a.calcAgents(ctx)
	if err != nil {
		log.Error().Err(err).Msg("calculate agents failed")
		return nil
	}

	if reqPoolAgents > 0 {
		log.Debug().Msgf("start %v additional agents", reqPoolAgents)

		if err := a.createAgents(ctx, int(reqPoolAgents)); err != nil {
			log.Error().Err(err).Msgf("create agents failed")
			return nil
		}
	}

	if reqPoolAgents < 0 {
		num := int(math.Abs(float64(reqPoolAgents)))

		log.Debug().Msgf("try to stop %v agents", num)
		if err := a.drainAgents(ctx, num); err != nil {
			log.Error().Err(err).Msg("remove agents failed")
			return nil
		}
	}

	if err := a.removeDetachedAgents(ctx); err != nil {
		log.Error().Err(err).Msg("remove agents failed")
		return nil
	}

	if err := a.removeDrainedAgents(ctx); err != nil {
		log.Error().Err(err).Msg("remove agents failed")
		return nil
	}

	return nil
}
