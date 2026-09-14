package oracle

import (
	"context"
	"errors"

	"github.com/oracle/oci-go-sdk/v65/core"
)

var (
	ErrIncompleteCredentials      = errors.New("tenancy ID, user ID, fingerprint and private key must all be set for API key authentication")
	ErrRegionRequired             = errors.New("region is required for API key authentication")
	ErrCompartmentIDRequired      = errors.New("compartment ID is required")
	ErrAvailabilityDomainRequired = errors.New("availability domain is required")
	ErrSubnetIDRequired           = errors.New("subnet ID is required")
	ErrShapeRequired              = errors.New("shape is required")
	ErrImageRequired              = errors.New("either image ID or operating system and version are required")
	ErrImageNotFound              = errors.New("no image found")
	ErrInvalidTag                 = errors.New("invalid freeform tag")
	ErrReservedTagPrefix          = errors.New("illegal freeform tag prefix")
	ErrMultipleInstances          = errors.New("multiple instances found")
)

const (
	defaultOcpus     = 1
	defaultMemoryGBs = 8
	listPageSize     = 100
)

// computeAPI is the subset of core.ComputeClient used by the provider.
type computeAPI interface {
	LaunchInstance(context.Context, core.LaunchInstanceRequest) (core.LaunchInstanceResponse, error)
	TerminateInstance(context.Context, core.TerminateInstanceRequest) (core.TerminateInstanceResponse, error)
	ListInstances(context.Context, core.ListInstancesRequest) (core.ListInstancesResponse, error)
	ListImages(context.Context, core.ListImagesRequest) (core.ListImagesResponse, error)
}
