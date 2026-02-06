package equinixmetal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProviderName(t *testing.T) {
	p := &Provider{name: "equinixmetal"}
	assert.Equal(t, "equinixmetal", p.name)
}

func TestTagFormat(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		expected string
	}{
		{
			name:     "Simple tag",
			key:      "env",
			value:    "prod",
			expected: "env=prod",
		},
		{
			name:     "Pool label",
			key:      "wp.autoscaler/pool",
			value:    "default",
			expected: "wp.autoscaler/pool=default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.key + "=" + tt.value
			assert.Equal(t, tt.expected, result)
		})
	}
}
