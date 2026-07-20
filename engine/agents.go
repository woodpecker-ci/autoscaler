package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"

	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/autoscaler/utils"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// This file owns the lifecycle of a single pool agent: discovering the agents
// that belong to this pool, bringing new ones up (or reactivating drained
// ones), draining idle ones, and removing them once they are safe to delete.
// The higher-level reconcile loop (autoscaler.go) decides how many agents each
// bucket needs; the functions here carry those decisions out.

func (a *Autoscaler) loadAgents(_ context.Context) error {
	a.agents = make(map[string]*woodpecker.Agent)

	agents, err := a.client.AgentList()
	if err != nil {
		return fmt.Errorf("client.AgentList: %w", err)
	}

	prefix := fmt.Sprintf("pool-%s-agent-", a.config.PoolID)
	for _, agent := range agents {
		if strings.HasPrefix(agent.Name, prefix) {
			a.agents[agent.Name] = agent
		}
	}

	return nil
}

// createAgents deploys `amount` new agents into the given bucket.
// It first tries to reactivate matching no-schedule agents
// (which are already provisioned on the provider) before deploying new ones.
func (a *Autoscaler) createAgents(ctx context.Context, bucket agentBucket, amount int) error {
	const suffixLength = 4

	if amount <= 0 {
		return nil
	}

	// First, reactivate matching no-schedule agents.
	reactivated := 0
	for _, agent := range a.agents {
		if reactivated >= amount {
			break
		}
		if !agent.NoSchedule {
			continue
		}
		if !agentMatchesCapability(agent, bucket.Capability) {
			continue
		}

		log.Info().Str("agent", agent.Name).Msg("reactivate agent")
		agent.NoSchedule = false
		if _, err := a.client.AgentUpdate(agent); err != nil {
			return fmt.Errorf("client.AgentUpdate: %w", err)
		}
		reactivated++
	}

	// Deploy fresh agents for whatever's left.
	for i := reactivated; i < amount; i++ {
		agent, err := a.client.AgentCreate(&woodpecker.Agent{
			Name: fmt.Sprintf("pool-%s-agent-%s", a.config.PoolID, utils.RandomString(suffixLength)),
		})
		if err != nil {
			return fmt.Errorf("client.AgentCreate: %w", err)
		}

		log.Info().
			Str("agent", agent.Name).
			Str("platform", bucket.Capability.Platform).
			Str("backend", string(bucket.Capability.Backend)).
			Msg("deploying agent")

		if err := a.provider.DeployAgent(ctx, agent, bucket.Capability); err != nil {
			return fmt.Errorf("types.DeployAgent: %w", err)
		}

		a.agents[agent.Name] = agent
	}

	return nil
}

func (a *Autoscaler) drainAgents(_ context.Context, bucket agentBucket, amount int) error {
	if amount <= 0 {
		return nil
	}
	drained := 0
	for _, agent := range a.agents {
		if drained >= amount {
			break
		}
		// agent is already marked for draining
		if agent.NoSchedule {
			continue
		}
		// only drain agents that belong to this bucket
		if !agentMatchesCapability(agent, bucket.Capability) {
			continue
		}
		// agent has never contacted the server => not ready for draining
		if agent.LastContact == 0 {
			continue
		}

		if a.config.BillingModel == types.BillingHourlyRoundUp {
			// hourly-round-up: the hour is already paid for, so keep the
			// agent schedulable until just before its hour boundary even
			// while idle, then drain it inside the teardown window.
			if !a.inTeardownWindow(agent) {
				continue
			}
		} else if !a.idleLongEnough(agent) {
			// agent has recently done work => not ready for draining
			continue
		}

		log.Info().Str("agent", agent.Name).Msg("drain agent")
		agent.NoSchedule = true
		if _, err := a.client.AgentUpdate(agent); err != nil {
			return fmt.Errorf("client.AgentUpdate: %w", err)
		}
		drained++
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

	// hourly-round-up: recency of work does not gate removal. The paid hour is
	// kept warm by the drain stage; once an agent is eligible for removal the
	// only thing that protects it is an in-flight task (checked above).
	if a.config.BillingModel == types.BillingHourlyRoundUp {
		return true, nil
	}

	// agent has done work recently => not idle
	if !a.idleLongEnough(agent) {
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

	delete(a.agents, agent.Name)

	return nil
}

func (a *Autoscaler) removeDrainedAgents(ctx context.Context) error {
	for _, agent := range a.agents {
		if !agent.NoSchedule {
			continue
		}

		// hourly-round-up: a drained agent that rolled into a fresh paid hour
		// (e.g. it was busy at the boundary) stays up until its next teardown
		// window rather than wasting the hour just bought.
		if a.config.BillingModel == types.BillingHourlyRoundUp && !a.inTeardownWindow(agent) {
			continue
		}

		err := a.removeAgent(ctx, agent, "was drained")
		if err != nil {
			return err
		}
	}

	return nil
}
