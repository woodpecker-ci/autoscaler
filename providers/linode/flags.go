package linode

import (
	"os"

	"github.com/urfave/cli/v2"
)

const category = "Linode"

//nolint:gomnd
var ProviderFlags = []cli.Flag{
	&cli.StringFlag{
		Name:     "linode-api-token",
		Usage:    "Linode api token",
		EnvVars:  []string{"WOODPECKER_LINODE_API_TOKEN"},
		FilePath: os.Getenv("WOODPECKER_LINODE_API_TOKEN_FILE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "linode-region",
		Value:    "ap-southeast",
		Usage:    "linode region",
		EnvVars:  []string{"WOODPECKER_LINODE_REGION"},
		Category: category,
	},
	&cli.StringFlag{
		Name:     "linode-instance-type",
		Value:    "g6-nanode-1",
		Usage:    "linode instance type",
		EnvVars:  []string{"WOODPECKER_LINODE_INSTANCE_TYPE"},
		Category: category,
	},
	&cli.IntFlag{
		Name:     "linode-stackscript-id",
		Value:    1227924,
		Usage:    "Linode Stackscript ID (set to -1 to use the beta user-data feature instead)",
		EnvVars:  []string{"WOODPECKER_LINODE_STACKSCRIPT_ID"},
		Category: category,
	},
	&cli.StringFlag{
		Name:     "linode-ssh-key",
		Usage:    "Name of Linode cloud ssh key",
		EnvVars:  []string{"WOODPECKER_LINODE_SSH_KEY"},
		Category: category,
	},
	&cli.StringFlag{
		Name:     "linode-root-pass",
		Usage:    "Linode Root Password",
		EnvVars:  []string{"WOODPECKER_LINODE_ROOT_PASS"},
		Category: category,
	},
	&cli.StringFlag{
		Name:     "linode-user-data",
		Usage:    "Linode userdata template",
		EnvVars:  []string{"WOODPECKER_LINODE_USERDATA"},
		FilePath: os.Getenv("WOODPECKER_LINODE_USERDATA_FILE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "linode-image",
		Value:    "linode/ubuntu22.04",
		Usage:    "Linode OS image",
		EnvVars:  []string{"WOODPECKER_LINODE_IMAGE"},
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "linode-tags",
		Usage:    "Linode tags",
		EnvVars:  []string{"WOODPECKER_LINODE_TAGS"},
		Category: category,
	},
}
