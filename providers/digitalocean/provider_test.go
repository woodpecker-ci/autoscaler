package digitalocean

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeTag(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple tag",
			input:    "test",
			expected: "test",
		},
		{
			name:     "Tag with slash",
			input:    "wp.autoscaler/pool",
			expected: "wp.autoscaler-pool",
		},
		{
			name:     "Tag with equals",
			input:    "key=value",
			expected: "key:value",
		},
		{
			name:     "Complex tag",
			input:    "wp.autoscaler/pool:mypool",
			expected: "wp.autoscaler-pool:mypool",
		},
		{
			name:     "Uppercase to lowercase",
			input:    "MyTag",
			expected: "mytag",
		},
		{
			name:     "Full label format",
			input:    "wp.autoscaler/pool=production",
			expected: "wp.autoscaler-pool:production",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeTag(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProviderName(t *testing.T) {
	p := &Provider{name: "digitalocean"}
	assert.Equal(t, "digitalocean", p.name)
}
