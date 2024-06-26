package engine

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/woodpecker/v2/woodpecker-go/woodpecker"
)

type Autoscaler struct {
	client   woodpecker.Client
	agents   []*woodpecker.Agent
	config   *config.Config
	provider Provider
}

// NewAutoscaler creates a new Autoscaler instance.
// It takes in a Provider, Client and Config, and returns a configured
// Autoscaler struct.
func NewAutoscaler(provider Provider, client woodpecker.Client, config *config.Config) Autoscaler {
	return Autoscaler{
		provider: provider,
		client:   client,
		config:   config,
	}
}

func (a *Autoscaler) loadAgents(_ context.Context) error {
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
	suffixLength := 4

	for i := 0; i < amount; i++ {
		agent, err := a.client.AgentCreate(&woodpecker.Agent{
			Name: fmt.Sprintf("pool-%s-agent-%s", a.config.PoolID, RandomString(suffixLength)),
		})
		if err != nil {
			return fmt.Errorf("client.AgentCreate: %w", err)
		}

		log.Info().Str("agent", agent.Name).Msg("deploying agent")

		err = a.provider.DeployAgent(ctx, agent)
		if err != nil {
			return err
		}

		a.agents = append(a.agents, agent)
	}

	return nil
}

func (a *Autoscaler) drainAgents(_ context.Context, amount int) error {
	for i := 0; i < amount; i++ {
		for _, agent := range a.agents {
			if !agent.NoSchedule && agent.LastContact != 0 {
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

func (a *Autoscaler) isAgentIdle(agent *woodpecker.Agent) (bool, error) {
	tasks, err := a.client.AgentTasksList(agent.ID)
	if err != nil {
		return false, fmt.Errorf("client.AgentTasksList: %w", err)
	}

	return len(tasks) == 0, nil
}

func (a *Autoscaler) removeAgent(ctx context.Context, agent *woodpecker.Agent, reason string) error {
	isIdle, err := a.isAgentIdle(agent)
	if err != nil {
		return err
	}
	if !isIdle {
		log.Info().Str("agent", agent.Name).Msg("agent is still processing workload")
		return nil
	}

	log.Info().Str("agent", agent.Name).Str("reason", reason).Msgf("removing agent")

	err = a.provider.RemoveAgent(ctx, agent)
	if err != nil {
		return err
	}

	err = a.client.AgentDelete(agent.ID)
	if err != nil {
		return fmt.Errorf("client.AgentDelete: %w", err)
	}

	filteredAgents := make([]*woodpecker.Agent, 0)
	for _, a := range a.agents {
		if a.ID != agent.ID {
			filteredAgents = append(filteredAgents, a)
		}
	}
	a.agents = filteredAgents

	return nil
}

func (a *Autoscaler) removeDrainedAgents(ctx context.Context) error {
	for _, agent := range a.getPoolAgents(true) {
		err := a.removeAgent(ctx, agent, "was drained")
		if err != nil {
			return err
		}
	}

	return nil
}

func (a *Autoscaler) cleanupAgents(ctx context.Context) error {
	woodpeckerAgents := a.getPoolAgents(false)
	providerAgentNames, err := a.provider.ListDeployedAgentNames(ctx)
	if err != nil {
		return err
	}

	// remove agents which are not in the woodpecker agent list anymore
	for _, agentName := range providerAgentNames {
		found := false
		for _, agent := range woodpeckerAgents {
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

			// remove agent from providerAgentNames
			_providerAgentNames := make([]string, 0)
			for _, a := range providerAgentNames {
				if a != agentName {
					_providerAgentNames = append(_providerAgentNames, a)
				}
			}
			providerAgentNames = _providerAgentNames
		}
	}

	// remove agents which do not exist on the provider anymore
	for _, agent := range woodpeckerAgents {
		found := false
		for _, agentName := range providerAgentNames {
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

			// remove agent from woodpeckerAgents
			_woodpeckerAgents := make([]*woodpecker.Agent, 0)
			for _, a := range a.agents {
				if a.Name != agent.Name {
					woodpeckerAgents = append(woodpeckerAgents, a)
				}
			}
			a.agents = _woodpeckerAgents
		}
	}

	// remove agents that are stuck in the provisioning phase
	for _, agent := range a.getPoolAgents(false) {
		// we don't need to remove agents that are already marked as NoSchedule
		if agent.NoSchedule {
			continue
		}

		if time.Since(time.Unix(agent.Created, 0)) > a.config.AgentAllowedStartupTime {
			err := a.removeAgent(ctx, agent, "never connected in time")
			if err != nil {
				return err
			}
		}
	}

	// remove agents that haven't contacted the server for a long time
	for _, agent := range a.getPoolAgents(false) {
		// agent hasn't even contacted the server so we don't need to check for inactivity
		if agent.LastContact == 0 {
			continue
		}

		if time.Since(time.Unix(agent.LastContact, 0)) > a.config.AgentInactivityTimeout {
			err := a.removeAgent(ctx, agent, "hasn't been seen for a while")
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (a *Autoscaler) getQueueInfo(_ context.Context) (freeTasks, runningTasks, pendingTasks int, err error) {
	queueInfo, err := a.client.QueueInfo()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("error from QueueInfo: %s", err.Error())
	}

	if a.config.FilterLabels == "" {
		return queueInfo.Stats.Workers, queueInfo.Stats.Running, queueInfo.Stats.Pending, nil
	}

	labelFilterKey, labelFilterValue, ok := strings.Cut(a.config.FilterLabels, "=")
	if !ok {
		return 0, 0, 0, fmt.Errorf("invalid labels filter: %s", a.config.FilterLabels)
	}

	running := countTasksByLabel(queueInfo.Running, labelFilterKey, labelFilterValue)
	pending := countTasksByLabel(queueInfo.Pending, labelFilterKey, labelFilterValue)

	return queueInfo.Stats.Workers, running, pending, nil
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

// Reconcile periodically checks the status of the agent pool and adjusts it to match
// the desired capacity based on the current queue state.
func (a *Autoscaler) Reconcile(ctx context.Context) {
	if err := a.loadAgents(ctx); err != nil {
		log.Error().Err(err).Msg("load agents failed")
		return
	}

	reqPoolAgents, err := a.calcAgents(ctx)
	if err != nil {
		log.Error().Err(err).Msg("calculating agents failed")
		return
	}

	if reqPoolAgents > 0 {
		log.Debug().Msgf("starting %f additional agents", reqPoolAgents)

		if err := a.createAgents(ctx, int(reqPoolAgents)); err != nil {
			log.Error().Err(err).Msg("creating agents failed")
			return
		}
	}

	if reqPoolAgents < 0 {
		num := int(math.Abs(reqPoolAgents))

		log.Debug().Msgf("trying to stop %d agents", num)
		if err := a.drainAgents(ctx, num); err != nil {
			log.Error().Err(err).Msg("draining agents failed")
			return
		}
	}

	if err := a.cleanupAgents(ctx); err != nil {
		log.Error().Err(err).Msg("cleanup of agents failed")
		return
	}

	if err := a.removeDrainedAgents(ctx); err != nil {
		log.Error().Err(err).Msg("removing drained agents failed")
		return
	}
}
