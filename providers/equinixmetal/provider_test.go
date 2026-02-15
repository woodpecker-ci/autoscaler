package equinixmetal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProviderName(t *testing.T) {
	p := &Provider{name: "equinixmetal"}
	assert.Equal(t, "equinixmetal", p.name)
}

func TestCategory(t *testing.T) {
	assert.Equal(t, "Equinix Metal", Category)
}

func TestProviderFlagsCount(t *testing.T) {
	// Should have 6 flags
	assert.Len(t, ProviderFlags, 6)
}
