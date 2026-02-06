package equinixmetal

import (
	"os"

	"github.com/urfave/cli/v3"
)

const category = "Equinix Metal"

var ProviderFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "equinixmetal-api-token",
		Usage: "Equinix Metal API token",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_EQUINIXMETAL_API_TOKEN"),
			cli.File(os.Getenv("WOODPECKER_EQUINIXMETAL_API_TOKEN_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "equinixmetal-project-id",
		Usage:    "Equinix Metal project ID",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_PROJECT_ID"),
		Category: category,
		Required: true,
	},
	&cli.StringFlag{
		Name:     "equinixmetal-metro",
		Value:    "sv",
		Usage:    "Equinix Metal metro (e.g., sv, da, ny, am)",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_METRO"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "equinixmetal-plan",
		Value:    "c3.small.x86",
		Usage:    "Equinix Metal server plan",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_PLAN"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "equinixmetal-os",
		Value:    "ubuntu_22_04",
		Usage:    "Equinix Metal operating system",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_OS"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "equinixmetal-ssh-keys",
		Usage:    "Equinix Metal SSH key IDs or labels",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_SSH_KEYS"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "equinixmetal-tags",
		Usage:    "Equinix Metal device tags",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_TAGS"),
		Category: category,
	},
	&cli.BoolFlag{
		Name:     "equinixmetal-spot-instance",
		Value:    false,
		Usage:    "Use spot instances for cost savings",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_SPOT_INSTANCE"),
		Category: category,
	},
	&cli.Float64Flag{
		Name:     "equinixmetal-spot-price-max",
		Value:    0.0,
		Usage:    "Maximum spot price (0 for market price)",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_SPOT_PRICE_MAX"),
		Category: category,
	},
}
