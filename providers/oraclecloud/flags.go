package oraclecloud

import (
	"os"

	"github.com/urfave/cli/v3"
)

const category = "Oracle Cloud Infrastructure"

var ProviderFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "oci-tenancy",
		Usage: "OCID of the tenancy",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_OCI_TENANCY"),
			cli.File(os.Getenv("WOODPECKER_OCI_TENANCY_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "oci-user",
		Usage: "OCID of the user",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_OCI_USER"),
			cli.File(os.Getenv("WOODPECKER_OCI_USER_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oci-region",
		Usage:    "OCI region identifier (e.g. us-ashburn-1)",
		Sources:  cli.EnvVars("WOODPECKER_OCI_REGION"),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "oci-fingerprint",
		Usage: "fingerprint of the API signing key",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_OCI_FINGERPRINT"),
			cli.File(os.Getenv("WOODPECKER_OCI_FINGERPRINT_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "oci-private-key",
		Usage: "PEM-encoded RSA private key for API authentication",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_OCI_PRIVATE_KEY"),
			cli.File(os.Getenv("WOODPECKER_OCI_PRIVATE_KEY_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oci-compartment-id",
		Usage:    "OCID of the compartment to launch instances in",
		Sources:  cli.EnvVars("WOODPECKER_OCI_COMPARTMENT_ID"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oci-availability-domain",
		Usage:    "availability domain for instances (e.g. Uocm:PHX-AD-1)",
		Sources:  cli.EnvVars("WOODPECKER_OCI_AVAILABILITY_DOMAIN"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oci-image-id",
		Usage:    "OCID of the image to use for agent instances",
		Sources:  cli.EnvVars("WOODPECKER_OCI_IMAGE_ID"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oci-shape",
		Value:    "VM.Standard2.1",
		Usage:    "compute shape for agent instances",
		Sources:  cli.EnvVars("WOODPECKER_OCI_SHAPE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oci-subnet-id",
		Usage:    "OCID of the subnet to attach agent instances to",
		Sources:  cli.EnvVars("WOODPECKER_OCI_SUBNET_ID"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "oci-tags",
		Usage:    "free-form tags to apply to agent instances (key=value)",
		Sources:  cli.EnvVars("WOODPECKER_OCI_TAGS"),
		Category: category,
	},
}
