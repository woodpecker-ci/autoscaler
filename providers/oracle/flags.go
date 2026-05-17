package oracle

import (
	"os"

	"github.com/urfave/cli/v3"
)

const category = "Oracle Cloud"

//nolint:mnd
var ProviderFlags = []cli.Flag{
	&cli.StringFlag{
		Name:     "oracle-config-file",
		Usage:    "OCI config file path; defaults to the SDK default config provider",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_CONFIG_FILE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-profile",
		Value:    "DEFAULT",
		Usage:    "OCI config profile name",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_PROFILE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-region",
		Usage:    "OCI region override, e.g. eu-frankfurt-1",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_REGION"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-compartment-id",
		Usage:    "OCI compartment OCID for autoscaler instances",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_COMPARTMENT_ID"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-availability-domain",
		Usage:    "OCI availability domain where agents are launched",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_AVAILABILITY_DOMAIN"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-subnet-id",
		Usage:    "OCI subnet OCID used for agent VNICs",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_SUBNET_ID"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-image-id",
		Usage:    "OCI image OCID used to boot agents",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_IMAGE_ID"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-shape",
		Value:    "VM.Standard.E4.Flex",
		Usage:    "OCI compute shape",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_SHAPE"),
		Category: category,
	},
	&cli.FloatFlag{
		Name:     "oracle-ocpus",
		Value:    1,
		Usage:    "OCPU count for flex shapes",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_OCPUS"),
		Category: category,
	},
	&cli.FloatFlag{
		Name:     "oracle-memory-gbs",
		Value:    6,
		Usage:    "memory in GB for flex shapes",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_MEMORY_GBS"),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "oracle-ssh-authorized-key",
		Usage: "SSH public key added to launched agents",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_ORACLE_SSH_AUTHORIZED_KEY"),
			cli.File(os.Getenv("WOODPECKER_ORACLE_SSH_AUTHORIZED_KEY_FILE")),
		),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "oracle-freeform-tags",
		Usage:    "additional OCI freeform tags as key=value pairs",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_FREEFORM_TAGS"),
		Category: category,
	},
	&cli.BoolFlag{
		Name:     "oracle-assign-public-ip",
		Value:    true,
		Usage:    "assign public IPv4 addresses to agent VNICs",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_ASSIGN_PUBLIC_IP"),
		Category: category,
	},
}
