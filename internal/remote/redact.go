package remote

import "strings"

// RedactedPlaceholder replaces any redacted secret value.
const RedactedPlaceholder = "***redacted***"

// secretPrefixes lists common credential/token prefixes that must never be
// stored as non-secret remote configuration values and must always be
// redacted from diagnostics.
var secretPrefixes = []string{
	"ghp_", "gho_", "ghu_", "ghs_", "ghr_", "github_pat_",
	"sk-", "xox", "akia", "eyj", "bearer ",
}

// looksLikeSecret applies conservative heuristics to flag values that are
// very likely pasted-in credentials rather than portable, non-secret
// configuration values.
func looksLikeSecret(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	for _, prefix := range secretPrefixes {
		if strings.Contains(lower, prefix) {
			return true
		}
	}
	return false
}

// Redact scrubs every non-empty value in secrets from text, replacing each
// occurrence with RedactedPlaceholder. It is used to keep bearer tokens, API
// keys, and sensitive tenant identifiers out of errors, JSON stdout, and
// structured logs.
func Redact(text string, secrets ...string) string {
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			continue
		}
		text = strings.ReplaceAll(text, secret, RedactedPlaceholder)
	}
	return text
}
