package openstack

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetadataParsing(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected map[string]string
	}{
		{
			name:  "valid key=value pairs",
			input: []string{"env=production", "team=ci"},
			expected: map[string]string{
				"env":  "production",
				"team": "ci",
			},
		},
		{
			name:     "empty input",
			input:    []string{},
			expected: map[string]string{},
		},
		{
			name:  "value with equals sign",
			input: []string{"key=val=ue"},
			expected: map[string]string{
				"key": "val=ue",
			},
		},
		{
			name:     "invalid entry without equals",
			input:    []string{"noequalssign"},
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseMetadata(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func parseMetadata(entries []string) map[string]string {
	metadata := make(map[string]string)
	for _, m := range entries {
		key, value, ok := func(s string) (string, string, bool) {
			idx := 0
			for i, r := range s {
				if r == '=' {
					idx = i
					break
				}
			}
			if idx == 0 {
				return "", "", false
			}
			return s[:idx], s[idx+1:], true
		}(m)
		if ok {
			metadata[key] = value
		}
	}
	return metadata
}
