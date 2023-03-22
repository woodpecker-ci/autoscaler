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

	agents, err := a.client.AgentList()
	if err != nil {
		return err
	}
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

func (a *Autoscaler) createAgents(ctx context.Context, amount int) error {
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

func (a *Autoscaler) drainAgents(ctx context.Context, amount int) error {
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

func (a *Autoscaler) isAgentRunningWorkflows(agent *woodpecker.Agent) (bool, error) {
	tasks, err := a.client.AgentTasksList(agent.ID)
	if err != nil {
		return false, err
	}

	return len(tasks) > 0, nil
}

func (a *Autoscaler) removeDrainedAgents(ctx context.Context) error {
	for _, agent := range a.agents {
		if agent.NoSchedule {
			isRunningWorkflows, err := a.isAgentRunningWorkflows(agent)
			if err != nil {
				return err
			}
			if isRunningWorkflows {
				continue
			}

			err = a.provider.RemoveAgent(ctx, agent)
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

	activeAgents := a.getActiveAgents()
	diffAmount := math.Floor(a.pidCtrl.State.ControlSignal / float64(a.workflowsPerAgent))
	diffAmount = math.Min(diffAmount, float64(a.maxAgents-activeAgents))
	diffAmount = math.Max(diffAmount, float64(a.minAgents-activeAgents))

	if diffAmount > 0 {
		return a.createAgents(ctx, int(diffAmount))
	}

	if diffAmount < 0 {
		err := a.drainAgents(ctx, int(diffAmount))
		if err != nil {
			return err
		}
	}

	return a.removeDrainedAgents(ctx)
}

func run(c *cli.Context) error {
	client, err := NewClient(c)
	if err != nil {
		return err
	}

	provider := &provider.Hetzner{
		ApiToken: c.String("hetzner-api-token"),
	}
	err = provider.Init()
	if err != nil {
		return err
	}

	autoscaler := &Autoscaler{
		poolID:            c.Int("pool-id"),
		minAgents:         c.Int("min-agents"),
		maxAgents:         c.Int("max-agents"),
		workflowsPerAgent: c.Int("workflows-per-agent"),
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
		if err := autoscaler.Reconcile(c.Context); err != nil {
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
			&cli.StringFlag{
				Name:    "server",
				Value:   "http://localhost:8000",
				Usage:   "the woodpecker server address",
				EnvVars: []string{"WOODPECKER_SERVER"},
			},
			&cli.StringFlag{
				Name:    "token",
				Usage:   "the woodpecker api token",
				EnvVars: []string{"WOODPECKER_TOKEN"},
			},
			&cli.StringFlag{
				Name:  "socks-proxy",
				Usage: "the socks proxy address",
			},
			&cli.BoolFlag{
				Name:  "socks-proxy-off",
				Usage: "disable the socks proxy",
			},
			&cli.BoolFlag{
				Name:  "skip-verify",
				Usage: "skip ssl verification",
			},

			// hetzner
			&cli.StringFlag{
				Name:    "hetzner-api-token",
				Usage:   "the hetzner api token",
				EnvVars: []string{"WOODPECKER_HETZNER_API_TOKEN"},
			},
		},
		Action: run,
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
