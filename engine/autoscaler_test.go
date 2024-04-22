package engine

import (
	"context"
	"testing"

	"github.com/franela/goblin"
	"github.com/rs/zerolog"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/woodpecker/v2/woodpecker-go/woodpecker"
)

type MockClient struct {
	workers       int
	running       int
	pending       int
	waitingOnDeps int
	woodpecker.Client
}

func (m MockClient) QueueInfo() (*woodpecker.Info, error) {
	info := &woodpecker.Info{}

	info.Stats.Workers = m.workers
	info.Stats.Running = m.running
	info.Stats.Pending = m.pending
	info.Stats.WaitingOnDeps = m.waitingOnDeps

	info.Pending = []woodpecker.Task{
		{
			Labels: map[string]string{
				"arch": "amd64",
			},
		},
	}

	return info, nil
}

func Test_calcAgents(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	g := goblin.Goblin(t)

	g.Describe("Agent creation", func() {
		g.It("Should create new agent (MinAgents)", func() {
			autoscaler := Autoscaler{client: &MockClient{
				pending: 0,
			}, config: &config.Config{
				WorkflowsPerAgent: 1,
				MaxAgents:         2,
				MinAgents:         1,
			}}

			value, _ := autoscaler.calcAgents(context.TODO())
			g.Assert(value).Equal(float64(1))
		})

		g.It("Should create single agent", func() {
			autoscaler := Autoscaler{client: &MockClient{
				pending: 2,
			}, config: &config.Config{
				WorkflowsPerAgent: 5,
				MaxAgents:         3,
			}}

			value, _ := autoscaler.calcAgents(context.TODO())
			g.Assert(value).Equal(float64(1))
		})

		g.It("Should create multiple agents", func() {
			autoscaler := Autoscaler{client: &MockClient{
				pending: 6,
			}, config: &config.Config{
				WorkflowsPerAgent: 5,
				MaxAgents:         3,
			}}

			value, _ := autoscaler.calcAgents(context.TODO())
			g.Assert(value).Equal(float64(2))
		})

		g.It("Should create new agent (MaxAgents)", func() {
			autoscaler := Autoscaler{client: &MockClient{
				pending: 2,
			}, config: &config.Config{
				WorkflowsPerAgent: 1,
				MaxAgents:         2,
			}, agents: []*woodpecker.Agent{
				{Name: "pool-1-agent-1234"},
			}}

			value, _ := autoscaler.calcAgents(context.TODO())
			g.Assert(value).Equal(float64(1))
		})

		g.It("Should not create new agent (availableAgents)", func() {
			autoscaler := Autoscaler{client: &MockClient{
				workers: 2,
				pending: 2,
			}, config: &config.Config{
				WorkflowsPerAgent: 1,
				MaxAgents:         2,
			}}

			value, _ := autoscaler.calcAgents(context.TODO())
			g.Assert(value).Equal(float64(0))
		})
	})
}

func Test_getQueueInfo(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	g := goblin.Goblin(t)

	g.Describe("Queue Info", func() {
		g.It("Should not filter", func() {
			autoscaler := Autoscaler{
				client: &MockClient{
					pending: 2,
				},
				config: &config.Config{},
			}

			free, running, pending, _ := autoscaler.getQueueInfo(context.TODO())
			g.Assert(free).Equal(0)
			g.Assert(running).Equal(0)
			g.Assert(pending).Equal(2)
		})
		g.It("Should filter one by label", func() {
			autoscaler := Autoscaler{
				client: &MockClient{
					pending: 2,
				},
				config: &config.Config{
					LabelsFilter: "arch=amd64",
				},
			}

			free, running, pending, _ := autoscaler.getQueueInfo(context.TODO())
			g.Assert(free).Equal(0)
			g.Assert(running).Equal(0)
			g.Assert(pending).Equal(1)
		})
		g.It("Should filter all by label", func() {
			autoscaler := Autoscaler{
				client: &MockClient{
					pending: 2,
				},
				config: &config.Config{
					LabelsFilter: "arch=arm64",
				},
			}

			free, running, pending, _ := autoscaler.getQueueInfo(context.TODO())
			g.Assert(free).Equal(0)
			g.Assert(running).Equal(0)
			g.Assert(pending).Equal(0)
		})
	})
}
