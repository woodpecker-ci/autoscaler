// Copyright 2024 Woodpecker Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
		Usage:    "DigitalOcean region (e.g., nyc1, sfo1, ams3)",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_REGION"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-size",
		Value:    "s-1vcpu-1gb",
		Usage:    "DigitalOcean droplet size (e.g., s-1vcpu-1gb, s-2vcpu-2gb)",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_SIZE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-image",
		Value:    "ubuntu-22-04-x64",
		Usage:    "DigitalOcean OS image slug (e.g., ubuntu-22-04-x64, ubuntu-24-04-x64)",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_IMAGE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-ssh-key",
		Usage:    "DigitalOcean SSH key fingerprint or ID",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_SSH_KEY"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "digitalocean-tags",
		Usage:    "DigitalOcean droplet tags",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_TAGS"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "digitalocean-vpc-uuid",
		Usage:    "DigitalOcean VPC UUID (optional)",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_VPC_UUID"),
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
		Name:     "digitalocean-firewall-id",
		Usage:    "DigitalOcean firewall ID to apply to droplets (optional)",
		Sources:  cli.EnvVars("WOODPECKER_DIGITALOCEAN_FIREWALL_ID"),
		Category: category,
	},
}
