package azure

import (
	"os"

	"github.com/urfave/cli/v3"
)

const category = "Azure"

var ProviderFlags = []cli.Flag{
	&cli.StringFlag{
		Name:     "azure-subscription-id",
		Usage:    "Azure subscription ID",
		Sources:  cli.EnvVars("WOODPECKER_AZURE_SUBSCRIPTION_ID", "AZURE_SUBSCRIPTION_ID"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "azure-resource-group",
		Usage:    "Azure resource group all agents are created in",
		Sources:  cli.EnvVars("WOODPECKER_AZURE_RESOURCE_GROUP"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "azure-location",
		Value:    "eastus",
		Usage:    "Azure region",
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
		Name:     "azure-image",
		Value:    "Canonical:ubuntu-24_04-lts:server:latest",
		Usage:    "Azure marketplace image in 'publisher:offer:sku:version' form",
		Sources:  cli.EnvVars("WOODPECKER_AZURE_IMAGE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "azure-subnet-id",
		Usage:    "resource ID of an existing subnet the agent NICs attach to",
		Sources:  cli.EnvVars("WOODPECKER_AZURE_SUBNET_ID"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "azure-admin-username",
		Value:    "woodpecker",
		Usage:    "admin username for the agent VMs",
		Sources:  cli.EnvVars("WOODPECKER_AZURE_ADMIN_USERNAME"),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "azure-ssh-public-key",
		Usage: "SSH public key for the admin user (authorized_keys format)",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_AZURE_SSH_PUBLIC_KEY"),
			cli.File(os.Getenv("WOODPECKER_AZURE_SSH_PUBLIC_KEY_FILE")),
		),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "azure-tags",
		Usage:    "additional tags for the agent VMs in 'key=value' form",
		Sources:  cli.EnvVars("WOODPECKER_AZURE_TAGS"),
		Category: category,
	},
}
