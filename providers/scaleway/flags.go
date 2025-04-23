package scaleway

import (
	"os"

	"github.com/scaleway/scaleway-sdk-go/scw"
	"github.com/urfave/cli/v3"
)

const category = "Scaleway"

//nolint:mnd
var ProviderFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "scaleway-access-key",
		Usage: "Scaleway IAM API Token Access Key",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_SCALEWAY_ACCESS_KEY"),
			cli.File(os.Getenv("WOODPECKER_SCALEWAY_ACCESS_KEY_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "scaleway-secret-key",
		Usage: "Scaleway IAM API Token Secret Key",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_SCALEWAY_SECRET_KEY"),
			cli.File(os.Getenv("WOODPECKER_SCALEWAY_SECRET_KEY_FILE")),
		),
		Category: category,
	},
	// TODO(raskyld): implement multi-AZ
	&cli.StringFlag{
		Name:     "scaleway-zone",
		Usage:    "Scaleway zone where to spawn instances",
		Sources:  cli.EnvVars("WOODPECKER_SCALEWAY_ZONE"),
		Category: category,
		Value:    scw.ZoneFrPar2.String(),
	},
	&cli.StringFlag{
		Name:     "scaleway-instance-type",
		Usage:    "Scaleway instance type to spawn",
		Sources:  cli.EnvVars("WOODPECKER_SCALEWAY_INSTANCE_TYPE"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "scaleway-tags",
		Usage:    "Comma separated list of tags to uniquely identify the instances spawned",
		Sources:  cli.EnvVars("WOODPECKER_SCALEWAY_TAGS"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "scaleway-project",
		Usage:    "Scaleway Project ID in which to spawn the instances",
		Sources:  cli.EnvVars("WOODPECKER_SCALEWAY_PROJECT"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "scaleway-prefix",
		Usage:    "Prefix prepended before any Scaleway resource name",
		Sources:  cli.EnvVars("WOODPECKER_SCALEWAY_PREFIX"),
		Category: category,
		Value:    "wip-woodpecker-ci-autoscaler",
	},
	&cli.BoolFlag{
		Name:     "scaleway-enable-ipv6",
		Usage:    "Enable IPv6 for the instances",
		Sources:  cli.EnvVars("WOODPECKER_SCALEWAY_ENABLE_IPV6"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "scaleway-image",
		Usage:    "The base image for your instance",
		Sources:  cli.EnvVars("WOODPECKER_SCALEWAY_IMAGE"),
		Category: category,
		Value:    "ubuntu_jammy",
	},
	&cli.Uint64Flag{
		Name:     "scaleway-storage-size",
		Usage:    "How much storage to provision for your agents in GB",
		Sources:  cli.EnvVars("WOODPECKER_SCALEWAY_STORAGE_SIZE"),
		Category: category,
		Value:    25,
	},
}
