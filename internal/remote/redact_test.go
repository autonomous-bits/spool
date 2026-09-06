package remote

import (
	"strings"
	"testing"
)

func TestRedactScrubsKnownSecrets(t *testing.T) {
	text := "request to https://rack.example.com failed for token sk-abcdefghijklmnop and tenant acme-042"
	redacted := Redact(text, "sk-abcdefghijklmnop", "acme-042")
	if strings.Contains(redacted, "sk-abcdefghijklmnop") || strings.Contains(redacted, "acme-042") {
		t.Fatalf("Redact left secret in output: %q", redacted)
	}
	if !strings.Contains(redacted, RedactedPlaceholder) {
		t.Fatalf("Redact did not insert placeholder: %q", redacted)
	}
}

func TestRedactIgnoresEmptySecrets(t *testing.T) {
	text := "no secrets here"
	if got := Redact(text, "", "   "); got != text {
		t.Fatalf("Redact(%q) = %q, want unchanged", text, got)
	}
}

func TestLooksLikeSecretDetectsKnownPrefixes(t *testing.T) {
	cases := map[string]bool{
		"ghp_abcdefghijklmnopqrstuvwxyz0123456789": true,
		"github_pat_11ABCDEF0abcdefghijklmnop":     true,
		"Bearer abc123":                            true,
		"acme-prod":                                false,
		"https://rack.example.com":                 false,
	}
	for value, want := range cases {
		if got := looksLikeSecret(value); got != want {
			t.Fatalf("looksLikeSecret(%q) = %v, want %v", value, got, want)
		}
	}
}
