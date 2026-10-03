package vultr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vultr/govultr/v3"
)

func TestImageToGoArch(t *testing.T) {
	assert.Equal(t, "amd64", imageToGoArch(govultr.OS{Arch: "x64"}))
	assert.Equal(t, "arm64", imageToGoArch(govultr.OS{Arch: "arm64"}))
}
