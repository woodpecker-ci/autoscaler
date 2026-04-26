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
                Value:   "5m",
                Usage:   "timeout after which an agent is considered inactive",
                Sources: cli.EnvVars("WOODPECKER_AGENT_INACTIVITY_TIMEOUT"),
            },
            &cli.StringFlag{
                Name:    "oracle-tenancy-ocid",
                Value:   "",
                Usage:   "OCI tenancy OCID",
                Sources: cli.EnvVars("WOODPECKER_ORACLE_TENANCY_OCID"),
            },
            &cli.StringFlag{
                Name:    "oracle-user-ocid",
                Value:   "",
                Usage:   "OCI user OCID",
                Sources: cli.EnvVars("WOODPECKER_ORACLE_USER_OCID"),
            },
            &cli.StringFlag{
                Name:    "oracle