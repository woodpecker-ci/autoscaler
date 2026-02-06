package oracle

import (
	"os"

	"github.com/urfave/cli/v3"
)

const category = "Oracle Cloud"

var ProviderFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "oracle-tenancy-ocid",
		Usage: "Oracle Cloud tenancy OCID",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_ORACLE_TENANCY_OCID"),
			cli.File(os.Getenv("WOODPECKER_ORACLE_TENANCY_OCID_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "oracle-user-ocid",
		Usage: "Oracle Cloud user OCID",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_ORACLE_USER_OCID"),
			cli.File(os.Getenv("WOODPECKER_ORACLE_USER_OCID_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "oracle-fingerprint",
		Usage: "Oracle Cloud API key fingerprint",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_ORACLE_FINGERPRINT"),
			cli.File(os.Getenv("WOODPECKER_ORACLE_FINGERPRINT_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "oracle-private-key",
		Usage: "Oracle Cloud API private key (PEM format)",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_ORACLE_PRIVATE_KEY"),
			cli.File(os.Getenv("WOODPECKER_ORACLE_PRIVATE_KEY_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-region",
		Value:    "us-phoenix-1",
		Usage:    "Oracle Cloud region (e.g., us-phoenix-1, us-ashburn-1)",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_REGION"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-compartment-ocid",
		Usage:    "Oracle Cloud compartment OCID",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_COMPARTMENT_OCID"),
		Category: category,
		Required: true,
	},
	&cli.StringFlag{
		Name:     "oracle-availability-domain",
		Usage:    "Oracle Cloud availability domain",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_AVAILABILITY_DOMAIN"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-subnet-ocid",
		Usage:    "Oracle Cloud subnet OCID",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_SUBNET_OCID"),
		Category: category,
		Required: true,
	},
	&cli.StringFlag{
		Name:     "oracle-shape",
		Value:    "VM.Standard.E4.Flex",
		Usage:    "Oracle Cloud instance shape",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_SHAPE"),
		Category: category,
	},
	&cli.IntFlag{
		Name:     "oracle-ocpus",
		Value:    1,
		Usage:    "Number of OCPUs for flex shapes",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_OCPUS"),
		Category: category,
	},
	&cli.IntFlag{
		Name:     "oracle-memory-gb",
		Value:    4, //nolint:mnd
		Usage:    "Memory in GB for flex shapes",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_MEMORY_GB"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-image-ocid",
		Usage:    "Oracle Cloud image OCID (defaults to latest Ubuntu)",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_IMAGE_OCID"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-ssh-public-key",
		Usage:    "SSH public key for instance access",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_SSH_PUBLIC_KEY"),
		Category: category,
	},
}
