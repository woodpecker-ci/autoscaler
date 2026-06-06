package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/autoscaler/server"
	"go.woodpecker-ci.org/autoscaler/utils"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type Autoscaler struct {
	client               server.Client
	agents               map[string]*woodpecker.Agent
	config               *config.Config
	provider             types.Provider
	providerCapabilities []types.Capability
}

// NewAutoscaler creates a new Autoscaler instance.
// It takes in a Provider, Client and Config, and returns a configured
// Autoscaler struct.
func NewAutoscaler(p types.Provider, client server.Client, config *config.Config) Autoscaler {
	return Autoscaler{
		provider: p,
		client:   client,
		config:   config,
		agents:   make(map[string]*woodpecker.Agent),
	}
}

func (a *Autoscaler) GetCaps(ctx context.Context) (err error) {
	a.providerCapabilities, err = a.provider.Capabilities(ctx)
	return err
}

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

// drainAgents marks up to `amount` agents in the given bucket as
// no-schedule so woodpecker stops dispatching new tasks to them.
// An agent is only drained if it's been idle long enough and has actually
// connected to the server before.
func (a *Autoscaler) drainAgents(_ context.Context, bucket agentBucket, amount int) error {
	if amount <= 0 {
		return nil
	}
	drained := 0
	for _, agent := range a.agents {
		if drained >= amount {
			break
		}
		if agent.NoSchedule {
			continue
		}
		if !agentMatchesCapability(agent, bucket.Capability) {
			continue
		}
		if time.Since(time.Unix(agent.LastWork, 0)) < a.config.AgentIdleTimeout {
			continue
		}
		if agent.LastContact == 0 {
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

	delete(a.agents, agent.Name)

	return nil
}

func (a *Autoscaler) removeDrainedAgents(ctx context.Context) error {
	for _, agent := range a.agents {
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
	providerAgentNames, err := a.provider.ListDeployedAgentNames(ctx)
	if err != nil {
		return err
	}

	// Build the provider-side set up front so the two reconciliation
	// directions below are independent of each other.
	providerSet := make(map[string]struct{}, len(providerAgentNames))
	for _, name := range providerAgentNames {
		providerSet[name] = struct{}{}
	}

	// On provider but not on woodpecker → tear down on the provider.
	for name := range providerSet {
		if _, ok := a.agents[name]; ok {
			continue
		}
		log.Info().Str("agent", name).Str("reason", "not found on woodpecker").Msg("remove agent")
		if err := a.provider.RemoveAgent(ctx, &woodpecker.Agent{Name: name}); err != nil {
			return fmt.Errorf("types.RemoveAgent: %w", err)
		}
	}

	// On woodpecker but not on provider → delete on woodpecker.
	for name, agent := range a.agents {
		if _, ok := providerSet[name]; ok {
			continue
		}
		log.Info().Str("agent", name).Str("reason", "not found on provider").Msg("remove agent")
		if err := a.client.AgentDelete(agent.ID); err != nil {
			return fmt.Errorf("client.AgentDelete: %w", err)
		}
		delete(a.agents, name)
	}

	return nil
}

func (a *Autoscaler) cleanupStaleAgents(ctx context.Context) error {
	// remove agents that haven't contacted the server for a while (including agents that never contacted the server)
	for _, agent := range a.agents {
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

// Reconcile periodically checks the status of the agent pool and adjusts
// it to match the desired capacity based on the current queue state.
//
// The decision is per-bucket: each provider capability merged with the
// configured ExtraAgentLabels is one bucket, and we ask the planner how
// much each bucket needs to scale up or down. Tasks that no bucket can
// serve are excluded from the math — spinning up agents that can't run
// them wouldn't help.
func (a *Autoscaler) Reconcile(ctx context.Context) error {
	if err := a.loadAgents(ctx); err != nil {
		return fmt.Errorf("loading agents failed: %w", err)
	}

	queueInfo, err := a.client.QueueInfo()
	if err != nil {
		return fmt.Errorf("loading queue info failed: %w", err)
	}
	log.Debug().
		Int("pending", len(queueInfo.Pending)).
		Int("running", len(queueInfo.Running)).
		Msg("queue snapshot")

	// planScaling already logs the per-bucket plan at debug level — we
	// just dispatch.
	for _, d := range a.planScaling(queueInfo.Pending, queueInfo.Running) {
		var err error
		switch {
		case d.Delta > 0:
			err = a.createAgents(ctx, d.Bucket, d.Delta)
		case d.Delta < 0:
			err = a.drainAgents(ctx, d.Bucket, -d.Delta)
		}
		if err != nil {
			return fmt.Errorf("scaling bucket %s/%s: %w",
				d.Bucket.Capability.Platform, d.Bucket.Capability.Backend, err)
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
