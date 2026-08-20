package linode

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/autoscaler/engine"
)

func TestCheckReservedTags(t *testing.T) {
	tests := []struct {
		name    string
		tags    []string
		wantErr error
	}{
		{
			name: "operator tags are kept",
			tags: []string{"env=prod", "team-ci"},
		},
		{
			name:    "a foreign pool tag is rejected",
			tags:    []string{engine.LabelPool + "=other-pool"},
			wantErr: ErrReservedTagPrefix,
		},
		{
			name:    "padding does not sneak a reserved tag through",
			tags:    []string{" " + engine.LabelPool + " =other-pool"},
			wantErr: ErrReservedTagPrefix,
		},
		{
			name: "the prefix only counts in the key",
			tags: []string{"origin=" + engine.LabelPrefix},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkReservedTags(tt.tags)
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}
