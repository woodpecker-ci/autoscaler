package digitalocean

import (
	"os"

	"github.com/urfave/cli/v3"
)

const category = "DigitalOcean"

var ProviderFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "digitalocean-api-token",
		Usage: "DigitalOcean API token",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_DIGITALOCEAN_API_TOKEN"),
			cli.File(os.Getenv("WOODPECKER_DIGITALOCEAN_API_TOKEN_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-region",
		Value:    "nyc1",
		Usage:    "DigitalOcean region (e.g., nyc1, sfo3, ams3)",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_REGION"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-size",
		Value:    "s-1vcpu-1gb",
		Usage:    "DigitalOcean droplet size",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_SIZE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-image",
		Value:    "ubuntu-22-04-x64",
		Usage:    "DigitalOcean image slug",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_IMAGE"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "digitalocean-ssh-keys",
		Usage:    "DigitalOcean SSH key names or fingerprints",
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
		Name:     "digitalocean-ipv6",
		Value:    false,
		Usage:    "Enable IPv6 for droplets",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_IPV6"),
		Category: category,
	},
	&cli.BoolFlag{
		Name:     "digitalocean-monitoring",
		Value:    true,
		Usage:    "Enable monitoring for droplets",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_MONITORING"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-vpc-uuid",
		Usage:    "DigitalOcean VPC UUID (optional)",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_VPC_UUID"),
		Category: category,
	},
}
