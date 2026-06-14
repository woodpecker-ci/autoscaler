package config

import (
	"time"

	"go.woodpecker-ci.org/autoscaler/engine/types"
)

type Config struct {
	MinAgents              int
	MaxAgents              int
	WorkflowsPerAgent      int
	PoolID                 string
	Image                  string
	Environment            map[string]string
	GRPCAddress            string
	GRPCSecure             bool
	AgentInactivityTimeout time.Duration
	AgentIdleTimeout       time.Duration
	UserData               string // cloudinit template
	ExtraAgentLabels       map[string]string

	// BillingModel is taken from the selected provider and selects the teardown
	// policy the engine applies to idle agents.
	BillingModel types.BillingModel
	// ReconciliationInterval is the loop period. It is added to
	// AgentBillingTeardownMargin so the billing-hour teardown window can never
	// be skipped between two reconciliations.
	ReconciliationInterval time.Duration
	// AgentBillingTeardownMargin is, for BillingHourlyRoundUp providers, how
	// long before each paid-hour boundary an idle agent becomes eligible for
	// teardown.
	AgentBillingTeardownMargin time.Duration
}
