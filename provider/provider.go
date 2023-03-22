package provider

import (
	"context"

	"github.com/woodpecker-ci/woodpecker/server/model"
)

type Provider interface {
	Init() error
	DeployAgent(context.Context, *model.Agent) error
	RemoveAgent(context.Context, *model.Agent) error
}
