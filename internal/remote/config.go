// Package remote resolves Rack remote configuration and credentials for the
// Spool CLI without ever persisting secrets to durable repository state.
package remote

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// AuthMode names a supported Rack authentication scheme.
type AuthMode string

const (
	// AuthModeBearer authenticates with a bearer token.
	AuthModeBearer AuthMode = "bearer"
	// AuthModeAPIKey authenticates with a static API key.
	AuthModeAPIKey AuthMode = "api_key"
)

// maxRepoIDLength bounds the logical repository/tenant identifier. Real
// identifiers are short human-chosen names; anything longer is far more
// likely to be an accidentally pasted credential.
const maxRepoIDLength = 64

var (
	// ErrInvalidConfig reports a remote configuration that fails validation.
	ErrInvalidConfig = errors.New("remote configuration is invalid")
	// ErrSecretLikeValue reports a non-secret field whose value resembles a
	// credential and must not be persisted to repository configuration.
	ErrSecretLikeValue = errors.New("value looks like a credential and cannot be stored as remote configuration")
)

// Config is the versioned, non-secret Rack remote configuration persisted in
// a repository's control state. It never contains credentials.
type Config struct {
	// Endpoint is the Rack HTTP(S) base URL.
	Endpoint string `toml:"endpoint"`
	// RepoID is the logical Rack repository/tenant identity.
	RepoID string `toml:"repo_id"`
	// AuthMode selects how credentials for this remote are authenticated.
	AuthMode AuthMode `toml:"auth_mode"`
}

// Validate reports whether c is a well-formed, non-secret remote
// configuration. It rejects missing fields, malformed endpoints, unsupported
// auth modes, and values that look like pasted-in credentials.
func (c Config) Validate() error {
	endpoint := strings.TrimSpace(c.Endpoint)
	if endpoint == "" {
		return fmt.Errorf("%w: endpoint is required", ErrInvalidConfig)
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("%w: endpoint is not a valid URL: %w", ErrInvalidConfig, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: endpoint must use http or https", ErrInvalidConfig)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%w: endpoint must include a host", ErrInvalidConfig)
	}
	if parsed.User != nil {
		return fmt.Errorf("%w: endpoint", ErrSecretLikeValue)
	}
	if looksLikeSecret(endpoint) {
		return fmt.Errorf("%w: endpoint", ErrSecretLikeValue)
	}

	repoID := strings.TrimSpace(c.RepoID)
	if repoID == "" {
		return fmt.Errorf("%w: repo_id is required", ErrInvalidConfig)
	}
	if len(repoID) > maxRepoIDLength {
		return fmt.Errorf("%w: repo_id", ErrSecretLikeValue)
	}
	if strings.ContainsAny(repoID, " \t\n\r") {
		return fmt.Errorf("%w: repo_id must not contain whitespace", ErrInvalidConfig)
	}
	if looksLikeSecret(repoID) {
		return fmt.Errorf("%w: repo_id", ErrSecretLikeValue)
	}

	switch c.AuthMode {
	case AuthModeBearer, AuthModeAPIKey:
	default:
		return fmt.Errorf("%w: auth_mode must be %q or %q", ErrInvalidConfig, AuthModeBearer, AuthModeAPIKey)
	}
	return nil
}

// EnvVar returns the conventional environment variable name used to resolve
// a credential for mode.
func EnvVar(mode AuthMode) (string, error) {
	switch mode {
	case AuthModeBearer:
		return "SPOOL_RACK_TOKEN", nil
	case AuthModeAPIKey:
		return "SPOOL_RACK_API_KEY", nil
	default:
		return "", fmt.Errorf("%w: unsupported auth mode %q", ErrInvalidConfig, mode)
	}
}
