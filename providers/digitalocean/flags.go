package digitalocean

import (
	"os"

	"github.com/urfave/cli/v3"
)

const category = "DigitalOcean"

var ProviderFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "digitalocean-token",
		Usage: "DigitalOcean API token",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_AUTOSCALER_DIGITALOCEAN_TOKEN"),
			cli.File(os.Getenv("WOODPECKER_AUTOSCALER_DIGITALOCEAN_TOKEN_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-region",
		Value:    "nyc1",
		Usage:    "DigitalOcean region slug (e.g. nyc1, sfo3, ams3)",
		Sources:  cli.EnvVars("WOODPECKER_AUTOSCALER_DIGITALOCEAN_REGION"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-size",
		Value:    "s-1vcpu-1gb",
		Usage:    "DigitalOcean Droplet size slug (e.g. s-1vcpu-1gb, s-2vcpu-4gb)",
		Sources:  cli.EnvVars("WOODPECKER_AUTOSCALER_DIGITALOCEAN_SIZE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-image",
		Value:    "ubuntu-22-04-x64",
		Usage:    "DigitalOcean Droplet image slug or ID (e.g. ubuntu-22-04-x64)",
		Sources:  cli.EnvVars("WOODPECKER_AUTOSCALER_DIGITALOCEAN_IMAGE"),
		Category: category,
	},
	&cli.BoolFlag{
		Name:     "digitalocean-ipv6",
		Value:    false,
		Usage:    "enable IPv6 networking on Droplets",
		Sources:  cli.EnvVars("WOODPECKER_AUTOSCALER_DIGITALOCEAN_IPV6"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "digitalocean-ssh-keys",
		Usage:    "SSH key fingerprints or IDs to add to Droplets",
		Sources:  cli.EnvVars("WOODPECKER_AUTOSCALER_DIGITALOCEAN_SSH_KEYS"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "digitalocean-tags",
		Usage:    "additional tags to apply to Droplets",
		Sources:  cli.EnvVars("WOODPECKER_AUTOSCALER_DIGITALOCEAN_TAGS"),
		Category: category,
	},
}
