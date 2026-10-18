package main

import (
	"context"
	"errors"
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
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/autoscaler/providers/aws"
	"go.woodpecker-ci.org/autoscaler/providers/digitalocean"
	"go.woodpecker-ci.org/autoscaler/providers/hetznercloud"
	"go.woodpecker-ci.org/autoscaler/providers/linode"
	"go.woodpecker-ci.org/autoscaler/providers/openstack"
	"go.woodpecker-ci.org/autoscaler/providers/scaleway"
	"go.woodpecker-ci.org/autoscaler/providers/vultr"
	"go.woodpecker-ci.org/autoscaler/server"
	"go.woodpecker-ci.org/autoscaler/version"
)

func setupProvider(ctx context.Context, cmd *cli.Command, config *config.Config) (types.Provider, error) {
	switch cmd.String("provider") {
	case "aws":
		return aws.New(ctx, cmd, config)
	case "digitalocean":
		return digitalocean.New(ctx, cmd, config)
	case "hetznercloud":
		return hetznercloud.New(ctx, cmd, config)
	case "linode":
		return linode.New(ctx, cmd, config)
	case "vultr":
		return vultr.New(ctx, cmd, config)
	case "openstack":
		return openstack.New(ctx, cmd, config)
	case "scaleway":
		return scaleway.New(ctx, cmd, config)
	case "":
		return nil, fmt.Errorf("please select a provider")
	}

	return nil, fmt.Errorf("unknown provider: %s", cmd.String("provider"))
}

// reservedAgentEnv lists agent environment variables the autoscaler sets
// itself, each with the autoscaler option to use instead.
var reservedAgentEnv = []struct{ env, option string }{
	{env: "WOODPECKER_AGENT_LABELS", option: "agent-labels"},
}

// checkAgentEnv rejects agent environment variables that are managed by an
// autoscaler option, as the engine relies on what that option configures.
func checkAgentEnv(agentEnv map[string]string) error {
	var err error
	for _, r := range reservedAgentEnv {
		if _, ok := agentEnv[r.env]; ok {
			err = errors.Join(err, fmt.Errorf("setting %s via WOODPECKER_AGENT_ENV is forbidden, use the autoscaler option --%s instead", r.env, r.option))
		}
	}
	return err
}

func run(ctx context.Context, cmd *cli.Command) error {
	log.Log().Msgf("starting autoscaler with version '%s'", version.String())

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

	agentLabels := make(map[string]string)
	for _, env := range cmd.StringSlice("agent-labels") {
		before, after, _ := strings.Cut(env, "=")
		if before == "" || after == "" {
			return fmt.Errorf("invalid agent labels variable: %s", env)
		}
		agentLabels[before] = after
	}

	if err := checkAgentEnv(agentEnvironment); err != nil {
		return err
	}

	config := &config.Config{
		MinAgents:         cmd.Int("min-agents"),
		MaxAgents:         cmd.Int("max-agents"),
		WorkflowsPerAgent: cmd.Int("workflows-per-agent"),
		PoolID:            cmd.String("pool-id"),
		GRPCAddress:       cmd.String("grpc-addr"),
		GRPCSecure:        cmd.Bool("grpc-secure"),
		Image:             cmd.String("agent-image"),
		UserData:          cmd.String("cloudinit-template"),
		ExtraAgentLabels:  agentLabels,
		Environment:       agentEnvironment,
	}

	provider, err := setupProvider(ctx, cmd, config)
	if err != nil {
		return err
	}
	config.BillingModel = provider.BillingModel()

	config.AgentInactivityTimeout, err = time.ParseDuration(cmd.String("agent-inactivity-timeout"))
	if err != nil {
		return fmt.Errorf("can't parse agent-inactivity-timeout: %w", err)
	}

	config.AgentCreationTimeout, err = time.ParseDuration(cmd.String("agent-creation-timeout"))
	if err != nil {
		return fmt.Errorf("can't parse agent-creation-timeout: %w", err)
	}

	config.AgentIdleTimeout, err = time.ParseDuration(cmd.String("agent-idle-timeout"))
	if err != nil {
		return fmt.Errorf("can't parse agent-idle-timeout: %w", err)
	}

	config.AgentBillingTeardownMargin, err = time.ParseDuration(cmd.String("agent-billing-teardown-margin"))
	if err != nil {
		return fmt.Errorf("can't parse agent-billing-teardown-margin: %w", err)
	}

	reconciliationInterval, err := time.ParseDuration(cmd.String("reconciliation-interval"))
	if err != nil {
		return fmt.Errorf("can't parse reconciliation-interval: %w", err)
	}
	config.ReconciliationInterval = reconciliationInterval

	autoscaler, err := engine.NewAutoscaler(ctx, provider, client, config)
	if err != nil {
		return fmt.Errorf("could not create autoscaler: %w", err)
	}

	if config.BillingModel == types.BillingHourlyRoundUp {
		log.Info().
			Str("provider", cmd.String("provider")).
			Str("teardown-window", (config.AgentBillingTeardownMargin + reconciliationInterval).String()).
			Msg("hourly-round-up billing: idle agents are kept warm until just before each paid-hour boundary")
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
		Name:    "autoscaler",
		Version: version.String(),
		Usage:   "scale to the moon and back",
		Flags:   flags,
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
	app.Flags = append(app.Flags, linode.ProviderFlags...)
	app.Flags = append(app.Flags, aws.ProviderFlags...)
	app.Flags = append(app.Flags, digitalocean.ProviderFlags...)
	app.Flags = append(app.Flags, vultr.ProviderFlags...)
	app.Flags = append(app.Flags, openstack.ProviderFlags...)

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Error().Err(err).Msg("got error while try to run autoscaler")
	}
}
