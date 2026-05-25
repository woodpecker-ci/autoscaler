package digitalocean

import (
	"os"

	"github.com/urfave/cli/v3"
)

const category = "DigitalOcean"

var ProviderFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "digitalocean-api-token",
		Usage: "DigitalOcean api token",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_DIGITALOCEAN_API_TOKEN"),
			cli.File(os.Getenv("WOODPECKER_DIGITALOCEAN_API_TOKEN_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-region",
		Usage:    "DigitalOcean region slug",
		Value:    "nyc1",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_REGION"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-size",
		Usage:    "DigitalOcean droplet size slug",
		Value:    "s-1vcpu-1gb",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_SIZE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-image",
		Usage:    "DigitalOcean image slug",
		Value:    "ubuntu-24-04-x64",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_IMAGE"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "digitalocean-ssh-keys",
		Usage:    "DigitalOcean SSH key IDs or fingerprints",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_SSH_KEYS"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "digitalocean-tags",
		Usage:    "DigitalOcean droplet tags",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_TAGS"),
		Category: category,
	},
	&cli.BoolFlag{
		Name:     "digitalocean-public-ipv6-enable",
		Value:    true,
		Usage:    "enables public ipv6 network for agents",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_PUBLIC_IPV6_ENABLE"),
		Category: category,
	},
	&cli.BoolFlag{
		Name:     "digitalocean-private-networking-enable",
		Value:    true,
		Usage:    "enables private networking for agents",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_PRIVATE_NETWORKING_ENABLE"),
		Category: category,
	},
	&cli.BoolFlag{
		Name:     "digitalocean-monitoring-enable",
		Value:    false,
		Usage:    "enables monitoring for agents",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_MONITORING_ENABLE"),
		Category: category,
	},
	&cli.BoolFlag{
		Name:     "digitalocean-backups-enable",
		Value:    false,
		Usage:    "enables backups for agents",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_BACKUPS_ENABLE"),
		Category: category,
	},
}
