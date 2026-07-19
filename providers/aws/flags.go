package aws

import "github.com/urfave/cli/v3"

const Category = "AWS"

var ProviderFlags = []cli.Flag{
	// aws
	&cli.StringSliceFlag{
		Name:     "aws-instance-type",
		Usage:    "EC2 instance types, optionally with region as 'type:region'; tried in order as deploy fallbacks",
		Value:    []string{"t3.medium", "t3.micro"},
		Sources:  cli.EnvVars("WOODPECKER_AWS_INSTANCE_TYPE"),
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "aws-ami-id",
		Usage:    "AMI ID or alias (ubuntu-<version>-server, amazon, suse[-<version>], debian-<version>); architecture and region come from the instance type",
		Value:    "ubuntu-26.04-server",
		Sources:  cli.EnvVars("WOODPECKER_AWS_AMI_ID"),
		Category: Category,
	},
	&cli.StringSliceFlag{
		Name:     "aws-tags",
		Usage:    "additional tags for your EC2 instances",
		Sources:  cli.EnvVars("WOODPECKER_AWS_TAGS"),
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "aws-access-key-id",
		Usage:    "AWS access key ID",
		Sources:  cli.EnvVars("WOODPECKER_AWS_ACCESS_KEY_ID", "AWS_ACCESS_KEY_ID"),
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "aws-secret-access-key",
		Usage:    "AWS secret access key",
		Sources:  cli.EnvVars("WOODPECKER_AWS_SECRET_ACCESS_KEY", "AWS_SECRET_ACCESS_KEY"),
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "aws-region",
		Usage:    "default AWS region for unqualified instance types and resources",
		Value:    "us-east-1",
		Sources:  cli.EnvVars("WOODPECKER_AWS_REGION"),
		Category: Category,
	},
	&cli.StringSliceFlag{
		Name:     "aws-subnets",
		Usage:    "VPC subnet IDs, optionally with region as 'subnet:region'; default subnets are used when omitted",
		Sources:  cli.EnvVars("WOODPECKER_AWS_SUBNETS"),
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "aws-iam-instance-profile-arn",
		Usage:    "IAM instance profile ARN",
		Sources:  cli.EnvVars("WOODPECKER_AWS_IAM_INSTANCE_PROFILE_ARN"),
		Category: Category,
	},
	&cli.StringSliceFlag{
		Name:     "aws-security-groups",
		Usage:    "security group IDs, optionally with region as 'group:region'",
		Sources:  cli.EnvVars("WOODPECKER_AWS_SECURITY_GROUPS"),
		Category: Category,
	},
	&cli.BoolFlag{
		Name:     "aws-use-spot-instances",
		Usage:    "use spot instances",
		Sources:  cli.EnvVars("WOODPECKER_AWS_USE_SPOT_INSTANCES"),
		Category: Category,
	},
	&cli.StringFlag{
		Name:     "aws-ssh-key-name",
		Usage:    "SSH keypair name",
		Sources:  cli.EnvVars("WOODPECKER_AWS_SSH_KEYNAME"),
		Category: Category,
	},
}
