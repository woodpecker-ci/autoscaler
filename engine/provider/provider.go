package provider

import (
	"context"

	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type Provider interface {
	DeployAgent(context.Context, *woodpecker.Agent, Capability) error
	RemoveAgent(context.Context, *woodpecker.Agent) error
	ListDeployedAgentNames(context.Context) ([]string, error)
	Capabilities(ctx context.Context) ([]Capability, error)
}

// Capability is a single (platform, backend) pair a provider can deploy,
// plus the internal method used to bootstrap it.
// Platform and Backend match exactly the label keys the woodpecker agent
// self-reports on connect ("platform", "backend").
type Capability struct {
	Platform     string
	Backend      Backend
	DeployMethod InitMethod
}

type Backend string

const (
	BackendDocker     Backend = "docker"
	BackendLocal      Backend = "local"
	BackendKubernetes Backend = "kubernetes"
)

type InitMethod string

const CloudInit InitMethod = "cloudinit"
