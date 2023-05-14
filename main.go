package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"regexp"
	"strings"
	"time"

	_ "github.com/joho/godotenv/autoload"
	"github.com/urfave/cli/v2"
	"github.com/woodpecker-ci/woodpecker/woodpecker-go/woodpecker"

	"github.com/woodpecker-ci/autoscaler/config"
	"github.com/woodpecker-ci/autoscaler/provider"
)

type Autoscaler struct {
	client   woodpecker.Client
	agents   []*woodpecker.Agent
	config   *config.Config
	provider provider.Provider
}

func (a *Autoscaler) getLoad(ctx context.Context) (freeTasks, runningTasks, pendingTasks int, err error) {
	info, err := a.client.QueueInfo()
	if err != nil {
		return -1, -1, -1, err
	}

	return info.Stats.Workers, info.Stats.Running, info.Stats.Pending + info.Stats.WaitingOnDeps, nil
}

func (a *Autoscaler) loadAgents(ctx context.Context) error {
	a.agents = []*woodpecker.Agent{}

	agents, err := a.client.AgentList()
	if err != nil {
		return err
	}
	r, _ := regexp.Compile(fmt.Sprintf("pool-%d-agent-.*?", a.config.PoolID))

	for _, agent := range agents {
		if r.MatchString(agent.Name) {
			a.agents = append(a.agents, agent)
		}
	}

	return nil
}

func (a *Autoscaler) getActiveAgents() []*woodpecker.Agent {
	activeAgents := make([]*woodpecker.Agent, 0)
	for _, agent := range a.agents {
		if !agent.NoSchedule {
			activeAgents = append(activeAgents, agent)
		}
	}
	return activeAgents
}

func (a *Autoscaler) createAgents(ctx context.Context, amount int) error {
	for i := 0; i < amount; i++ {
		agent, err := a.client.AgentCreate(&woodpecker.Agent{
			Name: fmt.Sprintf("pool-%d-agent-%s", a.config.PoolID, RandomString(4)),
		})
		if err != nil {
			// TODO: only log error
			return err
		}

		log.Println("Deploying agent", agent.Name, "...")

		err = a.provider.DeployAgent(ctx, agent)
		if err != nil {
			// TODO: only log error
			return err
		}

		log.Println("Deployed agent", agent.Name)

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
					// TODO: only log error
					return err
				}
				log.Println("Draining agent", agent.Name, "...")
				break
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
				log.Println("Agent is still executing workflows", agent.Name)
				continue
			}

			log.Println("Removing agent", agent.Name, "...")

			err = a.provider.RemoveAgent(ctx, agent)
			if err != nil {
				return err
			}

			err = a.client.AgentDelete(agent.ID)
			if err != nil {
				return err
			}

			log.Println("Removed agent", agent.Name)

			a.agents = append(a.agents[:0], a.agents[1:]...)
		}
	}

	return nil
}

func (a *Autoscaler) cleanupAgents(ctx context.Context) error {
	registeredAgents := a.getActiveAgents()
	deployedAgentNames, err := a.provider.ListDeployedAgentNames(ctx)
	if err != nil {
		return err
	}

	// remove agents which do not exist on the provider anymore
	for _, agentName := range deployedAgentNames {
		found := false
		for _, agent := range registeredAgents {
			if agent.Name == agentName {
				found = true
				break
			}
		}

		if !found {
			log.Println("Removing agent", agentName, "...")
			err = a.provider.RemoveAgent(ctx, &woodpecker.Agent{Name: agentName})
			if err != nil {
				return err
			}
			log.Println("Removed agent", agentName)
		}
	}

	// remove agents which are not in the agent list anymore
	for _, agent := range registeredAgents {
		found := false
		for _, agentName := range deployedAgentNames {
			if agent.Name == agentName {
				found = true
				break
			}
		}

		if !found {
			log.Println("Removing agent", agent.Name, "...")
			err = a.client.AgentDelete(agent.ID)
			if err != nil {
				return err
			}
			log.Println("Removed agent", agent.Name)
		}
	}

	// TODO: remove stale agents

	return nil
}

func (a *Autoscaler) Reconcile(ctx context.Context) error {
	err := a.loadAgents(ctx)
	if err != nil {
		return err
	}

	freeTasks, runningTasks, pendingTasks, err := a.getLoad(ctx)
	if err != nil {
		return err
	}

	availableAgents := math.Ceil(float64(freeTasks+runningTasks) / float64((a.config.WorkflowsPerAgent)))
	neededAmountAgents := float64(pendingTasks) / float64(a.config.WorkflowsPerAgent)

	amountActiveAgentsFromPool := len(a.getActiveAgents())
	maxUp := float64(a.config.MaxAgents - amountActiveAgentsFromPool)
	maxDown := float64(amountActiveAgentsFromPool - a.config.MinAgents)

	diffAmountAgents := math.Ceil(neededAmountAgents - availableAgents)
	diffAmountAgents = math.Max(diffAmountAgents, -maxDown)
	diffAmountAgents = math.Min(diffAmountAgents, maxUp)

	if diffAmountAgents > 0 {
		log.Println("Starting", diffAmountAgents, "additional agents ...")
		return a.createAgents(ctx, int(diffAmountAgents))
	}

	if diffAmountAgents < 0 {
		log.Println("Stopping", diffAmountAgents, "agents ...")
		err := a.drainAgents(ctx, int(math.Abs(float64(diffAmountAgents))))
		if err != nil {
			return err
		}
	}

	err = a.cleanupAgents(ctx)
	if err != nil {
		return err
	}

	return a.removeDrainedAgents(ctx)
}

