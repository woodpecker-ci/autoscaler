package provider

import (
	"context"

	"github.com/woodpecker-ci/woodpecker/woodpecker-go/woodpecker"
)

type Provider interface {
	Init() error
	DeployAgent(context.Context, *woodpecker.Agent) error
	RemoveAgent(context.Context, *woodpecker.Agent) error
}
