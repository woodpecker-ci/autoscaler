package aws

import "github.com/urfave/cli/v2"

const Category = "AWS"

var DriverFlags = []cli.Flag{
	// aws
	&cli.StringFlag{
		Name:     "aws-instance-type",
		Usage:    "EC2 instance type",
		EnvVars:  []string{"WOODPECKER_AWS_INSTANCE_TYPE"},
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "aws-ami-id",
		Usage:    "AMI id",
		EnvVars:  []string{"WOODPECKER_AWS_AMI_ID"},
		Category: Category,
	},
	&cli.StringSliceFlag{
		Name:     "aws-tags",
		Usage:    "additional tags for your EC2 instances",
		EnvVars:  []string{"WOODPECKER_AWS_TAGS"},
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "aws-region",
		Usage:    "AWS region",
		EnvVars:  []string{"WOODPECKER_AWS_REGION"},
		Category: Category,
	},
	&cli.StringSliceFlag{
		Name:     "aws-subnets",
		Usage:    "VPC subnets IDs, e.g. subnet-0987a87c8b37348ef",
		EnvVars:  []string{"WOODPECKER_AWS_SUBNETS"},
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "aws-iam-instance-profile-arn",
		Usage:    "IAM instance profile ARN",
		EnvVars:  []string{"WOODPECKER_AWS_IAM_INSTANCE_PROFILE_ARN"},
		Category: Category,
	},
	&cli.StringSliceFlag{
		Name:     "aws-security-groups",
		Usage:    "security groups attached to EC2 instances",
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
