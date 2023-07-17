package hetznercloud

import (
	"os"

	"github.com/urfave/cli/v2"
)

const category = "Hetzner Cloud"

var DriverFlags = []cli.Flag{
	// hetzner
	&cli.StringFlag{
		Name:     "hetznercloud-api-token",
		Usage:    "hetzner cloud api token",
		EnvVars:  []string{"WOODPECKER_HETZNERCLOUD_API_TOKEN"},
		FilePath: os.Getenv("WOODPECKER_HETZNERCLOUD_API_TOKEN_FILE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "hetznercloud-location",
		Value:    "nbg1",
		Usage:    "hetzner cloud location",
		EnvVars:  []string{"WOODPECKER_HETZNERCLOUD_LOCATION"},
		Category: category,
	},
	&cli.StringFlag{
		Name:     "hetznercloud-server-type",
		Value:    "cx11",
		Usage:    "hetzner cloud server type",
		EnvVars:  []string{"WOODPECKER_HETZNERCLOUD_SERVER_TYPE"},
		Category: category,
	},
	&cli.IntFlag{
		Name:     "hetznercloud-ssh-key-id",
		Value:    -1,
		Usage:    "id of a hetzner cloud ssh key",
		EnvVars:  []string{"WOODPECKER_HETZNERCLOUD_SSH_KEY_ID"},
		Category: category,
	},
	&cli.StringFlag{
		Name:     "hetznercloud-user-data",
		Usage:    "hetzner cloud userdata template",
		EnvVars:  []string{"WOODPECKER_HETZNERCLOUD_USERDATA"},
		FilePath: os.Getenv("WOODPECKER_HETZNERCLOUD_USERDATA_FILE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "hetznercloud-image",
		Value:    "ubuntu-22.04",
		Usage:    "hetzner cloud image",
		EnvVars:  []string{"WOODPECKER_HETZNERCLOUD_IMAGE"},
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "hetznercloud-labels",
		Usage:    "hetzner cloud server labels",
		EnvVars:  []string{"WOODPECKER_HETZNERCLOUD_LABELS"},
		Category: category,
	},
}
