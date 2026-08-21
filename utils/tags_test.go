package utils_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/autoscaler/utils"
)

func TestCheckReservedTags(t *testing.T) {
	const reservedPrefix = "reserved/"

	errReserved := errors.New("reserved tag prefix")

	tests := []struct {
		name    string
		tags    []string
		wantErr error
	}{
		{
			name: "no tags at all",
		},
		{
			name: "operator tags are kept",
			tags: []string{"env=prod", "team-ci"},
		},
		{
			name:    "a reserved key is rejected",
			tags:    []string{reservedPrefix + "pool=other-pool"},
			wantErr: errReserved,
		},
		{
			name:    "a reserved key without a value is rejected",
			tags:    []string{reservedPrefix + "pool"},
			wantErr: errReserved,
		},
		{
			name:    "padding does not sneak a reserved tag through",
			tags:    []string{" " + reservedPrefix + "pool =other-pool"},
			wantErr: errReserved,
		},
		{
			name: "the prefix only counts in the key",
			tags: []string{"origin=" + reservedPrefix},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := utils.CheckReservedTags(tt.tags, reservedPrefix, errReserved)
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
			require.Contains(t, err.Error(), reservedPrefix)
		})
	}
}
