package remote

import (
	"errors"
	"fmt"
	"os"
)

// CredentialSource identifies where a resolved credential came from.
type CredentialSource string

const (
	// CredentialSourceKeychain reports a credential resolved from the OS
	// keychain or secret store.
	CredentialSourceKeychain CredentialSource = "keychain"
	// CredentialSourceEnv reports a credential resolved from an explicit
	// process environment variable.
	CredentialSourceEnv CredentialSource = "env"
	// CredentialSourceInteractive reports a credential resolved from an
	// interactive TTY prompt.
	CredentialSourceInteractive CredentialSource = "interactive"
)

// ErrCredentialNotFound reports that no credential could be resolved from
// any configured source.
var ErrCredentialNotFound = errors.New("no credential available for remote authentication")

// Credential is a resolved secret value and the source it came from. It is
// never persisted to disk.
type Credential struct {
	Value  string
	Source CredentialSource
}

// KeychainStore resolves a credential from an OS-native keychain or secret
// store. Lookup must return ErrCredentialNotFound when no entry exists.
type KeychainStore interface {
	Lookup(service, account string) (string, error)
}

// PromptFunc requests a secret value interactively, typically reading hidden
// input from a TTY. Implementations must return ErrCredentialNotFound (or
// wrap it) when no interactive input is available, rather than blocking
// indefinitely.
type PromptFunc func(label string) (string, error)

// ResolveOptions configures ResolveCredential's credential sources. Any
// field may be nil/zero to skip that source.
type ResolveOptions struct {
	// Keychain resolves a credential from an OS secret store.
	Keychain KeychainStore
	// Getenv resolves a credential from the process environment. Defaults to
	// os.Getenv when nil via ResolveCredential's caller.
	Getenv func(string) string
	// Prompt resolves a credential interactively as a last resort.
	Prompt PromptFunc
}

// keychainService is the service name used for every Rack keychain entry.
const keychainService = "spool-rack"

// ResolveCredential resolves a credential for repoID under mode, trying the
// OS keychain, then the conventional environment variable, then an
// interactive prompt, in that order. The resolved value is never persisted
// to disk. It returns ErrCredentialNotFound if no source yields a value.
func ResolveCredential(repoID string, mode AuthMode, opts ResolveOptions) (Credential, error) {
	envVar, err := EnvVar(mode)
	if err != nil {
		return Credential{}, err
	}

	if opts.Keychain != nil {
		if value, err := opts.Keychain.Lookup(keychainService, repoID); err == nil && value != "" {
			return Credential{Value: value, Source: CredentialSourceKeychain}, nil
		}
	}

	if value := getenv(opts.Getenv)(envVar); value != "" {
		return Credential{Value: value, Source: CredentialSourceEnv}, nil
	}

	if opts.Prompt != nil {
		value, err := opts.Prompt(fmt.Sprintf("Enter Rack %s for %s: ", authModeLabel(mode), repoID))
		if err != nil {
			if errors.Is(err, ErrCredentialNotFound) {
				return Credential{}, ErrCredentialNotFound
			}
			return Credential{}, fmt.Errorf("prompt for credential: %w", err)
		}
		if value != "" {
			return Credential{Value: value, Source: CredentialSourceInteractive}, nil
		}
	}

	return Credential{}, ErrCredentialNotFound
}

func authModeLabel(mode AuthMode) string {
	if mode == AuthModeAPIKey {
		return "API key"
	}
	return "bearer token"
}

// getenv returns fn, or os.Getenv if fn is nil, so ResolveCredential's
// environment-variable fallback matches ResolveOptions.Getenv's documented
// default without every caller needing to wire os.Getenv explicitly.
func getenv(fn func(string) string) func(string) string {
	if fn != nil {
		return fn
	}
	return os.Getenv
}
