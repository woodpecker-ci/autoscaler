package v1

import (
	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	"io"
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
	ApiToken     io.Reader
	InstancePool map[string]InstancePool
}

// Locality defines a geographical area
//
// Scaleway Cloud has multiple Region that are made of several Zones.
// Exactly one of Zones or Region SHOULD be set,
// if both are set, use Zones and ignore Region
type Locality struct {
	Zones  []scw.Zone
	Region *scw.Region
}

// InstancePool is a small helper to handle a pool of instances
type InstancePool struct {
	// Locality where your instances should live
	// The InstancePool scheduler will try to spread your
	// instances evenly among Locality.Zones if possible
	Locality Locality
	// ProjectID where resources should be applied
	ProjectID *string
	// Prefix is added before each instance name
	Prefix string
	// Tags added to the placement group and its instances
	Tags []string
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
	PublicIPs map[instance.IPType]int
	// SecurityGroups to use per zone
	SecurityGroups map[scw.Zone]string
	// Storage of the block storage associated with your Instances
	// It should be a multiple of 512 bytes, in future version we could give
	// more customisation over the volumes used by the agents
	Storage scw.Size
}
