package v1

import (
	"errors"
	"os"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/scaleway/scaleway-sdk-go/scw"
	"github.com/urfave/cli/v2"
)

const (
	DefaultPool = "default"

	category   = "Scaleway"
	flagPrefix = "scw"
	envPrefix  = "WOODPECKER_SCW"
)

var ProviderFlags = []cli.Flag{
	&cli.StringFlag{
		Name:    flagPrefix + "-access-key",
		Usage:   "Scaleway IAM API Token Access Key",
		EnvVars: []string{envPrefix + "_ACCESS_KEY"},
		// NB(raskyld): We should recommend the usage of file-system to users
		// Most container runtimes support mounting secrets into the fs
		// natively.
		FilePath: os.Getenv(envPrefix + "_ACCESS_KEY_FILE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:    flagPrefix + "-secret-key",
		Usage:   "Scaleway IAM API Token Secret Key",
		EnvVars: []string{envPrefix + "_SECRET_KEY"},
		// NB(raskyld): We should recommend the usage of file-system to users
		// Most container runtimes support mounting secrets into the fs
		// natively.
		FilePath: os.Getenv(envPrefix + "_SECRET_KEY_FILE"),
		Category: category,
	},
	// TODO(raskyld): implement multi-AZ
	&cli.StringFlag{
		Name:        flagPrefix + "-zone",
		Usage:       "Scaleway Zone where to spawn instances",
		EnvVars:     []string{envPrefix + "_ZONE"},
		Category:    category,
		DefaultText: scw.ZoneFrPar2.String(),
	},
	&cli.StringFlag{
		Name:     flagPrefix + "-instance-type",
		Usage:    "Scaleway Instance type to spawn",
		EnvVars:  []string{envPrefix + "_INSTANCE_TYPE"},
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     flagPrefix + "-tags",
		Usage:    "Comma separated list of tags to uniquely identify the instances spawned",
		EnvVars:  []string{envPrefix + "_TAGS"},
		Category: category,
	},
	&cli.StringFlag{
		Name:     flagPrefix + "-project",
		Usage:    "Scaleway Project ID in which to spawn the instances",
		EnvVars:  []string{envPrefix + "_PROJECT"},
		Category: category,
	},
	&cli.StringFlag{
		Name:        flagPrefix + "-prefix",
		Usage:       "Prefix prepended before any Scaleway resource name",
		EnvVars:     []string{envPrefix + "_PREFIX"},
		Category:    category,
		DefaultText: "wip-woodpecker-ci-autoscaler",
	},
	&cli.BoolFlag{
		Name:     flagPrefix + "-enable-ipv6",
		Usage:    "Enable IPv6 for the instances",
		EnvVars:  []string{envPrefix + "_ENABLE_IPV6"},
		Category: category,
	},
	&cli.BoolFlag{
		Name:        flagPrefix + "-image",
		Usage:       "The base image for your instance",
		EnvVars:     []string{envPrefix + "_IMAGE"},
		Category:    category,
		DefaultText: "ubuntu_local",
	},
	&cli.Uint64Flag{
		Name:        flagPrefix + "-storage-size",
		Usage:       "How much storage to provision for your agents in bytes",
		EnvVars:     []string{envPrefix + "_STORAGE_SIZE"},
		Category:    category,
		DefaultText: "20000000000",
	},
	&cli.IntFlag{
		Name:        flagPrefix + "-client-max-retries",
		Usage:       "How much times should we retry requests (< 0: infinite, 0: no retry)",
		EnvVars:     []string{envPrefix + "_CLIENT_MAX_RETRIES"},
		Category:    category,
		DefaultText: "5",
	},
	&cli.StringFlag{
		Name:        flagPrefix + "-client-retry-exponential-base",
		Usage:       "Exponential base duration for the retry mechanisms",
		EnvVars:     []string{envPrefix + "_CLIENT_RETRY_EXPONENTIAL_BASE"},
		Category:    category,
		DefaultText: "2s",
	},
}

func FromCLI(c *cli.Context) (Config, error) {
	if !c.IsSet(flagPrefix + "-instance-type") {
		return Config{}, errors.New("you must specify an instance type")
	}

	if !c.IsSet(flagPrefix + "-tags") {
		return Config{}, errors.New("you must specify tags to apply to your resources")
	}

	if !c.IsSet(flagPrefix + "-project") {
		return Config{}, errors.New("you must specify in which project resources should be spawned")
	}

	if !c.IsSet(flagPrefix + "-secret-key") {
		return Config{}, errors.New("you must specify a secret key")
	}

	if !c.IsSet(flagPrefix + "-access-key") {
		return Config{}, errors.New("you must specify an access key")
	}

	zone := scw.Zone(c.String(flagPrefix + "-zone"))
	if !zone.Exists() {
		return Config{}, errors.New(zone.String() + " is not a valid zone")
	}

	cfg := Config{
		SecretKey:        c.String(flagPrefix + "-secret-key"),
		AccessKey:        c.String(flagPrefix + "-access-key"),
		DefaultProjectID: c.String(flagPrefix + "-project"),
	}

	maxRetries := c.Int(flagPrefix + "-client-max-retries")
	expoBase, err := time.ParseDuration(c.String(flagPrefix + "-client-retry-exponential-base"))
	if err != nil {
		return Config{}, err
	}

	if maxRetries == 0 {
		cfg.ClientRetry = &backoff.StopBackOff{}
	} else {
		bo := backoff.NewExponentialBackOff()
		bo.InitialInterval = expoBase
		cfg.ClientRetry = bo
	}

	if maxRetries > 0 {
		cfg.ClientRetry = backoff.WithMaxRetries(cfg.ClientRetry, uint64(maxRetries))
	}

	cfg.InstancePool = map[string]InstancePool{
		DefaultPool: {
			Locality: Locality{
				Zones: []scw.Zone{zone},
			},
			ProjectID: scw.StringPtr(c.String(flagPrefix + "-project")),
			Prefix:    c.String(flagPrefix + "-prefix"),
			Tags:      c.StringSlice(flagPrefix + "-tags"),
			// We do not need stables IP for our JIT runners
			DynamicIPRequired: scw.BoolPtr(true),
			CommercialType:    c.String(flagPrefix + "-instance-type"),
			Image:             c.String(flagPrefix + "-image"),
			EnableIPv6:        c.Bool(flagPrefix + "-enable-ipv6"),
			Storage:           scw.Size(c.Uint64(flagPrefix + "-storage-size")),
		},
	}

	return cfg, nil
}
