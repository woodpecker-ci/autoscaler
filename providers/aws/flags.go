package aws

import "github.com/urfave/cli/v2"

const Category = "AWS"

var DriverFlags = []cli.Flag{
	// aws
	&cli.StringFlag{
		Name:     "aws-instance-type",
		Usage:    "aws instance type",
		EnvVars:  []string{"WOODPECKER_AWS_INSTANCE_TYPE"},
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "aws-ami-id",
		Usage:    "aws ami id",
		EnvVars:  []string{"WOODPECKER_AWS_AMI_ID"},
		Category: Category,
	},
	&cli.StringSliceFlag{
		Name:     "aws-tags",
		Usage:    "aws tags",
		EnvVars:  []string{"WOODPECKER_AWS_TAGS"},
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "aws-region",
		Usage:    "aws region",
		EnvVars:  []string{"WOODPECKER_AWS_REGION"},
		Category: Category,
	},
	&cli.StringSliceFlag{
		Name:     "aws-subnets",
		Usage:    "aws subnets",
		EnvVars:  []string{"WOODPECKER_AWS_SUBNETS"},
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "aws-iam-instance-profile-arn",
		Usage:    "aws iam instance profile arn",
		EnvVars:  []string{"WOODPECKER_AWS_IAM_INSTANCE_PROFILE_ARN"},
		Category: Category,
	},
	&cli.StringSliceFlag{
		Name:     "aws-security-groups",
		Usage:    "aws security groups",
		EnvVars:  []string{"WOODPECKER_AWS_SECURITY_GROUPS"},
		Category: Category,
	},
	&cli.BoolFlag{
		Name:     "aws-use-spot-instances",
		Usage:    "use spot instances",
		EnvVars:  []string{"WOODPECKER_AWS_USE_SPOT_INSTANCES"},
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "aws-ssh-key-name",
		Usage:    "ssh keypair name",
		EnvVars:  []string{"WOODPECKER_AWS_SSH_KEYNAME"},
		Category: Category,
	},
}
