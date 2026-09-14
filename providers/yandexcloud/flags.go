package yandexcloud

import (
	"os"

	"github.com/urfave/cli/v3"
)

const category = "Yandex Cloud"

const (
	defaultCores        = 2
	defaultCoreFraction = 100
)

var ProviderFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "yandexcloud-folder-id",
		Usage: "Yandex Cloud folder ID",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_YANDEXCLOUD_FOLDER_ID"),
			cli.EnvVar("YC_FOLDER_ID"),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "yandexcloud-subnet-id",
		Usage: "Yandex Cloud subnet ID for agent instances",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_YANDEXCLOUD_SUBNET_ID"),
			cli.EnvVar("YC_SUBNET_ID"),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "yandexcloud-service-account-key-file",
		Usage: "path to a Yandex Cloud service account authorized key JSON file",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_YANDEXCLOUD_SERVICE_ACCOUNT_KEY_FILE"),
			cli.EnvVar("YC_SERVICE_ACCOUNT_KEY_FILE"),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "yandexcloud-iam-token",
		Usage: "Yandex Cloud IAM token",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_YANDEXCLOUD_IAM_TOKEN"),
			cli.EnvVar("YC_IAM_TOKEN"),
			cli.File(os.Getenv("WOODPECKER_YANDEXCLOUD_IAM_TOKEN_FILE")),
		),
		Category: category,
	},
	&cli.BoolFlag{
		Name:  "yandexcloud-use-instance-service-account",
		Usage: "use the service account attached to the VM running the autoscaler",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_YANDEXCLOUD_USE_INSTANCE_SERVICE_ACCOUNT"),
			cli.EnvVar("YC_USE_INSTANCE_SERVICE_ACCOUNT"),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "yandexcloud-image-id",
		Usage:    "Yandex Cloud Compute image ID",
		Sources:  cli.EnvVars("WOODPECKER_YANDEXCLOUD_IMAGE_ID"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "yandexcloud-image-family",
		Usage:    "Yandex Cloud Compute image family",
		Sources:  cli.EnvVars("WOODPECKER_YANDEXCLOUD_IMAGE_FAMILY"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "yandexcloud-image-folder-id",
		Usage:    "folder containing the Yandex Cloud image family",
		Value:    "standard-images",
		Sources:  cli.EnvVars("WOODPECKER_YANDEXCLOUD_IMAGE_FOLDER_ID"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "yandexcloud-platform-id",
		Usage:    "Yandex Cloud Compute platform ID",
		Value:    "standard-v3",
		Sources:  cli.EnvVars("WOODPECKER_YANDEXCLOUD_PLATFORM_ID"),
		Category: category,
	},
	&cli.IntFlag{
		Name:     "yandexcloud-cores",
		Usage:    "number of vCPUs for agent instances",
		Value:    defaultCores,
		Sources:  cli.EnvVars("WOODPECKER_YANDEXCLOUD_CORES"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "yandexcloud-memory",
		Usage:    "memory for agent instances, for example 4GiB or 4294967296",
		Value:    "4GiB",
		Sources:  cli.EnvVars("WOODPECKER_YANDEXCLOUD_MEMORY"),
		Category: category,
	},
	&cli.IntFlag{
		Name:     "yandexcloud-core-fraction",
		Usage:    "baseline CPU performance percentage",
		Value:    defaultCoreFraction,
		Sources:  cli.EnvVars("WOODPECKER_YANDEXCLOUD_CORE_FRACTION"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "yandexcloud-disk-type",
		Usage:    "boot disk type",
		Value:    "network-hdd",
		Sources:  cli.EnvVars("WOODPECKER_YANDEXCLOUD_DISK_TYPE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "yandexcloud-disk-size",
		Usage:    "boot disk size, for example 20GiB or 21474836480",
		Value:    "20GiB",
		Sources:  cli.EnvVars("WOODPECKER_YANDEXCLOUD_DISK_SIZE"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "yandexcloud-security-group-ids",
		Usage:    "security group IDs for agent network interfaces",
		Sources:  cli.EnvVars("WOODPECKER_YANDEXCLOUD_SECURITY_GROUP_IDS"),
		Category: category,
	},
	&cli.BoolFlag{
		Name:     "yandexcloud-public-ipv4-enable",
		Usage:    "assign a public IPv4 address to agent instances",
		Value:    true,
		Sources:  cli.EnvVars("WOODPECKER_YANDEXCLOUD_PUBLIC_IPV4_ENABLE"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "yandexcloud-labels",
		Usage:    "additional VM labels as key=value pairs",
		Sources:  cli.EnvVars("WOODPECKER_YANDEXCLOUD_LABELS"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "yandexcloud-operation-timeout",
		Usage:    "timeout for Yandex Cloud create and delete operations",
		Value:    "5m",
		Sources:  cli.EnvVars("WOODPECKER_YANDEXCLOUD_OPERATION_TIMEOUT"),
		Category: category,
	},
}
