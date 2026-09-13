package ociapi

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
)

type ComputeClient interface {
	LaunchInstance(ctx context.Context, request core.LaunchInstanceRequest) (core.LaunchInstanceResponse, error)
	ListInstances(ctx context.Context, request core.ListInstancesRequest) (core.ListInstancesResponse, error)
	TerminateInstance(ctx context.Context, request core.TerminateInstanceRequest) (core.TerminateInstanceResponse, error)
	ListImages(ctx context.Context, request core.ListImagesRequest) (core.ListImagesResponse, error)
	ListShapes(ctx context.Context, request core.ListShapesRequest) (core.ListShapesResponse, error)
}

type IdentityClient interface {
	ListAvailabilityDomains(ctx context.Context, request identity.ListAvailabilityDomainsRequest) (identity.ListAvailabilityDomainsResponse, error)
}
