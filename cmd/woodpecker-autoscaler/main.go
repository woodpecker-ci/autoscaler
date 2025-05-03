package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/joho/godotenv/autoload"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/autoscaler/providers/aws"
	"go.woodpecker-ci.org/autoscaler/providers/hetznercloud"
	"go.woodpecker-ci.org/autoscaler/providers/scaleway"
	"go.woodpecker-ci.org/autoscaler/providers/vultr"
	"go.woodpecker-ci.org/autoscaler/server"
)

func setupProvider(ctx context.Context, cmd *cli.Command, config *config.Config) (engine.Provider, error) {
	switch cmd.String("provider") {
	case "aws":
		return aws.New(ctx, cmd, config)
	case "hetznercloud":
		return hetznercloud.New(ctx, cmd, config)
	// TODO: Temp disabled due to the security issue https://github.com/woodpecker-ci/autoscaler/issues/91
	// Enable it again when the issue is fixed.
	// case "linode":
	// 	return linode.New(ctx, config)
	case "vultr":
		return vultr.New(ctx, cmd, config)
	case "scaleway":
		return scaleway.New(ctx, cmd, config)
	case "":
		return nil, fmt.Errorf("please select a provider")
	}

	return nil, fmt.Errorf("unknown provider: %s", cmd.String("provider"))
}

func run(ctx context.Context, cmd *cli.Command) error {
	log.Log().Msgf("starting autoscaler with log-level=%s", zerolog.GlobalLevel().String())

	client, err := server.NewClient(ctx, cmd)
	if err != nil {
		return err
	}

	agentEnvironment := make(map[string]string)
	for _, env := range cmd.StringSlice("agent-env") {
		before, after, _ := strings.Cut(env, "=")
		if before == "" || after == "" {
			return fmt.Errorf("invalid agent environment variable: %s", env)
		}
		agentEnvironment[before] = after
	}

	config := &config.Config{
		MinAgents:         cmd.Int("min-agents"),
		MaxAgents:         cmd.Int("max-agents"),
		WorkflowsPerAgent: cmd.Int("workflows-per-agent"),
		PoolID:            cmd.String("pool-id"),
		GRPCAddress:       cmd.String("grpc-addr"),
		GRPCSecure:        cmd.Bool("grpc-secure"),
		Image:             cmd.String("agent-image"),
		FilterLabels:      cmd.String("filter-labels"),
		UserData:          cmd.String("provider-user-data"),
		Environment:       agentEnvironment,
	}

	provider, err := setupProvider(ctx, cmd, config)
	if err != nil {
		return err
	}

	autoscaler := engine.NewAutoscaler(provider, client, config)

	config.AgentInactivityTimeout, err = time.ParseDuration(cmd.String("agent-inactivity-timeout"))
	if err != nil {
		return fmt.Errorf("can't parse agent-inactivity-timeout: %w", err)
	}

	config.AgentIdleTimeout, err = time.ParseDuration(cmd.String("agent-idle-timeout"))
	if err != nil {
		return fmt.Errorf("can't parse agent-idle-timeout: %w", err)
	}

	reconciliationInterval, err := time.ParseDuration(cmd.String("reconciliation-interval"))
	if err != nil {
		return fmt.Errorf("can't parse reconciliation-interval: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(reconciliationInterval):
			err := autoscaler.Reconcile(ctx)
			if err != nil {
				log.Error().Err(err).Msg("reconciliation failed")
			}
		}
	}
}

func main() {
	app := &cli.Command{
		Name:  "autoscaler",
		Usage: "scale to the moon and back",
		Flags: flags,
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			zerolog.SetGlobalLevel(zerolog.InfoLevel)
			if cmd.IsSet("log-level") {
				logLevelFlag := cmd.String("log-level")
				lvl, err := zerolog.ParseLevel(logLevelFlag)
				if err != nil {
					log.Warn().Msgf("log-level = %s is unknown", logLevelFlag)
				}
				zerolog.SetGlobalLevel(lvl)
			}

			// if debug or trace also log the caller
			if zerolog.GlobalLevel() <= zerolog.DebugLevel {
				log.Logger = log.With().Caller().Logger()
			}

			return ctx, nil
		},
		Action: run,
	}

	app.Flags = append(app.Flags, hetznercloud.ProviderFlags...)
	app.Flags = append(app.Flags, scaleway.ProviderFlags...)
	// TODO: Temp disabled due to the security issue https://github.com/woodpecker-ci/autoscaler/issues/91
	// Enable it again when the issue is fixed.
	// app.Flags = append(app.Flags, linode.ProviderFlags...)
	app.Flags = append(app.Flags, aws.ProviderFlags...)
	app.Flags = append(app.Flags, vultr.ProviderFlags...)

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Error().Err(err).Msg("got error while try to run autoscaler")
	}
}
