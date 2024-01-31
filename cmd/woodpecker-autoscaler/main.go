package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/joho/godotenv/autoload"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/autoscaler/providers/hetznercloud"
	"go.woodpecker-ci.org/autoscaler/server"
)

func setupProvider(ctx *cli.Context, config *config.Config) (engine.Provider, error) {
	switch ctx.String("provider") {
	case "hetznercloud":
		return hetznercloud.New(ctx, config)
	// TODO: Temp disabled due to the security issue https://github.com/woodpecker-ci/autoscaler/issues/91
	// Enable it again when the issue is fixed.
	// case "linode":
	// 	return linode.New(ctx, config)
	case "":
		return nil, fmt.Errorf("please select a provider")
	}

	return nil, fmt.Errorf("unknown provider: %s", ctx.String("provider"))
}

func run(ctx *cli.Context) error {
	log.Log().Msgf("Starting autoscaler with log-level=%s", zerolog.GlobalLevel().String())

	client, err := server.NewClient(ctx)
	if err != nil {
		return err
	}

	agentEnvironment := make(map[string]string)
	for _, env := range ctx.StringSlice("agent-env") {
		before, after, _ := strings.Cut(env, "=")
		if before == "" || after == "" {
			return fmt.Errorf("invalid agent environment variable: %s", env)
		}
		agentEnvironment[before] = after
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
		LabelsFilter:      ctx.String("labels-filter"),
		APIToken:          ctx.String("server-token"),
		APIUrl:            ctx.String("server-url"),
	}

	provider, err := setupProvider(ctx, config)
	if err != nil {
		return err
	}

	autoscaler := engine.NewAutoscaler(provider, client, config)

	config.AgentAllowedStartupTime, err = time.ParseDuration(ctx.String("agent-allowed-startup-time"))
	if err != nil {
		return fmt.Errorf("can't parse agent-allowed-startup-time: %w", err)
	}

	reconciliationInterval, err := time.ParseDuration(ctx.String("reconciliation-interval"))
	if err != nil {
		return fmt.Errorf("can't parse reconciliation-interval: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(reconciliationInterval):
			autoscaler.Reconcile(ctx.Context)
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
					log.Warn().Msgf("LogLevel = %s is unknown", logLevelFlag)
				}
				zerolog.SetGlobalLevel(lvl)
			}

			// if debug or trace also log the caller
			if zerolog.GlobalLevel() <= zerolog.DebugLevel {
				log.Logger = log.With().Caller().Logger()
			}

			return nil
		},
		Action: run,
	}

	// Register hetznercloud flags
	app.Flags = append(app.Flags, hetznercloud.ProviderFlags...)
	// Register linode flags
	// TODO: Temp disabled due to the security issue https://github.com/woodpecker-ci/autoscaler/issues/91
	// Enable it again when the issue is fixed.
	// app.Flags = append(app.Flags, linode.ProviderFlags...)

	if err := app.Run(os.Args); err != nil {
		log.Fatal().Err(err).Msg("")
	}
}
