package config

import "time"

type Config struct {
	MinAgents               int
	MaxAgents               int
	WorkflowsPerAgent       int
	PoolID                  string
	Image                   string
	Environment             map[string]string
	GRPCAddress             string
	GRPCSecure              bool
	AgentAllowedStartupTime time.Duration
	AgentInactivityTimeout  time.Duration
	FilterLabels            string
}
