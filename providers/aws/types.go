package aws

import (
	"errors"

	ec2_types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

var (
	ErrInstanceTypeNotFound     = errors.New("instance type not found")
	ErrAMINotFound              = errors.New("AMI not found")
	ErrSubnetsNotSet            = errors.New("aws-subnets must be set")
	ErrSubnetNotFound           = errors.New("subnet not found")
	ErrSecurityGroupNotFound    = errors.New("security group not found")
	ErrArchMismatch             = errors.New("instance type architecture not supported by AMI")
	ErrInstanceTypeArchitecture = errors.New("instance type must report exactly one architecture")
	ErrTypeNotInRegion          = errors.New("instance type not offered in region")
	ErrNoDeployCandidates       = errors.New("no deploy candidates resolved")
	ErrNoMatchingCandidate      = errors.New("no deploy candidate matches requested capability")
	ErrUnknownArchitecture      = errors.New("unknown architecture")
	ErrRegionNotSet             = errors.New("aws-region must be set for unqualified values")
)

// regionConfig contains the resources that exist together in an AWS region.
type regionConfig struct {
	region         string
	image          ec2_types.Image
	subnets        []string
	securityGroups []string
}

// deployCandidate is one deploy specification the provider tries in order.
// It owns its instance type and all region-scoped resources it needs.
type deployCandidate struct {
	instanceType ec2_types.InstanceTypeInfo
	regionConfig regionConfig
}
