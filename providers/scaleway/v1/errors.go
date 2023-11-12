package v1

import (
	"log/slog"

	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

type InstanceAlreadyExistsError struct {
	inst *instance.Server
}

type InstanceDoesNotExists struct {
	InstanceName string
	Project      string
	Zones        []scw.Zone
}

func (i InstanceAlreadyExistsError) Error() string {
	return "instance already exists"
}

func (i InstanceAlreadyExistsError) LogValue() slog.Value {
	return slog.GroupValue(slog.String("err", i.Error()),
		slog.Group("instance", slog.String("id", i.inst.ID), slog.String("name", i.inst.Name),
			slog.String("zone", i.inst.Zone.String()), slog.String("project", i.inst.Project)))
}

func (i InstanceDoesNotExists) Error() string {
	return "instance does not exist"
}

func (i InstanceDoesNotExists) LogValue() slog.Value {
	zones := make([]string, len(i.Zones))
	for _, zone := range i.Zones {
		zones = append(zones, zone.String())
	}

	return slog.GroupValue(slog.String("name", i.InstanceName), slog.String("project", i.Project), slog.Any("zones", zones))
}
