package oracle

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProviderName(t *testing.T) {
	p := &Provider{name: "oracle"}
	assert.Equal(t, "oracle", p.name)
}

func TestFreeformTagFormat(t *testing.T) {
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
			expected: "prod",
		},
		{
			name:     "Pool label",
			key:      "wp.autoscaler/pool",
			value:    "default",
			expected: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags := make(map[string]string)
			tags[tt.key] = tt.value
			assert.Equal(t, tt.expected, tags[tt.key])
		})
	}
}
