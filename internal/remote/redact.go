package remote

import "strings"

// RedactedPlaceholder replaces any redacted secret value.
const RedactedPlaceholder = "***redacted***"

// caseInsensitiveSecretPrefixes lists common credential/token prefixes that
// are conventionally lowercase regardless of where they're pasted, and that
// must never be stored as non-secret remote configuration values.
var caseInsensitiveSecretPrefixes = []string{
	"ghp_", "gho_", "ghu_", "ghs_", "ghr_", "github_pat_", "sk-", "xox",
}

// caseSensitiveSecretPrefixes lists credential/token prefixes whose real-world
// casing convention is significant for distinguishing an actual token from an
// ordinary word that merely starts with the same letters, e.g. an AWS access
// key ID always starts with the literal uppercase "AKIA", and a JWT always
// starts with the literal "eyJ" (lowercase e/y, uppercase J) because that is
// the base64 encoding of a JSON header's leading `{"`. Matching case
// preserves the heuristic's intent while avoiding false positives on ordinary
// values such as "akiametrics" or "eyjafjallajokull".
var caseSensitiveSecretPrefixes = []string{"AKIA", "eyJ"}

// looksLikeSecret applies conservative heuristics to flag values that are
// very likely pasted-in credentials rather than portable, non-secret
// configuration values. Prefixes are matched at the start of value (after
// trimming surrounding whitespace), not merely as a substring anywhere
// within it, so ordinary values that happen to contain a prefix mid-string
// are not flagged.
func looksLikeSecret(value string) bool {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "bearer ") {
		return true
	}
	for _, prefix := range caseSensitiveSecretPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	for _, prefix := range caseInsensitiveSecretPrefixes {
		if strings.HasPrefix(lower, prefix) {
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
