package vultr

import (
	"os"

	"github.com/urfave/cli/v3"
)

const category = "Vultr"

var ProviderFlags = []cli.Flag{
	// vultr
	&cli.StringFlag{
		Name:  "vultr-api-token",
		Usage: "vultr api token",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_VULTR_API_TOKEN"),
			cli.File(os.Getenv("WOODPECKER_VULTR_API_TOKEN_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "vultr-region",
		Value:    "nbg1",
		Usage:    "vultr region",
		Sources:  cli.EnvVars("WOODPECKER_VULTR_REGION"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "vultr-plan",
		Usage:    "vultr plan",
		Sources:  cli.EnvVars("WOODPECKER_VULTR_PLAN"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "vultr-ssh-keys",
		Usage:    "names of vultr ssh keys",
		Sources:  cli.EnvVars("WOODPECKER_VULTR_SSH_KEYS"),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "vultr-user-data",
		Usage: "vultr userdata template",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_VULTR_USERDATA"),
			cli.File(os.Getenv("WOODPECKER_VULTR_USERDATA_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "vultr-image",
		Value:    "ubuntu-22.04",
		Usage:    "vultr image",
		Sources:  cli.EnvVars("WOODPECKER_VULTR_IMAGE"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "vultr-labels",
		Usage:    "vultr server labels",
		Sources:  cli.EnvVars("WOODPECKER_VULTR_LABELS"),
		Category: category,
	},
	&cli.BoolFlag{
		Name:     "vultr-public-ipv6-enable",
		Value:    true,
		Usage:    "enables public ipv6 network for agents",
		Sources:  cli.EnvVars("WOODPECKER_VULTR_PUBLIC_IPV6_ENABLE"),
		Category: category,
	},
}
