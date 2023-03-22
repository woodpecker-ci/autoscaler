package main

import (
	"context"
	"math"
	"time"

	"go.einride.tech/pid"

	"github.com/woodpecker-ci/autoscaler/provider"
	"github.com/woodpecker-ci/woodpecker/server/model"
)

type Autoscaler struct {
	agents            []*model.Agent
	minAgents         int
	maxAgents         int
	workflowsPerAgent int
	pidCtrl           pid.Controller
	provider          provider.Provider
}

func (a *Autoscaler) getLoad(ctx context.Context) (freeWorkers, pendingTasks int) {
	// TODO: api get load
	return 0, 0
}

func (a *Autoscaler) loadAgents(ctx context.Context) error {
	a.agents = []*model.Agent{}
	return nil
}

func (a *Autoscaler) getActiveAgents() int {
	activeAgents := 0
	for _, agent := range a.agents {
		if !agent.Disabled {
			activeAgents++
		}
	}
	return activeAgents
}

func (a *Autoscaler) createAgents(ctx context.Context, _amount int) error {
	amount := int(math.Min(float64(_amount+a.getActiveAgents()), float64(a.maxAgents)))

	for i := 0; i < amount; i++ {
		// TODO: api create agent
		agent := &model.Agent{}

		err := a.provider.DeployAgent(ctx, agent)
		if err != nil {
			return err
		}

		a.agents = append(a.agents, agent)
	}

	return nil
}

func (a *Autoscaler) drainAgents(ctx context.Context, _amount int) error {
	amount := int(math.Max(float64(a.getActiveAgents()-_amount), float64(a.minAgents)))

	for i := 0; i < amount; i++ {
		for _, agent := range a.agents {
			if !agent.Disabled {
				agent.Disabled = true
				// TODO: api update agent
			}
		}
	}

	return nil
}

func (a *Autoscaler) removeDrainedAgents(ctx context.Context) error {
	for _, agent := range a.agents {
		if agent.Disabled {
			// TODO: check if agents is running workflows

			// TODO: api remove agent if no running workflows

			err := a.provider.RemoveAgent(ctx, agent)
			if err != nil {
				return err
			}

			// TODO: api remove agent

			a.agents = append(a.agents[:0], a.agents[1:]...)
		}
	}

	return nil
}

func (a *Autoscaler) Reconcile(ctx context.Context) error {
	freeWorkers, pendingTasks := a.getLoad(ctx)

	neededAmount := float64(pendingTasks) / float64(a.workflowsPerAgent)
	actualAmount := float64(freeWorkers)

	a.pidCtrl.Update(pid.ControllerInput{
		ReferenceSignal:  neededAmount,
		ActualSignal:     actualAmount,
		SamplingInterval: 1000 * time.Millisecond,
	})

	diffAmount := a.pidCtrl.State.ControlSignal
	if diffAmount > 0 {
		return a.createAgents(ctx, int(math.Floor(diffAmount)))
	}

	if diffAmount < 0 {
		err := a.drainAgents(ctx, int(math.Floor(diffAmount)))
		if err != nil {
			return err
		}
	}

	return a.removeDrainedAgents(ctx)
}

func main() {
	ctx := context.Background()

	provider := &provider.Hetzner{}
	err := provider.Init()
	if err != nil {
		panic(err)
	}

	autoscaler := &Autoscaler{
		minAgents:         1,
		maxAgents:         10,
		workflowsPerAgent: 2,
		pidCtrl: pid.Controller{
			Config: pid.ControllerConfig{
				ProportionalGain: 2.0,
				IntegralGain:     1.0,
				DerivativeGain:   1.0,
			},
		},
		provider: provider,
	}

	for {
		if err := autoscaler.Reconcile(ctx); err != nil {
			panic(err)
		}
		time.Sleep(1000 * time.Millisecond)
	}
}
