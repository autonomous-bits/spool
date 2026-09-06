package repository

import (
	"errors"
	"fmt"

	"github.com/autonomous-bits/spool/internal/remote"
)

// Remote types and auth modes re-exported from the remote package so CLI
// commands and callers never need to import internal/remote's config shape
// directly for persisted state.
type (
	RemoteConfig   = remote.Config
	RemoteAuthMode = remote.AuthMode
)

// Remote auth modes re-exported from the remote package.
const (
	RemoteAuthModeBearer = remote.AuthModeBearer
	RemoteAuthModeAPIKey = remote.AuthModeAPIKey
)

// Remote sentinel errors re-exported from the remote package.
var (
	ErrRemoteInvalidConfig   = remote.ErrInvalidConfig
	ErrRemoteSecretLikeValue = remote.ErrSecretLikeValue
)

// ErrRemoteNotConfigured reports that the repository has no configured
// remote.
var ErrRemoteNotConfigured = errors.New("repository has no configured remote")

// SetRemote validates and durably persists cfg as the repository's Rack
// remote configuration. cfg must never contain credentials: SetRemote
// rejects endpoint or repo_id values that look like pasted-in secrets.
func (r *Repository) SetRemote(cfg RemoteConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return err
	}
	previous := r.remote
	normalized := cfg
	r.remote = &normalized
	if err := r.writeConfigLocked(); err != nil {
		r.remote = previous
		return fmt.Errorf("persist remote configuration: %w", err)
	}
	return nil
}

// RemoveRemote durably clears any configured Rack remote. It is a no-op if
// no remote is configured.
func (r *Repository) RemoveRemote() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return err
	}
	if r.remote == nil {
		return nil
	}
	previous := r.remote
	r.remote = nil
	if err := r.writeConfigLocked(); err != nil {
		r.remote = previous
		return fmt.Errorf("remove remote configuration: %w", err)
	}
	return nil
}

// Remote returns the repository's configured Rack remote, or ok=false if
// none is configured.
func (r *Repository) Remote() (RemoteConfig, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.ensureOpenLocked(); err != nil {
		return RemoteConfig{}, false, err
	}
	if r.remote == nil {
		return RemoteConfig{}, false, nil
	}
	return *r.remote, true, nil
}
