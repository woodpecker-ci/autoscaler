package linode

import (
	"os"

	"github.com/urfave/cli/v3"
)

const category = "Linode"

var ProviderFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "linode-api-token",
		Usage: "Linode api token",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_LINODE_API_TOKEN"),
			cli.File(os.Getenv("WOODPECKER_LINODE_API_TOKEN_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "linode-region",
		Value:    "ap-southeast",
		Usage:    "linode region",
		Sources:  cli.EnvVars("WOODPECKER_LINODE_REGION"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "linode-instance-type",
		Value:    "g6-nanode-1",
		Usage:    "linode instance type",
		Sources:  cli.EnvVars("WOODPECKER_LINODE_INSTANCE_TYPE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "linode-ssh-key",
		Usage:    "Name of Linode cloud ssh key",
		Sources:  cli.EnvVars("WOODPECKER_LINODE_SSH_KEY"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "linode-root-pass",
		Usage:    "Linode Root Password",
		Sources:  cli.EnvVars("WOODPECKER_LINODE_ROOT_PASS"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "linode-image",
		Value:    "linode/ubuntu24.04",
		Usage:    "Linode OS image",
		Sources:  cli.EnvVars("WOODPECKER_LINODE_IMAGE"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "linode-tags",
		Usage:    "Linode tags",
		Sources:  cli.EnvVars("WOODPECKER_LINODE_TAGS"),
		Category: category,
	},
}
