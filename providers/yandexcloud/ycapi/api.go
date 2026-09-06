package ycapi

import "context"

type Image struct {
	ID          string
	MinDiskSize int64
	Status      string
}

type Subnet struct {
	ID       string
	FolderID string
	ZoneID   string
}

type Instance struct {
	ID     string
	Name   string
	Labels map[string]string
	Status string
}

type CreateRequest struct {
	FolderID         string
	Name             string
	ZoneID           string
	PlatformID       string
	Cores            int64
	Memory           int64
	CoreFraction     int64
	Labels           map[string]string
	UserData         string
	ImageID          string
	DiskType         string
	DiskSize         int64
	SubnetID         string
	SecurityGroupIDs []string
	PublicIPv4       bool
}

// Client is the subset of Yandex Cloud used by the provider.
// The SDK-specific long-running operation types stay inside the adapter.
type Client interface {
	GetImage(context.Context, string, string, string) (Image, error)
	GetSubnet(context.Context, string) (Subnet, error)
	Create(context.Context, CreateRequest) (Instance, error)
	Delete(context.Context, string) error
	List(context.Context, string) ([]Instance, error)
}
