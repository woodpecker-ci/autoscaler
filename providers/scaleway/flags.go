package scaleway

import (
	"os"

	"github.com/scaleway/scaleway-sdk-go/scw"
	"github.com/urfave/cli/v2"
)

const category = "Scaleway"

//nolint:mnd
var ProviderFlags = []cli.Flag{
	&cli.StringFlag{
		Name:    "scw-access-key",
		Usage:   "Scaleway IAM API Token Access Key",
		EnvVars: []string{"WOODPECKER_SCW_ACCESS_KEY"},
		// NB(raskyld): We should recommend the usage of file-system to users
		// Most container runtimes support mounting secrets into the fs
		// natively.
		FilePath: os.Getenv("WOODPECKER_SCW_ACCESS_KEY_FILE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:    "scw-secret-key",
		Usage:   "Scaleway IAM API Token Secret Key",
		EnvVars: []string{"WOODPECKER_SCW_SECRET_KEY"},
		// NB(raskyld): We should recommend the usage of file-system to users
		// Most container runtimes support mounting secrets into the fs
		// natively.
		FilePath: os.Getenv("WOODPECKER_SCW_SECRET_KEY_FILE"),
		Category: category,
	},
	// TODO(raskyld): implement multi-AZ
	&cli.StringFlag{
		Name:     "scw-zone",
		Usage:    "Scaleway Zone where to spawn instances",
		EnvVars:  []string{"WOODPECKER_SCW_ZONE"},
		Category: category,
		Value:    scw.ZoneFrPar2.String(),
	},
	&cli.StringFlag{
		Name:     "scw-instance-type",
		Usage:    "Scaleway Instance type to spawn",
		EnvVars:  []string{"WOODPECKER_SCW_INSTANCE_TYPE"},
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "scw-tags",
		Usage:    "Comma separated list of tags to uniquely identify the instances spawned",
		EnvVars:  []string{"WOODPECKER_SCW_TAGS"},
		Category: category,
	},
	&cli.StringFlag{
		Name:     "scw-project",
		Usage:    "Scaleway Project ID in which to spawn the instances",
		EnvVars:  []string{"WOODPECKER_SCW_PROJECT"},
		Category: category,
	},
	&cli.StringFlag{
		Name:     "scw-prefix",
		Usage:    "Prefix prepended before any Scaleway resource name",
		EnvVars:  []string{"WOODPECKER_SCW_PREFIX"},
		Category: category,
		Value:    "wip-woodpecker-ci-autoscaler",
	},
	&cli.BoolFlag{
		Name:     "scw-enable-ipv6",
		Usage:    "Enable IPv6 for the instances",
		EnvVars:  []string{"WOODPECKER_SCW_ENABLE_IPV6"},
		Category: category,
	},
	&cli.StringFlag{
		Name:     "scw-image",
		Usage:    "The base image for your instance",
		EnvVars:  []string{"WOODPECKER_SCW_IMAGE"},
		Category: category,
		Value:    "ubuntu_jammy",
	},
	&cli.Uint64Flag{
		Name:     "scw-storage-size",
		Usage:    "How much storage to provision for your agents in GB",
		EnvVars:  []string{"WOODPECKER_SCW_STORAGE_SIZE"},
		Category: category,
		Value:    25,
	},
}