func setupProvider(c *cli.Context, config *config.Config) (provider.Provider, error) {
	if c.String("provider") == "hetzner" {
		return &provider.Hetzner{
			ApiToken:   c.String("hetzner-api-token"),
			Location:   c.String("hetzner-location"),
			ServerType: c.String("hetzner-server-type"),
			Config:     config,
		}, nil
	}

	return nil, fmt.Errorf("unknown provider: %s", c.String("provider"))
}

func run(c *cli.Context) error {
	client, err := NewClient(c)
	if err != nil {
		return err
	}

	agentEnvironment := make(map[string]string)
	for _, env := range c.StringSlice("agent-env") {
		parts := strings.Split(env, "=")
		if len(parts) != 2 {
			return fmt.Errorf("invalid agent environment variable: %s", env)
		}
		agentEnvironment[parts[0]] = parts[1]
	}

	config := &config.Config{
		MinAgents:         c.Int("min-agents"),
		MaxAgents:         c.Int("max-agents"),
		WorkflowsPerAgent: c.Int("workflows-per-agent"),
		PoolID:            c.Int("pool-id"),
		GRPCAddress:       c.String("grpc-addr"),
		GRPCSecure:        c.Bool("grpc-secure"),
		Image:             c.String("image"),
		Environment:       agentEnvironment,
	}

	provider, err := setupProvider(c, config)
	if err != nil {
		return err
	}

	err = provider.Setup()
	if err != nil {
		return err
	}

	autoscaler := &Autoscaler{
		provider: provider,
		client:   client,
		config:   config,
	}

	const interval = time.Second * 1
	for {
		select {
		case <-c.Done():
			return nil
		case <-time.After(interval):
			if err := autoscaler.Reconcile(c.Context); err != nil {
				return err
			}
		}
	}
}

func main() {
	app := &cli.App{
		Name:  "autoscaler",
		Usage: "scale to the moon and back",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "pool-id",
				Value: 1,
				Usage: "an id of the pool to scale",
			},
			&cli.IntFlag{
				Name:    "min-agents",
				Value:   1,
				Usage:   "the minimum amount of agents",
				EnvVars: []string{"WOODPECKER_MIN_AGENTS"},
			},
			&cli.IntFlag{
				Name:    "max-agents",
				Value:   10,
				Usage:   "the maximum amount of agents",
				EnvVars: []string{"WOODPECKER_MAX_AGENTS"},
			},
			&cli.IntFlag{
				Name:    "workflows-per-agent",
				Value:   2,
				Usage:   "max workflows an agent will executed in parallel",
				EnvVars: []string{"WOODPECKER_WORKFLOWS_PER_AGENT"},
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
				Name:    "grpc-addr",
				Value:   "woodpecker-server:9000",
				Usage:   "the grpc address of the woodpecker server",
				EnvVars: []string{"WOODPECKER_GRPC_ADDR"},
			},
			&cli.BoolFlag{
				Name:    "grpc-secure",
				Value:   false,
				Usage:   "use a secure grpc connection to the woodpecker server",
				EnvVars: []string{"WOODPECKER_GRPC_SECURE"},
			},
			&cli.StringFlag{
				Name:    "provider",
				Value:   "hetzner",
				Usage:   "the provider to use",
				EnvVars: []string{"WOODPECKER_PROVIDER"},
			},
			&cli.StringFlag{
				Name:    "agent-image",
				Value:   "woodpeckerci/woodpecker-agent:next",
				Usage:   "the agent image to use",
				EnvVars: []string{"WOODPECKER_AGENT_IMAGE"},
			},
			&cli.StringSliceFlag{
				Name:    "agent-env",
				Usage:   "additional agent environment variables",
				EnvVars: []string{"WOODPECKER_AGENT_ENV"},
			},

			// hetzner
			&cli.StringFlag{
				Name:    "hetzner-api-token",
				Usage:   "the hetzner api token",
				EnvVars: []string{"WOODPECKER_HETZNER_API_TOKEN"},
			},
			&cli.StringFlag{
				Name:    "hetzner-location",
				Value:   "nbg1",
				Usage:   "the hetzner location",
				EnvVars: []string{"WOODPECKER_HETZNER_LOCATION"},
			},
			&cli.StringFlag{
				Name:    "hetzner-server-type",
				Value:   "cx11",
				Usage:   "the hetzner server type",
				EnvVars: []string{"WOODPECKER_HETZNER_SERVER_TYPE"},
			},
		},
		Action: run,
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
