package oraclecloud

import "github.com/urfave/cli/v3"

const Category = "Oracle Cloud"

var ProviderFlags = []cli.Flag{
	&cli.StringFlag{
		Name:     "oraclecloud-tenancy-ocid",
		Usage:    "Oracle Cloud tenancy OCID",
		Sources:  cli.EnvVars("WOODPECKER_ORACLECLOUD_TENANCY_OCID"),
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "oraclecloud-user-ocid",
		Usage:    "Oracle Cloud user OCID",
		Sources:  cli.EnvVars("WOODPECKER_ORACLECLOUD_USER_OCID"),
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "oraclecloud-fingerprint",
		Usage:    "Oracle Cloud API key fingerprint",
		Sources:  cli.EnvVars("WOODPECKER_ORACLECLOUD_FINGERPRINT"),
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "oraclecloud-private-key",
		Usage:    "Oracle Cloud API private key file path",
		Sources:  cli.EnvVars("WOODPECKER_ORACLECLOUD_PRIVATE_KEY"),
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "oraclecloud-region",
		Usage:    "Oracle Cloud region (e.g. us-ashburn-1)",
		Sources:  cli.EnvVars("WOODPECKER_ORACLECLOUD_REGION"),
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "oraclecloud-compartment-ocid",
		Usage:    "Oracle Cloud compartment OCID",
		Sources:  cli.EnvVars("WOODPECKER_ORACLECLOUD_COMPARTMENT_OCID"),
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "oraclecloud-shape",
		Usage:    "Instance shape (e.g. VM.Standard.E2.1)",
		Sources:  cli.EnvVars("WOODPECKER_ORACLECLOUD_SHAPE"),
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "oraclecloud-image-ocid",
		Usage:    "Image OCID",
		Sources:  cli.EnvVars("WOODPECKER_ORACLECLOUD_IMAGE_OCID"),
		Category: Category,
	},
	&cli.StringSliceFlag{
		Name:     "oraclecloud-subnet-ocids",
		Usage:    "Subnet OCIDs for instance",
		Sources:  cli.EnvVars("WOODPECKER_ORACLECLOUD_SUBNET_OCIDS"),
		Category: Category,
	},
	&cli.StringSliceFlag{
		Name:     "oraclecloud-ssh-keys",
		Usage:    "SSH public keys for instance access",
		Sources:  cli.EnvVars("WOODPECKER_ORACLECLOUD_SSH_KEYS"),
		Category: Category,
	},
}
