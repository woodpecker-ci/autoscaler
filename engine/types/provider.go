package types

import (
	"context"

	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// BillingModel describes how a provider charges for an agent's runtime. It
// selects the teardown policy the engine applies to idle agents.
type BillingModel int

const (
	// BillingPerSecond bills by the actual runtime (e.g. AWS, Scaleway). Holding
	// an idle agent open buys nothing, so the engine uses a plain idle timeout.
	// This is the zero value, so providers that do not override it keep the
	// historic behavior.
	BillingPerSecond BillingModel = iota

	// BillingHourlyRoundUp bills whole hours rounded up (e.g. Linode, Hetzner
	// Cloud). A partial hour costs the same as a full one, so the engine keeps
	// idle agents schedulable for the rest of the hour already paid for and only
	// tears them down just before each hour boundary.
	BillingHourlyRoundUp
)

func (b BillingModel) String() string {
	switch b {
	case BillingPerSecond:
		return "per-second"
	case BillingHourlyRoundUp:
		return "hourly-round-up"
	default:
		return "unknown"
	}
}

type Provider interface {
	DeployAgent(context.Context, *woodpecker.Agent, Capability) error
	RemoveAgent(context.Context, *woodpecker.Agent) error
	ListDeployedAgentNames(context.Context) ([]string, error)
	Capabilities(ctx context.Context) ([]Capability, error)

	// BillingModel reports how the provider charges for agent runtime, which
	// selects the engine's teardown policy for idle agents.
	BillingModel() BillingModel
}

// Capability is a single (platform, backend) pair a provider can deploy.
// Platform and Backend match exactly the label keys the woodpecker agent
// self-reports on connect ("platform", "backend").
type Capability struct {
	Platform string
	Backend  Backend
}

type Backend string

const (
	BackendDocker     Backend = "docker"
	BackendLocal      Backend = "local"
	BackendKubernetes Backend = "kubernetes"
)
