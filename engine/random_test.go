package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRandomString(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want int
	}{
		{
			name: "zero length",
			n:    0,
			want: 0,
		},
		{
			name: "length 10",
			n:    10,
			want: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			str := RandomString(tt.n)
			assert.Equal(t, tt.want, len(str))
		})

		t.Run("alphanumeric", func(t *testing.T) {
			str1 := RandomString(10)
			for _, r := range str1 {
				assert.Contains(t, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", string(r))
			}
		})
	}
}
