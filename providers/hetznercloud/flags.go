package hetznercloud

import (
	"os"

	"github.com/urfave/cli/v2"
)

const category = "Hetzner Cloud"

var ProviderFlags = []cli.Flag{
	&cli.StringFlag{
		Name:     "hetznercloud-api-token",
		Usage:    "hetzner cloud api token",
		EnvVars:  []string{"WOODPECKER_HETZNERCLOUD_API_TOKEN"},
		FilePath: os.Getenv("WOODPECKER_HETZNERCLOUD_API_TOKEN_FILE"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "hetznercloud-server-type",
		Value:    cli.NewStringSlice("cx11:nbg1"),
		Usage:    "hetzner cloud server type",
		EnvVars:  []string{"WOODPECKER_HETZNERCLOUD_SERVER_TYPE"},
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "hetznercloud-fallback-server-types",
		Usage:    "hetzner cloud fallback server types",
		EnvVars:  []string{"WOODPECKER_HETZNERCLOUD_FALLBACK_SERVER_TYPES"},
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "hetznercloud-ssh-keys",
		Usage:    "names of hetzner cloud ssh keys",
		EnvVars:  []string{"WOODPECKER_HETZNERCLOUD_SSH_KEYS"},
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
	&cli.StringSliceFlag{
		Name:     "hetznercloud-firewalls",
		Usage:    "names of hetzner cloud firewalls",
		EnvVars:  []string{"WOODPECKER_HETZNERCLOUD_FIREWALLS"},
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "hetznercloud-networks",
		Usage:    "names of hetzner cloud networks",
		EnvVars:  []string{"WOODPECKER_HETZNERCLOUD_NETWORKS"},
		Category: category,
	},
	&cli.BoolFlag{
		Name:     "hetznercloud-public-ipv4-enable",
		Value:    true,
		Usage:    "enables public ipv4 network for agents",
		EnvVars:  []string{"WOODPECKER_HETZNERCLOUD_PUBLIC_IPV4_ENABLE"},
		Category: category,
	},
	&cli.BoolFlag{
		Name:     "hetznercloud-public-ipv6-enable",
		Value:    true,
		Usage:    "enables public ipv6 network for agents",
		EnvVars:  []string{"WOODPECKER_HETZNERCLOUD_PUBLIC_IPV6_ENABLE"},
		Category: category,
	},
}
