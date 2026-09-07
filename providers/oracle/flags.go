package oracle

import (
	"os"

	"github.com/urfave/cli/v3"
)

const category = "Oracle Cloud"

var ProviderFlags = []cli.Flag{
	// API key authentication (used when set, otherwise the SDK config file is used).
	&cli.StringFlag{
		Name:     "oracle-tenancy-id",
		Usage:    "OCI tenancy OCID (API key authentication)",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_TENANCY_ID"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-user-id",
		Usage:    "OCI user OCID (API key authentication)",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_USER_ID"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-fingerprint",
		Usage:    "OCI API key fingerprint (API key authentication)",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_FINGERPRINT"),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "oracle-private-key",
		Usage: "OCI API private key in PEM format (API key authentication)",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_ORACLE_PRIVATE_KEY"),
			cli.File(os.Getenv("WOODPECKER_ORACLE_PRIVATE_KEY_FILE")),
		),
		Category: category,
	},
	// SDK config file authentication.
	&cli.StringFlag{
		Name:     "oracle-config-file",
		Usage:    "path to an OCI SDK config file (default: ~/.oci/config)",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_CONFIG_FILE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-profile",
		Usage:    "profile to use from the OCI SDK config file",
		Value:    "DEFAULT",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_PROFILE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-region",
		Usage:    "OCI region identifier, e.g. eu-frankfurt-1 (required for API key authentication, otherwise overrides the config file)",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_REGION"),
		Category: category,
	},
	// Instance placement.
	&cli.StringFlag{
		Name:     "oracle-compartment-id",
		Usage:    "OCI compartment OCID the agents are created in",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_COMPARTMENT_ID"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-availability-domain",
		Usage:    "OCI availability domain the agents are created in, e.g. Uocm:EU-FRANKFURT-1-AD-1",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_AVAILABILITY_DOMAIN"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-subnet-id",
		Usage:    "OCI subnet OCID the agents are attached to",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_SUBNET_ID"),
		Category: category,
	},
	// Instance shape.
	&cli.StringFlag{
		Name:     "oracle-shape",
		Usage:    "OCI compute shape",
		Value:    "VM.Standard.E4.Flex",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_SHAPE"),
		Category: category,
	},
	&cli.FloatFlag{
		Name:     "oracle-ocpus",
		Usage:    "number of OCPUs (only applied to flexible .Flex shapes)",
		Value:    defaultOcpus,
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_OCPUS"),
		Category: category,
	},
	&cli.FloatFlag{
		Name:     "oracle-memory-gbs",
		Usage:    "amount of memory in GB (only applied to flexible .Flex shapes)",
		Value:    defaultMemoryGBs,
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_MEMORY_GBS"),
		Category: category,
	},
	// Image.
	&cli.StringFlag{
		Name:     "oracle-image-id",
		Usage:    "OCI image OCID (takes precedence over operating system lookup)",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_IMAGE_ID"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-operating-system",
		Usage:    "operating system of the platform image to use",
		Value:    "Canonical Ubuntu",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_OPERATING_SYSTEM"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-operating-system-version",
		Usage:    "operating system version of the platform image to use",
		Value:    "24.04",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_OPERATING_SYSTEM_VERSION"),
		Category: category,
	},
	// Misc.
	&cli.StringFlag{
		Name:  "oracle-ssh-authorized-keys",
		Usage: "SSH public keys added to the agents, one per line",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_ORACLE_SSH_AUTHORIZED_KEYS"),
			cli.File(os.Getenv("WOODPECKER_ORACLE_SSH_AUTHORIZED_KEYS_FILE")),
		),
		Category: category,
	},
	&cli.BoolFlag{
		Name:     "oracle-assign-public-ip",
		Usage:    "assign a public IPv4 address to the agents",
		Value:    true,
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_ASSIGN_PUBLIC_IP"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "oracle-freeform-tags",
		Usage:    "additional freeform tags for the agents as key=value pairs",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_FREEFORM_TAGS"),
		Category: category,
	},
}
