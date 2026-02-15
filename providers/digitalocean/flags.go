package digitalocean

import "github.com/urfave/cli/v3"

const Category = "DigitalOcean"

var ProviderFlags = []cli.Flag{
	// digitalocean
	&cli.StringFlag{
		Name:     "digitalocean-instance-type",
		Usage:    "Droplet size slug (e.g. s-1vcpu-1gb)",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_INSTANCE_TYPE"),
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-image",
		Usage:    "Droplet image (e.g. ubuntu-22-04-x64)",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_IMAGE"),
		Category: Category,
	},
	&cli.StringSliceFlag{
		Name:     "digitalocean-tags",
		Usage:    "additional tags for your droplets",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_TAGS"),
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-region",
		Usage:    "DigitalOcean region (e.g. nyc1)",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_REGION"),
		Category: Category,
	},
	&cli.StringSliceFlag{
		Name:     "digitalocean-ssh-keys",
		Usage:    "SSH key fingerprints to add to droplets",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_SSH_KEYS"),
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-token",
		Usage:    "DigitalOcean API token",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_TOKEN", "DIGITALOCEAN_TOKEN"),
		Category: Category,
	},
}
