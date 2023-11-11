package v1

import (
	"bytes"
	"errors"
	"github.com/scaleway/scaleway-sdk-go/scw"
	"github.com/urfave/cli/v2"
	"go.woodpecker-ci.org/autoscaler/config"
	"os"
)

const category = "Scaleway"
const flagPrefix = "scw"
const envPrefix = "WOODPECKER_SCW"

var ProviderFlags = []cli.Flag{
	&cli.StringFlag{
		Name:    flagPrefix + "-api-token",
		Usage:   "Scaleway IAM API Token",
		EnvVars: []string{envPrefix + "_API_TOKEN"},
		// NB(raskyld): We should recommend the usage of file-system to users
		// Most container runtimes support mounting secrets into the fs
		// natively.
		FilePath: os.Getenv(envPrefix + "_API_TOKEN_FILE"),
		Category: category,
	},
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
}

func FromCLI(c *cli.Context, engineConfig *config.Config) (*Config, error) {
	if !c.IsSet(flagPrefix + "-instance-type") {
		return nil, errors.New("you must specify an instance type")
	}

	if !c.IsSet(flagPrefix + "-tags") {
		return nil, errors.New("you must specify tags to apply to your resources")
	}

	if !c.IsSet(flagPrefix + "-project") {
		return nil, errors.New("you must specify in which project resources should be spawned")
	}

	zone := scw.Zone(c.String(flagPrefix + "-zone"))
	if !zone.Exists() {
		return nil, errors.New(zone.String() + " is not a valid zone")
	}

	cfg := &Config{
		ApiToken: bytes.NewBufferString(c.String(flagPrefix + "-api-token")),
	}

	cfg.InstancePool = map[string]InstancePool{
		"default": {
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
