package equinixmetal

import "github.com/urfave/cli/v3"

const Category = "Equinix Metal"

var ProviderFlags = []cli.Flag{
	// equinixmetal
	&cli.StringFlag{
		Name:     "equinixmetal-project-id",
		Usage:    "Equinix Metal project ID",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_PROJECT_ID"),
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "equinixmetal-api-token",
		Usage:    "Equinix Metal API token",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_API_TOKEN", "METAL_AUTH_TOKEN"),
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "equinixmetal-plan",
		Usage:    "Server plan (e.g. c3.small.x86)",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_PLAN"),
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "equinixmetal-metro",
		Usage:    "Equinix Metal metro (e.g. da, ny, sv)",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_METRO"),
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "equinixmetal-os",
		Usage:    "Operating system (e.g. ubuntu_22_04)",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_OS"),
		Category: Category,
	},
	&cli.StringSliceFlag{
		Name:     "equinixmetal-tags",
		Usage:    "Additional tags for servers",
		Sources:  cli.EnvVars("WOODPECKER_EQUINIXMETAL_TAGS"),
		Category: Category,
	},
}
