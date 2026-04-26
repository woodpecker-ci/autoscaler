package types

import (
	"context"

	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type Provider interface {
	DeployAgent(context.Context, *woodpecker.Agent) error
	RemoveAgent(context.Context, *woodpecker.Agent) error
	ListDeployedAgentNames(context.Context) ([]string, error)
}
