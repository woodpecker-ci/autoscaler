package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSliceToMap(t *testing.T) {
	testCases := []struct {
		name    string
		input   []string
		del     string
		want    map[string]string
		wantErr error
	}{
		{
			name:    "basic",
			input:   []string{"key1=value1", "key2=value2"},
			del:     "=",
			want:    map[string]string{"key1": "value1", "key2": "value2"},
			wantErr: nil,
		},
		{
			name:    "whitespace",
			input:   []string{"key1 = value1", "key2=value2"},
			del:     "=",
			want:    map[string]string{"key1": "value1", "key2": "value2"},
			wantErr: nil,
		},
		{
			name:    "missing delimiter",
			input:   []string{"key1", "key2=value2"},
			del:     "=",
			want:    nil,
			wantErr: assert.AnError,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := SliceToMap(tt.input, tt.del)
			if tt.wantErr != nil {
				assert.Error(t, err)

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, actual)
		})
	}
}

func TestMergeMaps(t *testing.T) {
	testCases := []struct {
		name string
		m1   map[string]string
		m2   map[string]string
		want map[string]string
	}{
		{
			name: "nil maps",
			m1:   nil,
			m2:   nil,
			want: map[string]string{},
		},
		{
			name: "empty maps",
			m1:   map[string]string{},
			m2:   map[string]string{},
			want: map[string]string{},
		},
		{
			name: "overwrite",
			m1:   map[string]string{"key1": "value1", "key2": "value2"},
			m2:   map[string]string{"key2": "newvalue2", "key3": "value3"},
			want: map[string]string{"key1": "value1", "key2": "newvalue2", "key3": "value3"},
		},
		{
			name: "no overwrite",
			m1:   map[string]string{"key1": "value1", "key2": "value2"},
			m2:   map[string]string{"key3": "value3", "key4": "value4"},
			want: map[string]string{"key1": "value1", "key2": "value2", "key3": "value3", "key4": "value4"},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			merged := MergeMaps(tt.m1, tt.m2)
			assert.Equal(t, tt.want, merged)
		})
	}
}
