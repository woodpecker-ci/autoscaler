package azure

import (
	"errors"
	"testing"
)

func TestParseImageURN(t *testing.T) {
	tests := []struct {
		urn     string
		wantErr error
		want    imageReference
	}{
		{
			urn:  "Canonical:ubuntu-24_04-lts:server:latest",
			want: imageReference{publisher: "Canonical", offer: "ubuntu-24_04-lts", sku: "server", version: "latest"},
		},
		{
			urn:  "MicrosoftWindowsServer:WindowsServer:2022-Datacenter:latest",
			want: imageReference{publisher: "MicrosoftWindowsServer", offer: "WindowsServer", sku: "2022-Datacenter", version: "latest"},
		},
		{
			urn:     "Canonical:ubuntu-24_04-lts:server",
			wantErr: ErrImageInvalid,
		},
		{
			urn:     "Canonical:ubuntu-24_04-lts::latest",
			wantErr: ErrImageInvalid,
		},
		{
			urn:     "",
			wantErr: ErrImageInvalid,
		},
	}

	for _, tt := range tests {
		got, err := parseImageURN(tt.urn)
		if tt.wantErr != nil {
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("parseImageURN(%q): got err %v, want %v", tt.urn, err, tt.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseImageURN(%q): unexpected error: %v", tt.urn, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseImageURN(%q): got %+v, want %+v", tt.urn, got, tt.want)
		}
	}
}

func TestSanitizeComputerName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"wp-agent-1", "wp-agent-1"},
		{"wp_agent_1", "wpagent1"},
		{"pool.123@agent!", "pool123agent"},
		{"", ""},
		{
			// 66-char input → truncated to 64
			"abcdefghijklmnopqrstuvwxyz0123456789-abcdefghijklmnopqrstuvwxyz012",
			"abcdefghijklmnopqrstuvwxyz0123456789-abcdefghijklmnopqrstuvwxyz0",
		},
	}

	for _, tt := range tests {
		got := sanitizeComputerName(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeComputerName(%q): got %q, want %q", tt.input, got, tt.want)
		}
	}
}
