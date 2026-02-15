package oraclecloud

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProviderName(t *testing.T) {
	p := &Provider{name: "oraclecloud"}
	assert.Equal(t, "oraclecloud", p.name)
}

func TestCategory(t *testing.T) {
	assert.Equal(t, "Oracle Cloud", Category)
}

func TestProviderFlagsCount(t *testing.T) {
	// Should have 10 flags
	assert.Len(t, ProviderFlags, 10)
}
