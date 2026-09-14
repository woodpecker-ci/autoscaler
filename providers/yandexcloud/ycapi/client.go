package ycapi

import (
	"context"
	"errors"
	"fmt"
	"time"

	compute "github.com/yandex-cloud/go-genproto/yandex/cloud/compute/v1"
	vpc "github.com/yandex-cloud/go-genproto/yandex/cloud/vpc/v1"
	computesdk "github.com/yandex-cloud/go-sdk/services/compute/v1"
	vpcsdk "github.com/yandex-cloud/go-sdk/services/vpc/v1"
	ycsdk "github.com/yandex-cloud/go-sdk/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const cleanupTimeout = time.Minute

const listPageSize int64 = 1000

type client struct {
	instances computesdk.InstanceClient
	images    computesdk.ImageClient
	subnets   vpcsdk.SubnetClient
}

func NewClient(sdk *ycsdk.SDK) Client {
	return &client{
		instances: computesdk.NewInstanceClient(sdk),
		images:    computesdk.NewImageClient(sdk),
		subnets:   vpcsdk.NewSubnetClient(sdk),
	}
}

func (c *client) GetImage(ctx context.Context, folderID, imageID, family string) (Image, error) {
	var (
		image *compute.Image
		err   error
	)
	if imageID != "" {
		image, err = c.images.Get(ctx, &compute.GetImageRequest{ImageId: imageID})
	} else {
		image, err = c.images.GetLatestByFamily(ctx, &compute.GetImageLatestByFamilyRequest{
			FolderId: folderID,
			Family:   family,
		})
	}
	if err != nil {
		return Image{}, err
	}
	if image == nil {
		return Image{}, errors.New("image API returned an empty response")
	}

	return Image{
		ID:          image.Id,
		MinDiskSize: image.MinDiskSize,
		Status:      image.Status.String(),
	}, nil
}

func (c *client) GetSubnet(ctx context.Context, subnetID string) (Subnet, error) {
	subnet, err := c.subnets.Get(ctx, &vpc.GetSubnetRequest{SubnetId: subnetID})
	if err != nil {
		return Subnet{}, err
	}
	if subnet == nil {
		return Subnet{}, errors.New("subnet API returned an empty response")
	}

	return Subnet{
		ID:       subnet.Id,
		FolderID: subnet.FolderId,
		ZoneID:   subnet.ZoneId,
	}, nil
}

func (c *client) Create(ctx context.Context, req CreateRequest) (Instance, error) {
	addressSpec := &compute.PrimaryAddressSpec{}
	if req.PublicIPv4 {
		addressSpec.OneToOneNatSpec = &compute.OneToOneNatSpec{
			IpVersion: compute.IpVersion_IPV4,
		}
	}

	operation, err := c.instances.Create(ctx, &compute.CreateInstanceRequest{
		FolderId:   req.FolderID,
		Name:       req.Name,
		ZoneId:     req.ZoneID,
		PlatformId: req.PlatformID,
		Labels:     req.Labels,
		Metadata:   map[string]string{"user-data": req.UserData},
		ResourcesSpec: &compute.ResourcesSpec{
			Cores:        req.Cores,
			Memory:       req.Memory,
			CoreFraction: req.CoreFraction,
		},
		BootDiskSpec: &compute.AttachedDiskSpec{
			AutoDelete: true,
			Disk: &compute.AttachedDiskSpec_DiskSpec_{
				DiskSpec: &compute.AttachedDiskSpec_DiskSpec{
					TypeId: req.DiskType,
					Size:   req.DiskSize,
					Source: &compute.AttachedDiskSpec_DiskSpec_ImageId{ImageId: req.ImageID},
				},
			},
		},
		NetworkInterfaceSpecs: []*compute.NetworkInterfaceSpec{{
			SubnetId:             req.SubnetID,
			PrimaryV4AddressSpec: addressSpec,
			SecurityGroupIds:     req.SecurityGroupIDs,
		}},
	})
	if err != nil {
		return Instance{}, err
	}

	created, err := operation.Wait(ctx)
	if err == nil {
		if created == nil {
			return Instance{}, errors.New("create operation returned an empty response")
		}
		return instanceFromProto(created), nil
	}

	instanceID := operation.Metadata().GetInstanceId()
	if instanceID == "" {
		return Instance{}, fmt.Errorf("wait for create operation: %w", err)
	}

	cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	cleanupErr := c.Delete(cleanupContext, instanceID)
	if cleanupErr != nil {
		return Instance{}, errors.Join(
			fmt.Errorf("wait for create operation: %w", err),
			fmt.Errorf("cleanup instance %q: %w", instanceID, cleanupErr),
		)
	}
	return Instance{}, fmt.Errorf("wait for create operation: %w", err)
}

func (c *client) Delete(ctx context.Context, instanceID string) error {
	operation, err := c.instances.Delete(ctx, &compute.DeleteInstanceRequest{InstanceId: instanceID})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil
		}
		return err
	}
	_, err = operation.Wait(ctx)
	if status.Code(err) == codes.NotFound {
		return nil
	}
	return err
}

func (c *client) List(ctx context.Context, folderID string) ([]Instance, error) {
	instances := make([]Instance, 0)
	pageToken := ""
	for {
		response, err := c.instances.List(ctx, &compute.ListInstancesRequest{
			FolderId:  folderID,
			PageSize:  listPageSize,
			PageToken: pageToken,
		})
		if err != nil {
			return nil, err
		}
		if response == nil {
			return nil, errors.New("list API returned an empty response")
		}
		for _, instance := range response.Instances {
			if instance != nil {
				instances = append(instances, instanceFromProto(instance))
			}
		}
		if response.NextPageToken == "" {
			return instances, nil
		}
		pageToken = response.NextPageToken
	}
}

func instanceFromProto(instance *compute.Instance) Instance {
	labels := make(map[string]string, len(instance.Labels))
	for key, value := range instance.Labels {
		labels[key] = value
	}
	return Instance{
		ID:     instance.Id,
		Name:   instance.Name,
		Labels: labels,
		Status: instance.Status.String(),
	}
}
