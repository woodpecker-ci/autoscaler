package main

import (
	"os"

	"github.com/urfave/cli/v3"
)

//nolint:mnd
var flags = []cli.Flag{
	&cli.StringFlag{
		Name:    "log-level",
		Value:   "info",
		Usage:   "default log level",
		Sources: cli.EnvVars("WOODPECKER_LOG_LEVEL"),
	},
	&cli.StringFlag{
		Name:    "reconciliation-interval",
		Value:   "1m",
		Usage:   "interval at which the autoscaler will reconcile as duration string like 2h45m (https://pkg.go.dev/time#ParseDuration)",
		Sources: cli.EnvVars("WOODPECKER_RECONCILIATION_INTERVAL"),
	},
	&cli.StringFlag{
		Name:    "pool-id",
		Value:   "1",
		Usage:   "id of the autoscaler pool",
		Sources: cli.EnvVars("WOODPECKER_POOL_ID"),
	},
	&cli.IntFlag{
		Name:    "min-agents",
		Value:   1,
		Usage:   "minimum amount of agents",
		Sources: cli.EnvVars("WOODPECKER_MIN_AGENTS"),
	},
	&cli.IntFlag{
		Name:    "max-agents",
		Value:   10,
		Usage:   "maximum amount of agents",
		Sources: cli.EnvVars("WOODPECKER_MAX_AGENTS"),
	},
	&cli.StringFlag{
		Name:    "agent-inactivity-timeout",
		Value:   "10m",
		Usage:   "time an agent is allowed to be inactive before it can be terminated as duration string like 2h45m (https://pkg.go.dev/time#ParseDuration)",
		Sources: cli.EnvVars("WOODPECKER_AGENT_INACTIVITY_TIMEOUT", "WOODPECKER_AGENT_ALLOWED_STARTUP_TIME"),
	},
	&cli.StringFlag{
		Name:    "agent-idle-timeout",
		Value:   "10m",
		Usage:   "time an agent is allowed to be idle before it can be terminated as duration string like 2h45m (https://pkg.go.dev/time#ParseDuration)",
		Sources: cli.EnvVars("WOODPECKER_AGENT_IDLE_TIMEOUT"),
	},
	&cli.StringFlag{
		Name:    "agent-billing-teardown-margin",
		Value:   "2m",
		Usage:   "for providers billed by the rounded-up hour (e.g. linode, hetznercloud), how long before each paid-hour boundary an idle agent becomes eligible for teardown as duration string like 2m45s (https://pkg.go.dev/time#ParseDuration); the reconciliation interval is added to this so the window is never skipped",
		Sources: cli.EnvVars("WOODPECKER_AGENT_BILLING_TEARDOWN_MARGIN"),
	},
	&cli.IntFlag{
		Name:    "workflows-per-agent",
		Value:   2,
		Usage:   "max workflows an agent will executed in parallel",
		Sources: cli.EnvVars("WOODPECKER_WORKFLOWS_PER_AGENT"),
	},
	&cli.StringFlag{
		Name:    "server-url",
		Value:   "http://localhost:8000",
		Usage:   "woodpecker server address",
		Sources: cli.EnvVars("WOODPECKER_SERVER"),
	},
	&cli.StringFlag{
		Name:  "server-token",
		Usage: "woodpecker api token",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_TOKEN"),
			cli.File(os.Getenv("WOODPECKER_TOKEN_FILE")),
		),
	},
	&cli.StringFlag{
		Name:    "grpc-addr",
		Value:   "woodpecker-server:9000",
		Usage:   "grpc address of the woodpecker server",
		Sources: cli.EnvVars("WOODPECKER_GRPC_ADDR"),
	},
	&cli.BoolFlag{
		Name:    "grpc-secure",
		Value:   false,
		Usage:   "use secure grpc connection to the woodpecker server",
		Sources: cli.EnvVars("WOODPECKER_GRPC_SECURE"),
	},
	&cli.StringFlag{
		Name:    "provider",
		Value:   "",
		Usage:   "cloud provider to use",
		Sources: cli.EnvVars("WOODPECKER_PROVIDER"),
	},
	&cli.StringFlag{
		Name:  "cloudinit-template",
		Usage: "cloudinit userdata template to setup the provider instance",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_CLOUDINIT_TEMPLATE"),
			cli.File(os.Getenv("WOODPECKER_CLOUDINIT_TEMPLATE_FILE")),
		),
	},
	&cli.StringFlag{
		Name:    "agent-image",
		Value:   "woodpeckerci/woodpecker-agent:next",
		Usage:   "agent image to use",
		Sources: cli.EnvVars("WOODPECKER_AGENT_IMAGE"),
	},
	&cli.StringSliceFlag{
		Name:    "agent-env",
		Usage:   "additional agent environment variables as list with key=value pairs",
		Sources: cli.EnvVars("WOODPECKER_AGENT_ENV"),
	},
	&cli.StringSliceFlag{
		Name:    "agent-labels",
		Usage:   "add additional labels the agent will report to the server. list with key=value pairs",
		Sources: cli.EnvVars("WOODPECKER_AGENT_LABELS"),
	},
}
