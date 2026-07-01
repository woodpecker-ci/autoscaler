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
		Name:  "openstack-application-credential-id",
		Usage: "OpenStack Application Credential ID",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_OPENSTACK_APPLICATION_CREDENTIAL_ID"),
			cli.File(os.Getenv("WOODPECKER_OPENSTACK_APPLICATION_CREDENTIAL_ID_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "openstack-application-credential-name",
		Usage: "OpenStack Application Credential name",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_OPENSTACK_APPLICATION_CREDENTIAL_NAME"),
			cli.File(os.Getenv("WOODPECKER_OPENSTACK_APPLICATION_CREDENTIAL_NAME_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "openstack-application-credential-secret",
		Usage: "OpenStack Application Credential secret",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_OPENSTACK_APPLICATION_CREDENTIAL_SECRET"),
			cli.File(os.Getenv("WOODPECKER_OPENSTACK_APPLICATION_CREDENTIAL_SECRET_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "openstack-project-name",
		Usage:    "OpenStack project (formerly known as tenant) name",
		Sources:  cli.EnvVars("WOODPECKER_OPENSTACK_PROJECT_NAME"),
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
		Name:     "openstack-flavor-ref",
		Usage:    "OpenStack flavor ID for the agent instances",
		Sources:  cli.EnvVars("WOODPECKER_OPENSTACK_FLAVOR_REF"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "openstack-flavor-name",
		Usage:    "OpenStack flavor name for the agent instances",
		Sources:  cli.EnvVars("WOODPECKER_OPENSTACK_FLAVOR_NAME"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "openstack-image-ref",
		Usage:    "OpenStack image ID for the agent instances",
		Sources:  cli.EnvVars("WOODPECKER_OPENSTACK_IMAGE_REF"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "openstack-image-name",
		Usage:    "OpenStack image name for the agent instances",
		Sources:  cli.EnvVars("WOODPECKER_OPENSTACK_IMAGE_NAME"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "openstack-volume-size",
		Usage:    "Size in GiB for the agent instance volumes. If not set, ephemeral storage based on the selected flavor will be used.",
		Sources:  cli.EnvVars("WOODPECKER_OPENSTACK_VOLUME_SIZE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "openstack-network",
		Usage:    "OpenStack network name or ID to attach the instances to",
		Sources:  cli.EnvVars("WOODPECKER_OPENSTACK_NETWORK"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "openstack-security-groups",
		Usage:    "OpenStack security groups for the agent instances",
		Sources:  cli.EnvVars("WOODPECKER_OPENSTACK_SECURITY_GROUPS"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "openstack-keypair",
		Usage:    "OpenStack SSH keypair name (optional)",
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
		Usage:    "OpenStack server metadata (key=value) (optional)",
		Sources:  cli.EnvVars("WOODPECKER_OPENSTACK_METADATA"),
		Category: category,
	},
}
