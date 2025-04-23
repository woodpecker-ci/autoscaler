package hetznercloud

import (
	"os"

	"github.com/urfave/cli/v3"
)

const category = "Hetzner Cloud"

var ProviderFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "hetznercloud-api-token",
		Usage: "hetzner cloud api token",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_HETZNERCLOUD_API_TOKEN"),
			cli.File(os.Getenv("WOODPECKER_HETZNERCLOUD_API_TOKEN_FILE")),
		),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "hetznercloud-server-type",
		Value:    []string{"cx11:nbg1"},
		Usage:    "hetzner cloud server type",
		Sources:  cli.EnvVars("WOODPECKER_HETZNERCLOUD_SERVER_TYPE"),
		Category: category,
	},
	// TODO: Deprecated remove in v1.0
	&cli.StringFlag{
		Name:     "hetznercloud-location",
		Value:    "nbg1",
		Usage:    "hetzner cloud location (deprecated)",
		Sources:  cli.EnvVars("WOODPECKER_HETZNERCLOUD_LOCATION"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "hetznercloud-ssh-keys",
		Usage:    "names of hetzner cloud ssh keys",
		Sources:  cli.EnvVars("WOODPECKER_HETZNERCLOUD_SSH_KEYS"),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "hetznercloud-user-data",
		Usage: "hetzner cloud userdata template",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_HETZNERCLOUD_USERDATA"),
			cli.File(os.Getenv("WOODPECKER_HETZNERCLOUD_USERDATA_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "hetznercloud-image",
		Value:    "ubuntu-22.04",
		Usage:    "hetzner cloud image",
		Sources:  cli.EnvVars("WOODPECKER_HETZNERCLOUD_IMAGE"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "hetznercloud-labels",
		Usage:    "hetzner cloud server labels",
		Sources:  cli.EnvVars("WOODPECKER_HETZNERCLOUD_LABELS"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "hetznercloud-firewalls",
		Usage:    "names of hetzner cloud firewalls",
		Sources:  cli.EnvVars("WOODPECKER_HETZNERCLOUD_FIREWALLS"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "hetznercloud-networks",
		Usage:    "names of hetzner cloud networks",
		Sources:  cli.EnvVars("WOODPECKER_HETZNERCLOUD_NETWORKS"),
		Category: category,
	},
	&cli.BoolFlag{
		Name:     "hetznercloud-public-ipv4-enable",
		Value:    true,
		Usage:    "enables public ipv4 network for agents",
		Sources:  cli.EnvVars("WOODPECKER_HETZNERCLOUD_PUBLIC_IPV4_ENABLE"),
		Category: category,
	},
	&cli.BoolFlag{
		Name:     "hetznercloud-public-ipv6-enable",
		Value:    true,
		Usage:    "enables public ipv6 network for agents",
		Sources:  cli.EnvVars("WOODPECKER_HETZNERCLOUD_PUBLIC_IPV6_ENABLE"),
		Category: category,
	},
}
