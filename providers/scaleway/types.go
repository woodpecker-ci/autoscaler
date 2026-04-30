package scaleway

import (
	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// deployCandidate is a fully-resolved server type + image pair ready for use
// in CreateServer. Candidates are built once in New() and filtered at deploy
// time by the requested Capability arch.
type deployCandidate struct {
	rawType    string
	zone       scw.Zone
	serverType *instance.ServerType
	// imageID is the zone- and arch-specific ID resolved from the configured
	// image name list. The first name that resolves for the server type's
	// architecture wins.
	imageID   string
	imageName string
}
