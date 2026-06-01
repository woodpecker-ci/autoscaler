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
		Value:    "da",
		Usage:    "Equinix Metal metro (e.g. da, sv, ny, am)",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_METRO"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "equinixmetal-plan",
		Value:    "c3.small.x86",
		Usage:    "Equinix Metal device plan (e.g. c3.small.x86, m3.small.x86)",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_PLAN"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "equinixmetal-os",
		Value:    "ubuntu_22_04",
		Usage:    "Equinix Metal operating system slug",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_OS"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "equinixmetal-tags",
		Usage:    "Equinix Metal device tags",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_TAGS"),
		Category: category,
	},
}
