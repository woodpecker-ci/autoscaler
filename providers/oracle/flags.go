// Copyright 2024 Woodpecker Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package oracle

import (
	"os"

	"github.com/urfave/cli/v3"
)

const category = "Oracle Cloud"

var ProviderFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "oracle-tenancy-ocid",
		Usage: "Oracle Cloud tenancy OCID",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_ORACLE_TENANCY_OCID"),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "oracle-user-ocid",
		Usage: "Oracle Cloud user OCID",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_ORACLE_USER_OCID"),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "oracle-fingerprint",
		Usage: "Oracle Cloud API key fingerprint",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_ORACLE_FINGERPRINT"),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:  "oracle-private-key",
		Usage: "Oracle Cloud API private key (PEM format)",
		Sources: cli.NewValueSourceChain(
			cli.EnvVar("WOODPECKER_ORACLE_PRIVATE_KEY"),
			cli.File(os.Getenv("WOODPECKER_ORACLE_PRIVATE_KEY_FILE")),
		),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-region",
		Value:    "us-ashburn-1",
		Usage:    "Oracle Cloud region (e.g., us-ashburn-1, eu-frankfurt-1)",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_REGION"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-compartment-ocid",
		Usage:    "Oracle Cloud compartment OCID for resources",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_COMPARTMENT_OCID"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-availability-domain",
		Usage:    "Oracle Cloud availability domain (e.g., Uocm:PHX-AD-1)",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_AVAILABILITY_DOMAIN"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-subnet-ocid",
		Usage:    "Oracle Cloud subnet OCID for instances",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_SUBNET_OCID"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-shape",
		Value:    "VM.Standard.E4.Flex",
		Usage:    "Oracle Cloud instance shape (e.g., VM.Standard.E4.Flex, VM.Standard.A1.Flex)",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_SHAPE"),
		Category: category,
	},
	&cli.IntFlag{
		Name:     "oracle-ocpus",
		Value:    1,
		Usage:    "Number of OCPUs for flexible shapes",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_OCPUS"),
		Category: category,
	},
	&cli.IntFlag{
		Name:     "oracle-memory-gb",
		Value:    4,
		Usage:    "Memory in GB for flexible shapes",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_MEMORY_GB"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-image-ocid",
		Usage:    "Oracle Cloud image OCID (e.g., Oracle Linux or Ubuntu)",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_IMAGE_OCID"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "oracle-ssh-public-key",
		Usage:    "SSH public key for instance access",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_SSH_PUBLIC_KEY"),
		Category: category,
	},
	&cli.IntFlag{
		Name:     "oracle-boot-volume-size-gb",
		Value:    50,
		Usage:    "Boot volume size in GB",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_BOOT_VOLUME_SIZE_GB"),
		Category: category,
	},
	&cli.StringSliceFlag{
		Name:     "oracle-freeform-tags",
		Usage:    "Freeform tags for instances (key=value format)",
		Sources:  cli.EnvVars("WOODPECKER_ORACLE_FREEFORM_TAGS"),
		Category: category,
	},
}
