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
	},
	&cli.StringFlag{
		Name:     "equinixmetal-metro",
		Usage:    "Equinix Metal metro code (mutually exclusive with facility)",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_METRO"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "equinixmetal-facility",
		Usage:    "Equinix Metal facility code(s) (mutually exclusive with metro)",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_FACILITY"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "equinixmetal-plan",
		Usage:    "Equinix Metal server plan slug",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_PLAN"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "equinixmetal-operating-system",
		Value:    "ubuntu_22_04",
		Usage:    "Equinix Metal operating system slug",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_OPERATING_SYSTEM"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "equinixmetal-billing-cycle",
		Value:    "hourly",
		Usage:    "Equinix Metal billing cycle",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_BILLING_CYCLE"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "equinixmetal-tags",
		Usage:    "additional Equinix Metal device tags",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_TAGS"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "equinixmetal-project-ssh-keys",
		Usage:    "Equinix Metal project SSH key UUIDs to install on created devices",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_PROJECT_SSH_KEYS"),
		Category: category,
	},
	&cli.BoolFlag{
		Name:     "equinixmetal-spot-instance",
		Usage:    "use Equinix Metal spot instances",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_SPOT_INSTANCE"),
		Category: category,
	},
	&cli.Float64Flag{
		Name:     "equinixmetal-spot-price-max",
		Usage:    "maximum spot price when using spot instances",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_SPOT_PRICE_MAX"),
		Category: category,
	},
	// TODO: Deprecated remove in v2.0
	&cli.StringFlag{
		Name:  "equinixmetal-user-data",
		Usage: "Equinix Metal userdata template (deprecated)",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_EQUINIXMETAL_USERDATA"),
			cli.File(os.Getenv("WOODPECKER_EQUINIXMETAL_USERDATA_FILE")),
		),
		Category: category,
	},
}
