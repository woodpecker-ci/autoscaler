package digitalocean

import (
	"os"

	"github.com/urfave/cli/v2"
)

const category = "Digtial Ocean"

var DriverFlags = []cli.Flag{
	// digitalocean
	&cli.StringFlag{
		Name:     "digitalocean-api-token",
		Usage:    "digitalocean api token",
		EnvVars:  []string{"WOODPECKER_DIGITALOCEAN_API_TOKEN"},
		FilePath: os.Getenv("WOODPECKER_DIGITALOCEAN_API_TOKEN_FILE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-region",
		Value:    "nbg1", // TODO
		Usage:    "digitalocean region",
		EnvVars:  []string{"WOODPECKER_DIGITALOCEAN_REGION"},
		Category: category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-droplet-size",
		Value:    "cx11",
		Usage:    "digitalocean server type",
		EnvVars:  []string{"WOODPECKER_DIGITALOCEAN_SERVER_TYPE"},
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "digitalocean-ssh-keys",
		Usage:    "names of digitalocean ssh keys",
		EnvVars:  []string{"WOODPECKER_DIGITALOCEAN_SSH_KEYS"},
		Category: category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-user-data",
		Usage:    "digitalocean userdata template",
		EnvVars:  []string{"WOODPECKER_DIGITALOCEAN_USERDATA"},
		FilePath: os.Getenv("WOODPECKER_DIGITALOCEAN_USERDATA_FILE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-image",
		Value:    "ubuntu-22.04",
		Usage:    "digitalocean image",
		EnvVars:  []string{"WOODPECKER_DIGITALOCEAN_IMAGE"},
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "digitalocean-labels",
		Usage:    "digitalocean server labels",
		EnvVars:  []string{"WOODPECKER_DIGITALOCEAN_LABELS"},
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "digitalocean-firewall",
		Usage:    "names of digitalocean firewall",
		EnvVars:  []string{"WOODPECKER_DIGITALOCEAN_FIREWALL"},
		Category: category,
	},
	&cli.BoolFlag{
		Name:     "digitalocean-public-ipv6-enable",
		Value:    true,
		Usage:    "enables public ipv6 network for agents",
		EnvVars:  []string{"WOODPECKER_DIGITALOCEAN_PUBLIC_IPV6_ENABLE"},
		Category: category,
	},
}
