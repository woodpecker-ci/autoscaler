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

// Provider is the cloud side of the autoscaler: it turns the engine's
// decisions into machines. Every method is scoped to a single pool
// (config.PoolID) and is called from the reconcile loop, so any error aborts
// the rest of that cycle and is retried on the next one. Implementations must
// therefore tolerate being re-run against a half-finished state.
type Provider interface {
	// DeployAgent provisions one machine for the given agent and capability
	// and returns once it has been handed to the cloud API. The agent has
	// already been registered on the woodpecker server, so agent.Name is the
	// machine's identity: RemoveAgent and ListDeployedAgentNames must be able
	// to find it by that name again, and the machine must be labeled or
	// tagged so it is recognizable as belonging to this pool.
	//
	// The capability is one the provider itself reported via Capabilities.
	// An implementation that cannot serve it must return an error rather than
	// silently deploy something else — the engine counts the agent against
	// the demand of that capability, so a substitute would leave the demand
	// unserved while occupying a slot of the MaxAgents budget.
	//
	// The agent boots asynchronously and reports its own labels when it first
	// connects; DeployAgent does not wait for that.
	DeployAgent(context.Context, *woodpecker.Agent, Capability) error

	// RemoveAgent tears down the machine belonging to the given agent.
	//
	// Only agent.Name is guaranteed to be set: drift cleanup passes a
	// synthetic agent for a machine the woodpecker server no longer knows.
	//
	// It must be idempotent — an already-gone machine is success, not an
	// error — because the engine deletes the server-side agent after this
	// returns and retries the whole step if anything in between fails.
	RemoveAgent(context.Context, *woodpecker.Agent) error

	// ListDeployedAgentNames reports the names of the machines this pool
	// currently has deployed, as passed to DeployAgent.
	//
	// The result is the provider's half of the drift reconciliation: the
	// engine deletes agents the server knows but this list omits, and tears
	// down machines this list reports but the server does not know. So the
	// implementation MUST scope the listing to its own pool (config.PoolID),
	// and MUST report agents that are still booting — anything else makes the
	// engine destroy a machine that belongs to someone else, or one it just
	// created. Providers whose API has no label store filter on an equivalent
	// pool tag set at deploy time.
	//
	// An error means "unknown", not "nothing deployed": it aborts the rest of
	// the reconcile cycle, so returning a partial list instead of the error
	// is worse than returning nothing.
	ListDeployedAgentNames(context.Context) ([]string, error)

	// Capabilities reports every (platform, backend) pair this provider can
	// deploy with its current configuration. The engine turns each pair into
	// one scheduling bucket, so a pair listed here must be deployable via
	// DeployAgent, and a pair left out is one no queued task can be served
	// with.
	//
	// It is queried once at startup and the result is cached for the process
	// lifetime: an error aborts startup, and later configuration changes on
	// the cloud side are not picked up until the autoscaler restarts.
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
