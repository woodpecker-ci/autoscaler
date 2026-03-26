package azure

import (
	"os"

	"github.com/urfave/cli/v3"
)

const category = "Azure"

var ProviderFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "azure-subscription-id",
		Usage: "Azure subscription ID",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_AZURE_SUBSCRIPTION_ID"),
			cli.File(os.Getenv("WOODPECKER_AZURE_SUBSCRIPTION_ID_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "azure-resource-group",
		Usage:    "Azure resource group name",
		Sources:  cli.EnvVars("WOODPECKER_AZURE_RESOURCE_GROUP"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "azure-location",
		Value:    "eastus",
		Usage:    "Azure region/location",
		Sources:  cli.EnvVars("WOODPECKER_AZURE_LOCATION"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "azure-vm-size",
		Value:    "Standard_B2s",
		Usage:    "Azure VM size",
		Sources:  cli.EnvVars("WOODPECKER_AZURE_VM_SIZE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "azure-image-publisher",
		Value:    "Canonical",
		Usage:    "Azure VM image publisher",
		Sources:  cli.EnvVars("WOODPECKER_AZURE_IMAGE_PUBLISHER"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "azure-image-offer",
		Value:    "ubuntu-24_04-lts",
		Usage:    "Azure VM image offer",
		Sources:  cli.EnvVars("WOODPECKER_AZURE_IMAGE_OFFER"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "azure-image-sku",
		Value:    "server",
		Usage:    "Azure VM image SKU",
		Sources:  cli.EnvVars("WOODPECKER_AZURE_IMAGE_SKU"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "azure-image-version",
		Value:    "latest",
		Usage:    "Azure VM image version",
		Sources:  cli.EnvVars("WOODPECKER_AZURE_IMAGE_VERSION"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "azure-admin-username",
		Value:    "woodpecker",
		Usage:    "Azure VM admin username",
		Sources:  cli.EnvVars("WOODPECKER_AZURE_ADMIN_USERNAME"),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "azure-ssh-public-key",
		Usage: "SSH public key for Azure VM authentication",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_AZURE_SSH_PUBLIC_KEY"),
			cli.File(os.Getenv("WOODPECKER_AZURE_SSH_PUBLIC_KEY_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "azure-nsg-name",
		Usage:    "Azure network security group name (optional)",
		Sources:  cli.EnvVars("WOODPECKER_AZURE_NSG_NAME"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "azure-subnet-id",
		Usage:    "Azure subnet resource ID (optional, creates a new VNet/subnet if not set)",
		Sources:  cli.EnvVars("WOODPECKER_AZURE_SUBNET_ID"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "azure-tags",
		Usage:    "Azure VM tags (key=value)",
		Sources:  cli.EnvVars("WOODPECKER_AZURE_TAGS"),
		Category: category,
	},
	// TODO: Deprecated remove in v2.0
	&cli.StringFlag{
		Name:  "azure-user-data",
		Usage: "Azure userdata template (deprecated)",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_AZURE_USERDATA"),
			cli.File(os.Getenv("WOODPECKER_AZURE_USERDATA_FILE")),
		),
		Category: category,
	},
}
