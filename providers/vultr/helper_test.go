package vultr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vultr/govultr/v3"

	"go.woodpecker-ci.org/autoscaler/engine/types"
)

func TestMatchImagesOnlyAcceptsX64(t *testing.T) {
	images := []govultr.OS{
		{ID: 1, Name: "Debian 13 arm64 (trixie)", Arch: "arm64"},
		{ID: 2, Name: "Debian 13 x64 (trixie)", Arch: "x64"},
	}

	assert.Equal(t, images[1:], matchImages(images, "debian 13"))
	assert.Empty(t, matchImages(images, "debian 13 arm64"))
}

func TestCapabilitiesAdvertiseAMD64Only(t *testing.T) {
	caps, err := (&provider{}).Capabilities(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []types.Capability{{Platform: "linux/amd64", Backend: types.BackendDocker}}, caps)
}
