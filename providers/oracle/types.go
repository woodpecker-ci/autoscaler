package oracle

import "errors"

var (
	ErrCompartmentNotSet        = errors.New("no compartment ocid provided")
	ErrSubnetNotSet             = errors.New("no subnet ocid provided")
	ErrIncompleteAPIKeyAuth     = errors.New("incomplete api key credentials")
	ErrNoAvailabilityDomains    = errors.New("no availability domain available in compartment")
	ErrImageNotFound            = errors.New("image not found")
	ErrNoShapes                 = errors.New("no shape configured")
	ErrShapeNotFound            = errors.New("shape not found in compartment")
	ErrNoMatchingShape          = errors.New("no configured shape provides the requested platform")
	ErrPublicIPProhibited       = errors.New("subnet prohibits public IPs")
	ErrSubnetAvailabilityDomain = errors.New("subnet is bound to an availability domain that is not configured")
	ErrBootVolumeTooSmall       = errors.New("boot volume size below the oracle cloud minimum")
	ErrInstanceNotFound         = errors.New("instance not found")
	ErrReservedTagPrefix        = errors.New("reserved tag prefix")
	ErrInvalidTag               = errors.New("invalid tag")
	ErrNoCapacity               = errors.New("no shape had capacity in any availability domain")
)

var capacityErrorCodes = []string{
	"OutOfCapacity",
	"OutOfHostCapacity",
	"LimitExceeded",
	"TooManyRequests",
}

const (
	maxFreeformTags = 10
	metadataUserKey = "user_data"
	metadataSSHKey  = "ssh_authorized_keys"
	listPageLimit   = 100
	minBootVolumeGB = 50
)
