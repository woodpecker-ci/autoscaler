package azure

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeComputerName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid name",
			input:    "wp-agent-1",
			expected: "wp-agent-1",
		},
		{
			name:     "name with underscores",
			input:    "wp_agent_1",
			expected: "wpagent1",
		},
		{
			name:     "name with special chars",
			input:    "wp.agent@1!",
			expected: "wpagent1",
		},
		{
			name:     "long name truncated to 64",
			input:    "abcdefghijklmnopqrstuvwxyz0123456789-abcdefghijklmnopqrstuvwxyz0123456789",
			expected: "abcdefghijklmnopqrstuvwxyz0123456789-abcdefghijklmnopqrstuvwxyz0",
		},
		{
			name:     "empty name",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeComputerName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "resource not found",
			err:      fmt.Errorf("ResourceNotFound: the resource was not found"),
			expected: true,
		},
		{
			name:     "not found",
			err:      fmt.Errorf("NotFound: VM does not exist"),
			expected: true,
		},
		{
			name:     "other error",
			err:      fmt.Errorf("InternalServerError: something went wrong"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNotFoundError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
