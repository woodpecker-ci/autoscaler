package main

import (
	"os"

	"github.com/urfave/cli/v2"
)

var flags = []cli.Flag{
	&cli.StringFlag{
		Name:    "log-level",
		Value:   "info",
		Usage:   "default log level",
		EnvVars: []string{"WOODPECKER_LOG_LEVEL"},
	},
	&cli.StringFlag{
		Name:    "reconciliation-interval",
		Value:   "1m",
		Usage:   "interval at which the autoscaler will reconcile as duration string like 2h45m (https://pkg.go.dev/maze.io/x/duration#ParseDuration)",
		EnvVars: []string{"WOODPECKER_RECONCILIATION_INTERVAL"},
	},
	&cli.StringFlag{
		Name:  "pool-id",
		Value: "1",
		Usage: "id of the scaler pool",
	},
	&cli.IntFlag{
		Name:    "min-agents",
		Value:   1,
		Usage:   "minimum amount of agents",
		EnvVars: []string{"WOODPECKER_MIN_AGENTS"},
	},
	&cli.IntFlag{
		Name:    "max-agents",
		Value:   10,
		Usage:   "maximum amount of agents",
		EnvVars: []string{"WOODPECKER_MAX_AGENTS"},
	},
	&cli.StringFlag{
		Name:    "agent-allowed-startup-time",
		Value:   "10m",
		Usage:   "time an agent is allowed to start before it can be terminated again as duration string like 2h45m (https://pkg.go.dev/maze.io/x/duration#ParseDuration)",
		EnvVars: []string{"WOODPECKER_AGENT_ALLOWED_STARTUP_TIME"},
	},
	&cli.IntFlag{
		Name:    "workflows-per-agent",
		Value:   2,
		Usage:   "max workflows an agent will executed in parallel",
		EnvVars: []string{"WOODPECKER_WORKFLOWS_PER_AGENT"},
	},
	&cli.StringFlag{
		Name:    "server",
		Value:   "http://localhost:8000",
		Usage:   "woodpecker server address",
		EnvVars: []string{"WOODPECKER_SERVER"},
	},
	&cli.StringFlag{
		Name:     "token",
		Usage:    "woodpecker api token",
		EnvVars:  []string{"WOODPECKER_TOKEN"},
		FilePath: os.Getenv("WOODPECKER_TOKEN_FILE"),
	},
	&cli.StringFlag{
		Name:    "grpc-addr",
		Value:   "woodpecker-server:9000",
		Usage:   "grpc address of the woodpecker server",
		EnvVars: []string{"WOODPECKER_GRPC_ADDR"},
	},
	&cli.BoolFlag{
		Name:    "grpc-secure",
		Value:   false,
		Usage:   "use secure grpc connection to the woodpecker server",
		EnvVars: []string{"WOODPECKER_GRPC_SECURE"},
	},
	&cli.StringFlag{
		Name:    "provider",
		Value:   "hetznercloud",
		Usage:   "cloud provider to use",
		EnvVars: []string{"WOODPECKER_PROVIDER"},
	},
	&cli.StringFlag{
		Name:    "agent-image",
		Value:   "woodpeckerci/woodpecker-agent:next",
		Usage:   "agent image to use",
		EnvVars: []string{"WOODPECKER_AGENT_IMAGE"},
	},
	&cli.StringSliceFlag{
		Name:    "agent-env",
		Usage:   "additional agent environment variables as list with key=value pairs",
		EnvVars: []string{"WOODPECKER_AGENT_ENV"},
	},
}
