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
		Value:    "nyc3",
		Usage:    "DigitalOcean region slug",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_REGION"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-size",
		Value:    "s-1vcpu-1gb",
		Usage:    "DigitalOcean droplet size slug",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_SIZE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-image",
		Value:    "ubuntu-22-04-x64",
		Usage:    "DigitalOcean droplet image slug",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_IMAGE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-ssh-key",
		Usage:    "Name of DigitalOcean SSH key",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_SSH_KEY"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "digitalocean-tags",
		Usage:    "Additional DigitalOcean tags",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_TAGS"),
		Category: category,
	},
}
