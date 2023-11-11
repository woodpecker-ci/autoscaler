package v1

import (
	"errors"
	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// Config is the Scaleway Provider specific configuration
// NB(raskyld): In the future, I think each provider should be able to
// unmarshal from a config file JSON stream passed by the engine.
// The engine should also provide utilities, for example, a pre-defined
// type that allows the providers to either read hard-coded values
// or retrieve them from the filesystem, e.g. for secrets.
type Config struct {
	// ApiToken of Scaleway IAM
	//
	// Creating a standalone IAM Applications is recommended to segregate
	// permissions.
	SecretKey        string                  `json:"secret_key"`
	AccessKey        string                  `json:"access_key"`
	DefaultProjectID string                  `json:"default_project_id"`
	InstancePool     map[string]InstancePool `json:"instance_pool"`
}

// Locality defines a geographical area
//
// Scaleway Cloud has multiple Region that are made of several Zones.
// Exactly one of Zones or Region SHOULD be set,
// if both are set, use Region and ignore Zones
type Locality struct {
	Zones  []scw.Zone  `json:"zones,omitempty"`
	Region *scw.Region `json:"region,omitempty"`
}

// InstancePool is a small helper to handle a pool of instances
type InstancePool struct {
	// Locality where your instances should live
	// The InstancePool scheduler will try to spread your
	// instances evenly among Locality.Zones if possible
	Locality Locality `json:"locality"`
	// ProjectID where resources should be applied
	ProjectID *string `json:"project_id,omitempty"`
	// Prefix is added before each instance name
	Prefix string `json:"prefix"`
	// Tags added to the placement group and its instances
	Tags []string `json:"tags"`
	// DynamicIPRequired: define if a dynamic IPv4 is required for the Instance.
	DynamicIPRequired *bool `json:"dynamic_ip_required,omitempty"`
	// RoutedIPEnabled: if true, configure the Instance, so it uses the new routed IP mode.
	RoutedIPEnabled *bool `json:"routed_ip_enabled,omitempty"`
	// CommercialType: define the Instance commercial type (i.e. GP1-S).
	CommercialType string `json:"commercial_type,omitempty"`
	// Image: instance image ID or label.
	Image string `json:"image,omitempty"`
	// EnableIPv6: true if IPv6 is enabled on the server.
	EnableIPv6 bool `json:"enable_ipv6,omitempty"`
	// PublicIPs to attach to your instance indexed per instance.IPType
	PublicIPs map[instance.IPType]int `json:"public_ips,omitempty"`
	// SecurityGroups to use per zone
	SecurityGroups map[scw.Zone]string `json:"security_groups,omitempty"`
	// Storage of the block storage associated with your Instances
	// It should be a multiple of 512 bytes, in future version we could give
	// more customisation over the volumes used by the agents
	Storage scw.Size `json:"storage"`
}

func (l Locality) ResolveZones() ([]scw.Zone, error) {
	if l.Region != nil {
		if !l.Region.Exists() {
			return nil, errors.New("you specified an invalid region: " + l.Region.String())
		}

		return l.Region.GetZones(), nil
	}

	zones := l.Zones
	if zones == nil || len(zones) <= 0 {
		return nil, errors.New("you need to specify a valid locality")
	}

	for _, zone := range zones {
		if !zone.Exists() {
			return nil, errors.New("you specified a non-existing zone: " + zone.String())
		}
	}

	return zones, nil
}
