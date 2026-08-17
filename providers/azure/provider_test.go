package azure

import "testing"

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
