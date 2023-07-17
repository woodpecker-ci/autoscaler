package config

type Config struct {
	MinAgents         int
	MaxAgents         int
	WorkflowsPerAgent int
	PoolID            string
	Image             string
	Environment       map[string]string
	GRPCAddress       string
	GRPCSecure        bool
}
