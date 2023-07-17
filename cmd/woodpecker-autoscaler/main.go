package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/woodpecker-ci/autoscaler/drivers/hetznercloud"
	"github.com/woodpecker-ci/autoscaler/server"

	_ "github.com/joho/godotenv/autoload"
	"github.com/urfave/cli/v2"

	"github.com/woodpecker-ci/autoscaler/config"
	"github.com/woodpecker-ci/autoscaler/engine"
)

func setupProvider(ctx *cli.Context, config *config.Config) (engine.Provider, error) {
	switch driver := ctx.String("provider"); driver {
	case "hetznercloud":
		return hetznercloud.New(ctx, config)
	}

	return nil, fmt.Errorf("unknown provider: %s", ctx.String("provider"))
}

func run(ctx *cli.Context) error {
	client, err := server.NewClient(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	agentEnvironment := make(map[string]string)
	for _, env := range ctx.StringSlice("agent-env") {
		parts := strings.Split(env, "=")
		if len(parts) != 2 {
			return fmt.Errorf("invalid agent environment variable: %s", env)
		}
		agentEnvironment[parts[0]] = parts[1]
	}

	config := &config.Config{
		MinAgents:         ctx.Int("min-agents"),
		MaxAgents:         ctx.Int("max-agents"),
		WorkflowsPerAgent: ctx.Int("workflows-per-agent"),
		PoolID:            ctx.String("pool-id"),
		GRPCAddress:       ctx.String("grpc-addr"),
		GRPCSecure:        ctx.Bool("grpc-secure"),
		Image:             ctx.String("agent-image"),
		Environment:       agentEnvironment,
	}

	provider, err := setupProvider(ctx, config)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	autoscaler := engine.NewAutoscaler(provider, client, config)

	interval, err := time.ParseDuration(ctx.String("interval"))
	if err != nil {
		log.Error().Err(err).Msgf("failed to parse reconcilation interval, use default: %v", optionIntervalDefault)
		interval, _ = time.ParseDuration(optionIntervalDefault)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(interval):
			if err := autoscaler.Reconcile(ctx.Context); err != nil {
				return err
			}
		}
	}
}

func main() {
	app := &cli.App{
		Name:  "autoscaler",
		Usage: "scale to the moon and back",
		Flags: flags,
		Before: func(ctx *cli.Context) error {
			zerolog.SetGlobalLevel(zerolog.InfoLevel)
			if ctx.IsSet("log-level") {
				logLevelFlag := ctx.String("log-level")
				lvl, err := zerolog.ParseLevel(logLevelFlag)
				if err != nil {
					log.Warn().Msgf("unknown logging level: %s", logLevelFlag)
				}
				zerolog.SetGlobalLevel(lvl)
			}

			return nil
		},
		Action: run,
	}

	// Register hetznercloud flags
	app.Flags = append(app.Flags, hetznercloud.DriverFlags...)

	if err := app.Run(os.Args); err != nil {
		log.Error().Err(err).Msg("")
	}
}
