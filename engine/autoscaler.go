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
	"go.woodpecker-ci.org/autoscaler/server"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type Autoscaler struct {
	client   server.Client
	agents   []*woodpecker.Agent
	config   *config.Config
	provider Provider
}

// NewAutoscaler creates a new Autoscaler instance.
// It takes in a Provider, Client and Config, and returns a configured
// Autoscaler struct.
func NewAutoscaler(provider Provider, client server.Client, config *config.Config) Autoscaler {
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

func (a *Autoscaler) getPoolAgents(excludeNoSchedule bool) []*woodpecker.Agent {
	agents := make([]*woodpecker.Agent, 0)
	for _, agent := range a.agents {
		if excludeNoSchedule && agent.NoSchedule {
			continue
		}
		agents = append(agents, agent)
	}
	return agents
}

func (a *Autoscaler) createAgents(ctx context.Context, amount int) error {
	suffixLength := 4

	reactivatedAgents := 0

	// try to re-activate agents that are in no-schedule state
	for i := 0; i < amount; i++ {
		for _, agent := range a.agents {
			if agent.NoSchedule {
				log.Info().Str("agent", agent.Name).Msg("reactivate agent")
				agent.NoSchedule = false
				_, err := a.client.AgentUpdate(agent)
				if err != nil {
					return fmt.Errorf("client.AgentUpdate: %w", err)
				}
				reactivatedAgents++
			}
		}
	}

	// create new agents
	for i := 0; i < amount-reactivatedAgents; i++ {
		agent, err := a.client.AgentCreate(&woodpecker.Agent{
			Name: fmt.Sprintf("pool-%s-agent-%s", a.config.PoolID, RandomString(suffixLength)),
		})
		if err != nil {
			return fmt.Errorf("client.AgentCreate: %w", err)
		}

		log.Info().Str("agent", agent.Name).Msg("deploying agent")

		err = a.provider.DeployAgent(ctx, agent)
		if err != nil {
			return fmt.Errorf("provider.DeployAgent: %w", err)
		}

		a.agents = append(a.agents, agent)
	}

	return nil
}

func (a *Autoscaler) drainAgents(_ context.Context, amount int) error {
	for i := 0; i < amount; i++ {
		for _, agent := range a.agents {
			// agent is already marked for draining
			if agent.NoSchedule {
				continue
			}

			// agent has recently done work => not ready for draining
			if time.Since(time.Unix(agent.LastWork, 0)) < a.config.AgentIdleTimeout {
				continue
			}

			// agent has never contacted the server => not ready for draining
			if agent.LastContact == 0 {
				continue
			}

			log.Info().Str("agent", agent.Name).Msg("drain agent")
			agent.NoSchedule = true
			_, err := a.client.AgentUpdate(agent)
			if err != nil {
				return fmt.Errorf("client.AgentUpdate: %w", err)
			}
			break
		}
	}

	return nil
}

func (a *Autoscaler) isAgentIdle(agent *woodpecker.Agent) (bool, error) {
	tasks, err := a.client.AgentTasksList(agent.ID)
	if err != nil {
		return false, fmt.Errorf("client.AgentTasksList: %w", err)
	}

	// agent still has tasks => not idle
	if len(tasks) > 0 {
		return false, nil
	}

	// agent has done work recently => not idle
	if time.Since(time.Unix(agent.LastWork, 0)) < a.config.AgentIdleTimeout {
		return false, nil
	}

	return true, nil
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
	for _, agent := range a.getPoolAgents(false) {
		if !agent.NoSchedule {
			continue
		}

		err := a.removeAgent(ctx, agent, "was drained")
		if err != nil {
			return err
		}
	}

	return nil
}

func (a *Autoscaler) cleanupDanglingAgents(ctx context.Context) error {
	woodpeckerAgents := a.getPoolAgents(false)
	providerAgentNames, err := a.provider.ListDeployedAgentNames(ctx)
	if err != nil {
		return err
	}

	// remove agents that are not in the woodpecker agent list anymore
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
				return fmt.Errorf("provider.RemoveAgent: %w", err)
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

	// remove agents that do not exist on the provider anymore
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

	return nil
}

func (a *Autoscaler) cleanupStaleAgents(ctx context.Context) error {
	// remove agents that haven't contacted the server for a while (including agents that never contacted the server)
	for _, agent := range a.getPoolAgents(false) {
		if agent.NoSchedule {
			continue
		}

		lastContact := agent.LastContact

		// if agent has never contacted the server, use the creation time
		if lastContact == 0 {
			lastContact = agent.Created
		}

		if time.Since(time.Unix(lastContact, 0)) > a.config.AgentInactivityTimeout {
			err := a.removeAgent(ctx, agent, "hasn't connected to the server for a while")
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
func (a *Autoscaler) Reconcile(ctx context.Context) error {
	if err := a.loadAgents(ctx); err != nil {
		return fmt.Errorf("loading agents failed: %w", err)
	}

	reqPoolAgents, err := a.calcAgents(ctx)
	if err != nil {
		return fmt.Errorf("calculating agents failed: %w", err)
	}

	if reqPoolAgents > 0 {
		num := int(math.Abs(reqPoolAgents))
		log.Debug().Msgf("starting %d additional agents", num)

		if err := a.createAgents(ctx, num); err != nil {
			return fmt.Errorf("creating agents failed: %w", err)
		}
	}

	if reqPoolAgents < 0 {
		num := int(math.Abs(reqPoolAgents))

		log.Debug().Msgf("checking %d agents if ready for draining", num)
		if err := a.drainAgents(ctx, num); err != nil {
			return fmt.Errorf("draining agents failed: %w", err)
		}
	}

	// cleanup agents that are only present at the provider or woodpecker
	if err := a.cleanupDanglingAgents(ctx); err != nil {
		return fmt.Errorf("cleaning up dangling agents failed: %w", err)
	}

	// cleanup agents that haven't contacted the server for a while
	if err := a.cleanupStaleAgents(ctx); err != nil {
		return fmt.Errorf("cleaning up stale agents failed: %w", err)
	}

	// remove agents that are drained
	if err := a.removeDrainedAgents(ctx); err != nil {
		return fmt.Errorf("removing drained agents failed: %w", err)
	}

	return nil
}
