package oracle

import (
	"os"

	"github.com/urfave/cli/v3"
)

const category = "Oracle Cloud"

//nolint:mnd
var ProviderFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "oracle-tenancy-ocid",
		Usage: "Oracle Cloud tenancy OCID",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_ORACLE_TENANCY_OCID"),
			cli.EnvVar("OCI_TENANCY_OCID"),
			cli.File(os.Getenv("WOODPECKER_ORACLE_TENANCY_OCID_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "oracle-user-ocid",
		Usage: "Oracle Cloud user OCID the API key belongs to",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_ORACLE_USER_OCID"),
			cli.EnvVar("OCI_USER_OCID"),
			cli.File(os.Getenv("WOODPECKER_ORACLE_USER_OCID_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "oracle-fingerprint",
		Usage: "Fingerprint of the uploaded API signing key",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_ORACLE_FINGERPRINT"),
			cli.EnvVar("OCI_FINGERPRINT"),
			cli.File(os.Getenv("WOODPECKER_ORACLE_FINGERPRINT_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "oracle-private-key",
		Usage: "PEM encoded API signing key",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_ORACLE_PRIVATE_KEY"),
			cli.File(os.Getenv("WOODPECKER_ORACLE_PRIVATE_KEY_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "oracle-private-key-passphrase",
		Usage: "Passphrase of the API signing key, if it is encrypted",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_ORACLE_PRIVATE_KEY_PASSPHRASE"),
			cli.File(os.Getenv("WOODPECKER_ORACLE_PRIVATE_KEY_PASSPHRASE_FILE")),
		),
		Category: category,
	},
	&cli.BoolFlag{
		Name:     "oracle-use-instance-principal",
		Usage:    "Authenticate as the instance the autoscaler runs on instead of using an API signing key",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_USE_INSTANCE_PRINCIPAL"),
		Category: category,
	},
	&cli.StringFlag{
		Name: "oracle-region",
		Usage: "Region to deploy the agents in (e.g. \"eu-frankfurt-1\"). " +
			"Required for API signing key authentication, defaults to the region of the autoscaler instance otherwise.",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_ORACLE_REGION"),
			cli.EnvVar("OCI_REGION"),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-compartment-ocid",
		Usage:    "OCID of the compartment the agents are created in",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_COMPARTMENT_OCID"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-subnet-ocid",
		Usage:    "OCID of the subnet the agents are attached to",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_SUBNET_OCID"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name: "oracle-availability-domains",
		Usage: "Ordered list of availability domain names to try (e.g. \"Uocm:EU-FRANKFURT-1-AD-1\"). " +
			"On capacity errors the next one is tried. Defaults to every availability domain of the compartment.",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_AVAILABILITY_DOMAINS"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-shape",
		Usage:    "Compute shape of the agents",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_SHAPE"),
		Value:    "VM.Standard.E4.Flex",
		Category: category,
	},
	&cli.Float32Flag{
		Name:     "oracle-ocpus",
		Usage:    "OCPUs to assign, only used by flexible shapes",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_OCPUS"),
		Value:    1,
		Category: category,
	},
	&cli.Float32Flag{
		Name:     "oracle-memory-in-gbs",
		Usage:    "Memory in GB to assign, only used by flexible shapes",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_MEMORY_IN_GBS"),
		Value:    8,
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-image-ocid",
		Usage:    "OCID of the image to boot from, skips image lookup by operating system",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_IMAGE_OCID"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-image-operating-system",
		Usage:    "Operating system to pick the most recent image for, when no image OCID is set",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_IMAGE_OPERATING_SYSTEM"),
		Value:    "Canonical Ubuntu",
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-image-operating-system-version",
		Usage:    "Operating system version to pick the most recent image for, when no image OCID is set",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_IMAGE_OPERATING_SYSTEM_VERSION"),
		Value:    "24.04",
		Category: category,
	},
	&cli.Int64Flag{
		Name:     "oracle-boot-volume-size",
		Usage:    "Boot volume size in GB, Oracle Cloud rejects anything below 50",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_BOOT_VOLUME_SIZE"),
		Value:    minBootVolumeGB,
		Category: category,
	},
	&cli.BoolFlag{
		Name:     "oracle-assign-public-ip",
		Usage:    "Assign a public IPv4 address to the agents",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_ASSIGN_PUBLIC_IP"),
		Value:    true,
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-ssh-key",
		Usage:    "Public SSH key to add to the agents",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_SSH_KEY"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "oracle-tags",
		Usage:    "Comma separated list of freeform tags in \"key=value\" format to add to the agents",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_TAGS"),
		Category: category,
	},
}
