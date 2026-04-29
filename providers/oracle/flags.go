package oracle

import (
	"os"

	"github.com/urfave/cli/v3"
)

const category = "Oracle Cloud"

var ProviderFlags = []cli.Flag{
	// Authentication — either all four explicit fields or none (falls back to ~/.oci/config).
	&cli.StringFlag{
		Name:  "oracle-tenancy-ocid",
		Usage: "Tenancy OCID for OCI API authentication",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_ORACLE_TENANCY_OCID"),
			cli.File(os.Getenv("WOODPECKER_ORACLE_TENANCY_OCID_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "oracle-user-ocid",
		Usage: "User OCID for OCI API authentication",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_ORACLE_USER_OCID"),
			cli.File(os.Getenv("WOODPECKER_ORACLE_USER_OCID_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "oracle-fingerprint",
		Usage: "API key fingerprint for OCI API authentication",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_ORACLE_FINGERPRINT"),
			cli.File(os.Getenv("WOODPECKER_ORACLE_FINGERPRINT_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-private-key-file",
		Usage:    "Path to the PEM-encoded RSA private key for OCI API authentication",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_PRIVATE_KEY_FILE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-region",
		Usage:    "OCI region identifier (e.g. us-ashburn-1)",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_REGION"),
		Category: category,
	},
	// Compute placement
	&cli.StringFlag{
		Name:     "oracle-compartment-id",
		Usage:    "Compartment OCID in which to launch agent instances",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_COMPARTMENT_ID"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-availability-domain",
		Usage:    "Availability domain name (e.g. IiCz:US-ASHBURN-AD-1)",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_AVAILABILITY_DOMAIN"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-subnet-id",
		Usage:    "Subnet OCID for agent instance network interfaces",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_SUBNET_ID"),
		Category: category,
	},
	// Instance configuration
	&cli.StringFlag{
		Name:     "oracle-image-id",
		Usage:    "OCID of the base image for agent instances",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_IMAGE_ID"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-shape",
		Usage:    "Instance shape (e.g. VM.Standard.E4.Flex)",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_SHAPE"),
		Value:    "VM.Standard.E4.Flex",
		Category: category,
	},
	&cli.FloatFlag{
		Name:     "oracle-shape-ocpus",
		Usage:    "Number of OCPUs to allocate (only applies to flexible shapes)",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_SHAPE_OCPUS"),
		Value:    1,
		Category: category,
	},
	&cli.FloatFlag{
		Name:     "oracle-shape-memory-gbs",
		Usage:    "Amount of memory in GB to allocate (only applies to flexible shapes)",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_SHAPE_MEMORY_GBS"),
		Value:    6,
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-ssh-authorized-key",
		Usage:    "SSH public key to inject into agent instances",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_SSH_AUTHORIZED_KEY"),
		Category: category,
	},
}
