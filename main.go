package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"regexp"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/woodpecker-ci/woodpecker/woodpecker-go/woodpecker"
	"go.einride.tech/pid"

	"github.com/woodpecker-ci/autoscaler/provider"
)

type Autoscaler struct {
	client            woodpecker.Client
	agents            []*woodpecker.Agent
	poolID            int
	minAgents         int
	maxAgents         int
	workflowsPerAgent int
	pidCtrl           pid.Controller
	provider          provider.Provider
}

func (a *Autoscaler) getLoad(ctx context.Context) (freeWorker, pendingTasks int, err error) {
	info, err := a.client.QueueInfo()
	if err != nil {
		return 0, 0, err
	}

	return info.Stats.Workers, info.Stats.Pending + info.Stats.WaitingOnDeps, nil
}

func (a *Autoscaler) loadAgents(ctx context.Context) error {
	a.agents = []*woodpecker.Agent{}

	agents := []*woodpecker.Agent{}
	r, _ := regexp.Compile(fmt.Sprintf("pool-%d-agent-.*?", a.poolID))

	for _, agent := range agents {
		if r.MatchString(agent.Name) {
			a.agents = append(a.agents, agent)
		}
	}

	return nil
}

func (a *Autoscaler) getActiveAgents() int {
	activeAgents := 0
	for _, agent := range a.agents {
		if !agent.NoSchedule {
			activeAgents++
		}
	}
	return activeAgents
}

func (a *Autoscaler) createAgents(ctx context.Context, _amount int) error {
	amount := int(math.Min(float64(_amount+a.getActiveAgents()), float64(a.maxAgents)))

	for i := 0; i < amount; i++ {
		agent, err := a.client.AgentCreate(&woodpecker.Agent{
			Name: fmt.Sprintf("pool-%d-agent-%d", a.poolID, i),
		})
		if err != nil {
			return err
		}

		err = a.provider.DeployAgent(ctx, agent)
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
			if !agent.NoSchedule {
				agent.NoSchedule = true
				_, err := a.client.AgentUpdate(agent)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (a *Autoscaler) isAgentRunningWorkflows(agent *woodpecker.Agent) bool {
	info, err := a.client.QueueInfo()
	if err != nil {
		return false
	}

	for _, task := range info.Running {
		if task.AgentID == agent.ID {
			return true
		}
	}

	return false
}

func (a *Autoscaler) removeDrainedAgents(ctx context.Context) error {
	for _, agent := range a.agents {
		if agent.NoSchedule {
			if a.isAgentRunningWorkflows(agent) {
				continue
			}

			err := a.provider.RemoveAgent(ctx, agent)
			if err != nil {
				return err
			}

			err = a.client.AgentDelete(agent.ID)
			if err != nil {
				return err
			}

			a.agents = append(a.agents[:0], a.agents[1:]...)
		}
	}

	return nil
}

func (a *Autoscaler) Reconcile(ctx context.Context) error {
	err := a.loadAgents(ctx)
	if err != nil {
		return err
	}

	freeWorkers, pendingTasks, err := a.getLoad(ctx)
	if err != nil {
		return err
	}

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

func run(ctx *cli.Context) error {
	client, err := NewClient(ctx)
	if err != nil {
		return err
	}

	provider := &provider.Hetzner{
		ApiToken: "token123",
	}
	err = provider.Init()
	if err != nil {
		return err
	}

	autoscaler := &Autoscaler{
		poolID:            ctx.Int("pool-id"),
		minAgents:         ctx.Int("min-agents"),
		maxAgents:         ctx.Int("max-agents"),
		workflowsPerAgent: ctx.Int("workflows-per-agent"),
		pidCtrl: pid.Controller{
			Config: pid.ControllerConfig{
				ProportionalGain: 2.0,
				IntegralGain:     1.0,
				DerivativeGain:   1.0,
			},
		},
		provider: provider,
		client:   client,
	}

	for {
		if err := autoscaler.Reconcile(ctx.Context); err != nil {
			return err
		}
		time.Sleep(1000 * time.Millisecond)
	}

}

func main() {
	app := &cli.App{
		Name:  "autoscaler",
		Usage: "make an explosive entrance",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "pool-id",
				Value: 1,
				Usage: "an id of the pool to scale",
			},
			&cli.IntFlag{
				Name:  "min-agents",
				Value: 1,
				Usage: "the minimum amount of agents",
			},
			&cli.IntFlag{
				Name:  "max-agents",
				Value: 10,
				Usage: "the maximum amount of agents",
			},
			&cli.IntFlag{
				Name:  "workflows-per-agent",
				Value: 2,
				Usage: "max workflows an agent will executed in parallel",
			},
		},
		Action: run,
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
