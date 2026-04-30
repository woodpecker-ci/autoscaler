package hetznercloud

import (
	"errors"
	"testing"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
)

func TestResolveLocation(t *testing.T) {
	nbg1 := &hcloud.Location{Name: "nbg1"}

	tests := []struct {
		name        string
		st          *hcloud.ServerType
		location    string
		wantLoc     *hcloud.Location
		wantErr     error
		wantErrText string
	}{
		{
			name:     "EmptyLocationReturnsNil",
			st:       &hcloud.ServerType{},
			location: "",
			wantLoc:  nil,
		},
		{
			name: "Match",
			st: &hcloud.ServerType{
				Locations: []hcloud.ServerTypeLocation{
					{Location: &hcloud.Location{Name: "fsn1"}},
					{Location: nbg1},
				},
			},
			location: "nbg1",
			wantLoc:  nbg1,
		},
		{
			name: "DeprecatedLocation",
			st: &hcloud.ServerType{
				Locations: []hcloud.ServerTypeLocation{
					{
						Location: nbg1,
						DeprecatableResource: hcloud.DeprecatableResource{
							Deprecation: &hcloud.DeprecationInfo{Announced: time.Now()},
						},
					},
				},
			},
			location: "nbg1",
			wantErr:  ErrLocationNotSupported,
		},
		{
			name: "NotInList",
			st: &hcloud.ServerType{
				Locations: []hcloud.ServerTypeLocation{
					{Location: &hcloud.Location{Name: "fsn1"}},
				},
			},
			location: "nbg1",
			wantErr:  ErrLocationNotSupported,
		},
		{
			name: "NilLocationEntrySkipped",
			st: &hcloud.ServerType{
				Locations: []hcloud.ServerTypeLocation{
					{Location: nil},
					{Location: nbg1},
				},
			},
			location: "nbg1",
			wantLoc:  nbg1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveLocation(tt.st, tt.location)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantLoc, got)
		})
	}

	// sanity: errors.Is wiring works
	_, err := resolveLocation(&hcloud.ServerType{}, "nowhere")
	assert.True(t, errors.Is(err, ErrLocationNotSupported))
}
