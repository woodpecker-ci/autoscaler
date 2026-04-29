package engine

import (
	"context"
	"fmt"
	"regexp"
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
	agents               []*woodpecker.Agent
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
	}
}

func (a *Autoscaler) GetCaps(ctx context.Context) (err error) {
	a.providerCapabilities, err = a.provider.Capabilities(ctx)
	return err
}

func (a *Autoscaler) loadAgents(_ context.Context) error {
	a.agents = []*woodpecker.Agent{}

	agents, err := a.client.AgentList()
	if err != nil {
		return fmt.Errorf("client.AgentList: %w", err)
	}
	r, err := regexp.Compile(fmt.Sprintf("pool-%s-agent-.*?", a.config.PoolID))
	if err != nil {
		return fmt.Errorf("could not create regex matcher for agent names by pool ID: %w", err)
	}

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

// createAgents deploys `amount` new agents into the given bucket. It first
// tries to reactivate matching no-schedule agents (which are already
// provisioned on the provider) before deploying new ones, since
// reactivation is much cheaper than spinning up a new VM.
//
// "Matching" means an agent whose woodpecker-reported (platform, backend)
// matches the bucket's capability — those agents already advertise the
// right base labels and will pick up the same workflows once reactivated.
func (a *Autoscaler) createAgents(ctx context.Context, bucket agentBucket, amount int) error {
	const suffixLength = 4

	if amount <= 0 {
		return nil
	}

	// First, reactivate matching no-schedule agents. The legacy version of
	// this loop walked all no-schedule agents on every outer iteration and
	// reactivated all of them regardless of `amount`; we cap at `amount`.
	reactivated := 0
	for _, agent := range a.agents {
		if reactivated >= amount {
			break
		}
		if !agent.NoSchedule {
			continue
		}
		if matchAgentToBucket(agent, []agentBucket{bucket}) < 0 {
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

		a.agents = append(a.agents, agent)
	}

	return nil
}

// drainAgents marks up to `amount` agents in the given bucket as
// no-schedule so woodpecker stops dispatching new tasks to them. An agent
// is only drained if it's been idle long enough and has actually
// connected to the server before — same readiness rules as the legacy
// flow, just scoped to one bucket.
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
		if matchAgentToBucket(agent, []agentBucket{bucket}) < 0 {
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
				return fmt.Errorf("types.RemoveAgent: %w", err)
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

// queueSnapshot is the full task-level view of the queue used for
// bucket-aware scheduling. It carries the actual pending/running task
// lists (so we can match each task's labels against an agent bucket) plus
// the global free-worker count reported by the server.
type queueSnapshot struct {
	Free    int
	Pending []woodpecker.Task
	Running []woodpecker.Task
}

// loadQueueSnapshot fetches the queue and applies any operator-configured
// FilterLabels filter, returning the surviving pending and running tasks
// alongside the free-worker count.
//
// FilterLabels is a coarse pre-filter retained for backwards compat;
// fine-grained per-bucket filtering happens later in planScaling using
// each bucket's full label set.
func (a *Autoscaler) loadQueueSnapshot(_ context.Context) (queueSnapshot, error) {
	queueInfo, err := a.client.QueueInfo()
	if err != nil {
		return queueSnapshot{}, fmt.Errorf("error from QueueInfo: %s", err.Error())
	}

	pending := queueInfo.Pending
	running := queueInfo.Running

	if a.config.FilterLabels != "" {
		key, value, ok := strings.Cut(a.config.FilterLabels, "=")
		if !ok {
			return queueSnapshot{}, fmt.Errorf("invalid labels filter: %s", a.config.FilterLabels)
		}
		pending = filterTasksByLabel(pending, key, value)
		running = filterTasksByLabel(running, key, value)
	}

	return queueSnapshot{
		Free:    queueInfo.Stats.Workers,
		Pending: pending,
		Running: running,
	}, nil
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

	snap, err := a.loadQueueSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("loading queue snapshot failed: %w", err)
	}
	log.Debug().
		Int("free", snap.Free).
		Int("pending", len(snap.Pending)).
		Int("running", len(snap.Running)).
		Msg("queue snapshot")

	for _, d := range a.planScaling(snap) {
		if d.Delta > 0 {
			log.Debug().
				Str("platform", d.Bucket.Capability.Platform).
				Str("backend", string(d.Bucket.Capability.Backend)).
				Int("count", d.Delta).
				Msg("starting additional agents")
			if err := a.createAgents(ctx, d.Bucket, d.Delta); err != nil {
				return fmt.Errorf("creating agents failed: %w", err)
			}
		} else {
			log.Debug().
				Str("platform", d.Bucket.Capability.Platform).
				Str("backend", string(d.Bucket.Capability.Backend)).
				Int("count", -d.Delta).
				Msg("checking agents for draining")
			if err := a.drainAgents(ctx, d.Bucket, -d.Delta); err != nil {
				return fmt.Errorf("draining agents failed: %w", err)
			}
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

func filterTasksByLabel(jobs []woodpecker.Task, labelKey, labelValue string) []woodpecker.Task {
	out := make([]woodpecker.Task, 0, len(jobs))
	for _, job := range jobs {
		if val, ok := job.Labels[labelKey]; ok && val == labelValue {
			out = append(out, job)
		}
	}
	return out
}
