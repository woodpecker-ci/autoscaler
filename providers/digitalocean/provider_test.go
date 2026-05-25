package digitalocean

import (
	"strings"
	"testing"
)

func TestParseSSHKeys(t *testing.T) {
	keys := parseSSHKeys([]string{"12345", " SHA256:fingerprint ", ""})

	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	if keys[0].ID != 12345 {
		t.Fatalf("expected numeric key to be parsed as ID, got %d", keys[0].ID)
	}
	if keys[1].Fingerprint != "SHA256:fingerprint" {
		t.Fatalf("expected non-numeric key to be parsed as fingerprint, got %q", keys[1].Fingerprint)
	}
}

func TestPoolTag(t *testing.T) {
	tag := poolTag("Woodpecker Pool/1.Main")

	if tag != "woodpecker-pool-woodpecker-pool-1-main" {
		t.Fatalf("unexpected pool tag %q", tag)
	}
}

func TestSanitizeTag(t *testing.T) {
	tag := sanitizeTag("  Team A / Pool.Main  ")

	if tag != "team-a-pool-main" {
		t.Fatalf("unexpected sanitized tag %q", tag)
	}
}

func TestTrimTag(t *testing.T) {
	tag := trimTag(strings.Repeat("a", maxTagLength+1))

	if len(tag) != maxTagLength {
		t.Fatalf("expected tag length %d, got %d", maxTagLength, len(tag))
	}
}
