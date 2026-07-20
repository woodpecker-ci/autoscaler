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

	// Attribute agents that have not connected yet to the capability they
	// were deployed for, so the planner counts them as capacity while they
	// boot. Once an agent reports its own identity — or is gone — the
	// deploy record has served its purpose and is dropped.
	for name, capability := range a.pendingDeploys {
		agent, ok := a.agents[name]
		if !ok || agent.Platform != "" || agent.Backend != "" {
			delete(a.pendingDeploys, name)
			continue
		}
		agent.Platform = capability.Platform
		agent.Backend = string(capability.Backend)
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

		// Remember what this agent was deployed for; until it connects and
		// reports its own identity, the planner attributes it to this bucket
		// (see loadAgents) instead of re-provisioning the same demand.
		if a.pendingDeploys == nil {
			a.pendingDeploys = make(map[string]types.Capability)
		}
		a.pendingDeploys[agent.Name] = bucket.Capability
		agent.Platform = bucket.Capability.Platform
		agent.Backend = string(bucket.Capability.Backend)

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
		// only drain agents that belong to this bucket
		if !agentMatchesCapability(agent, bucket.Capability) {
			continue
		}
		if !a.agentReadyForDrain(agent) {
			continue
		}
		if err := a.markAgentForDrain(agent); err != nil {
			return err
		}
		drained++
	}

	return nil
}

// agentReadyForDrain reports whether an idle agent may be drained now: it must
// be schedulable, have contacted the server at least once, and — depending on
// the billing model — be idle past the timeout or inside its teardown window.
func (a *Autoscaler) agentReadyForDrain(agent *woodpecker.Agent) bool {
	// never contacted the server, or already draining => not ready
	if agent.NoSchedule || agent.LastContact == 0 {
		return false
	}
	if a.config.BillingModel == types.BillingHourlyRoundUp {
		// hourly-round-up: the hour is already paid for, so keep the agent
		// schedulable until just before its hour boundary even while idle,
		// then drain it inside the teardown window.
		return a.inTeardownWindow(agent)
	}
	return a.idleLongEnough(agent)
}

// markAgentForDrain flags an agent NoSchedule on the woodpecker server so it
// stops picking up work and can later be removed.
func (a *Autoscaler) markAgentForDrain(agent *woodpecker.Agent) error {
	log.Info().Str("agent", agent.Name).Msg("drain agent")
	agent.NoSchedule = true
	if _, err := a.client.AgentUpdate(agent); err != nil {
		return fmt.Errorf("client.AgentUpdate: %w", err)
	}
	return nil
}

// drainUnmatchedAgents drains idle agents whose capability no longer maps to
// any current bucket (e.g. a provider config change). Such agents would
// otherwise hold a provider slot against MaxAgents and block a replacement
// with a needed capability. An empty bucket list — no known capabilities, e.g.
// a failed provider query — is treated as "unknown", not "drain everything".
func (a *Autoscaler) drainUnmatchedAgents(buckets []agentBucket) error {
	if len(buckets) == 0 {
		return nil
	}
	for _, agent := range a.agents {
		if matchAgentToBucket(agent, buckets) >= 0 || !a.agentReadyForDrain(agent) {
			continue
		}
		if err := a.markAgentForDrain(agent); err != nil {
			return err
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
