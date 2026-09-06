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

// RemoteBranchTracking records, for one local branch, which remote branch it
// tracks and the last wire commit ID known to have been synchronized with
// Rack for that branch (set after a successful `spl remote branch create` or
// a push that establishes tracking for a not-yet-tracked branch).
type RemoteBranchTracking struct {
	// RemoteBranch is the tracked remote branch name.
	RemoteBranch string `json:"remoteBranch" toml:"remote_branch"`
	// RemoteHeadCommit is the last wire commit ID known for RemoteBranch.
	RemoteHeadCommit string `json:"remoteHeadCommit" toml:"remote_head_commit"`
}

// RemoteBranchTracking returns the remote-branch tracking metadata recorded
// for localBranch, or ok=false if localBranch has no tracked remote branch.
func (r *Repository) RemoteBranchTracking(localBranch string) (RemoteBranchTracking, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.ensureOpenLocked(); err != nil {
		return RemoteBranchTracking{}, false, err
	}
	tracking, ok := r.remoteBranchTracking[localBranch]
	return tracking, ok, nil
}

// ErrInvalidRemoteBranchTracking reports that a caller attempted to record
// remote-branch tracking metadata with an invalid local branch name, remote
// branch name, or head commit. Persisting an invalid entry would make the
// repository fail to reopen (loadControlState rejects invalid entries), so
// SetRemoteBranchTracking validates before ever touching durable state.
var ErrInvalidRemoteBranchTracking = errors.New("remote branch tracking entry is invalid")

// SetRemoteBranchTracking durably records that localBranch tracks
// remoteBranch at remoteHeadCommit, overwriting any prior tracking entry for
// localBranch.
func (r *Repository) SetRemoteBranchTracking(localBranch, remoteBranch, remoteHeadCommit string) error {
	if !validRefName(localBranch) {
		return fmt.Errorf("%w: local branch %q", ErrInvalidRemoteBranchTracking, localBranch)
	}
	if !validRefName(remoteBranch) {
		return fmt.Errorf("%w: remote branch %q", ErrInvalidRemoteBranchTracking, remoteBranch)
	}
	if remoteHeadCommit == "" {
		return fmt.Errorf("%w: remote head commit is required for local branch %q", ErrInvalidRemoteBranchTracking, localBranch)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return err
	}
	previous, hadPrevious := r.remoteBranchTracking[localBranch]
	r.remoteBranchTracking[localBranch] = RemoteBranchTracking{RemoteBranch: remoteBranch, RemoteHeadCommit: remoteHeadCommit}
	if err := r.writeConfigLocked(); err != nil {
		if hadPrevious {
			r.remoteBranchTracking[localBranch] = previous
		} else {
			delete(r.remoteBranchTracking, localBranch)
		}
		return fmt.Errorf("persist remote branch tracking: %w", err)
	}
	return nil
}
