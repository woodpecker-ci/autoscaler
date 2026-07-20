package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// This file reconciles drift between the two sources of truth for pool agents:
// the provider (what is actually deployed) and the woodpecker server (what is
// registered). Agents that exist on only one side, or that have gone silent,
// are cleaned up here. This runs independently of the scale up/down decision.

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
