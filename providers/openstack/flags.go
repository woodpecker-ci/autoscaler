package openstack

import (
	"os"

	"github.com/urfave/cli/v3"
)

const category = "OpenStack"

var ProviderFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "openstack-auth-url",
		Usage: "OpenStack authentication URL (Keystone endpoint)",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_OPENSTACK_AUTH_URL"),
			cli.File(os.Getenv("WOODPECKER_OPENSTACK_AUTH_URL_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "openstack-username",
		Usage: "OpenStack username",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_OPENSTACK_USERNAME"),
			cli.File(os.Getenv("WOODPECKER_OPENSTACK_USERNAME_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "openstack-password",
		Usage: "OpenStack password",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_OPENSTACK_PASSWORD"),
			cli.File(os.Getenv("WOODPECKER_OPENSTACK_PASSWORD_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "openstack-tenant-name",
		Usage:    "OpenStack tenant/project name",
		Sources:  cli.EnvVars("WOODPECKER_OPENSTACK_TENANT_NAME"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "openstack-domain-name",
		Value:    "Default",
		Usage:    "OpenStack domain name",
		Sources:  cli.EnvVars("WOODPECKER_OPENSTACK_DOMAIN_NAME"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "openstack-region",
		Usage:    "OpenStack region",
		Sources:  cli.EnvVars("WOODPECKER_OPENSTACK_REGION"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "openstack-flavor",
		Usage:    "OpenStack flavor name or ID for the server",
		Sources:  cli.EnvVars("WOODPECKER_OPENSTACK_FLAVOR"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "openstack-image",
		Usage:    "OpenStack image name or ID for the server",
		Sources:  cli.EnvVars("WOODPECKER_OPENSTACK_IMAGE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "openstack-network",
		Usage:    "OpenStack network name or ID to attach the server to",
		Sources:  cli.EnvVars("WOODPECKER_OPENSTACK_NETWORK"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "openstack-security-groups",
		Usage:    "OpenStack security groups for the server",
		Sources:  cli.EnvVars("WOODPECKER_OPENSTACK_SECURITY_GROUPS"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "openstack-keypair",
		Usage:    "OpenStack SSH keypair name",
		Sources:  cli.EnvVars("WOODPECKER_OPENSTACK_KEYPAIR"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "openstack-floating-ip-pool",
		Usage:    "OpenStack floating IP pool name (optional, assigns a public IP if set)",
		Sources:  cli.EnvVars("WOODPECKER_OPENSTACK_FLOATING_IP_POOL"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "openstack-metadata",
		Usage:    "OpenStack server metadata (key=value)",
		Sources:  cli.EnvVars("WOODPECKER_OPENSTACK_METADATA"),
		Category: category,
	},
	// TODO: Deprecated remove in v2.0
	&cli.StringFlag{
		Name:  "openstack-user-data",
		Usage: "OpenStack userdata template (deprecated)",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_OPENSTACK_USERDATA"),
			cli.File(os.Getenv("WOODPECKER_OPENSTACK_USERDATA_FILE")),
		),
		Category: category,
	},
}
