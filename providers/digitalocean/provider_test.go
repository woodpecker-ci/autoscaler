package digitalocean

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProviderName(t *testing.T) {
	p := &Provider{name: "digitalocean"}
	assert.Equal(t, "digitalocean", p.name)
}

func TestCategory(t *testing.T) {
	assert.Equal(t, "DigitalOcean", Category)
}

func TestProviderFlagsCount(t *testing.T) {
	// Should have 6 flags
	assert.Len(t, ProviderFlags, 6)
}
