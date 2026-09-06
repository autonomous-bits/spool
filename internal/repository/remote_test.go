package repository

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validRemoteConfig() RemoteConfig {
	return RemoteConfig{Endpoint: "https://rack.example.com", RepoID: "acme-prod", AuthMode: RemoteAuthModeBearer}
}

func TestSetRemotePersistsAndRoundTripsThroughReopen(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	if err := repo.SetRemote(validRemoteConfig()); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}
	closeTestRepository(t, reopened)
	cfg, ok, err := reopened.Remote()
	if err != nil {
		t.Fatalf("Remote: %v", err)
	}
	if !ok {
		t.Fatal("Remote ok = false, want true after reopen")
	}
	if cfg != validRemoteConfig() {
		t.Fatalf("Remote() = %#v, want %#v", cfg, validRemoteConfig())
	}
}

func TestRemoveRemoteClearsPersistedConfig(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	if err := repo.SetRemote(validRemoteConfig()); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	if err := repo.RemoveRemote(); err != nil {
		t.Fatalf("RemoveRemote: %v", err)
	}
	if _, ok, err := repo.Remote(); err != nil || ok {
		t.Fatalf("Remote() after RemoveRemote = ok=%v, err=%v, want ok=false", ok, err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}
	closeTestRepository(t, reopened)
	if _, ok, err := reopened.Remote(); err != nil || ok {
		t.Fatalf("Remote() after reopen = ok=%v, err=%v, want ok=false", ok, err)
	}
}

func TestRemoveRemoteIsNoOpWithoutConfiguredRemote(t *testing.T) {
	repo, err := NewSeedRepository()
	if err != nil {
		t.Fatalf("NewSeedRepository: %v", err)
	}
	if err := repo.RemoveRemote(); err != nil {
		t.Fatalf("RemoveRemote: %v", err)
	}
}

func TestSetRemoteRejectsSecretLikeRepoID(t *testing.T) {
	repo, err := NewSeedRepository()
	if err != nil {
		t.Fatalf("NewSeedRepository: %v", err)
	}
	cfg := validRemoteConfig()
	cfg.RepoID = "ghp_1234567890abcdefghijklmnopqrstuvwxyz"
	if err := repo.SetRemote(cfg); !errors.Is(err, ErrRemoteSecretLikeValue) {
		t.Fatalf("SetRemote error = %v, want ErrRemoteSecretLikeValue", err)
	}
	if _, ok, err := repo.Remote(); err != nil || ok {
		t.Fatalf("Remote() after rejected SetRemote = ok=%v, err=%v, want ok=false", ok, err)
	}
}

func TestSetRemoteRejectsInvalidConfig(t *testing.T) {
	repo, err := NewSeedRepository()
	if err != nil {
		t.Fatalf("NewSeedRepository: %v", err)
	}
	cfg := validRemoteConfig()
	cfg.AuthMode = "oauth"
	if err := repo.SetRemote(cfg); !errors.Is(err, ErrRemoteInvalidConfig) {
		t.Fatalf("SetRemote error = %v, want ErrRemoteInvalidConfig", err)
	}
}

func TestUnrelatedRepositoryMutationsPreserveRemoteConfig(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	if err := repo.SetRemote(validRemoteConfig()); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	if _, err := repo.CreateBranch("feature", BranchSource{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if _, err := repo.SwitchBranch("feature"); err != nil {
		t.Fatalf("SwitchBranch: %v", err)
	}
	if cfg, ok, err := repo.Remote(); err != nil || !ok || cfg != validRemoteConfig() {
		t.Fatalf("Remote() after unrelated mutations = %#v, %v, %v", cfg, ok, err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}
	closeTestRepository(t, reopened)
	if cfg, ok, err := reopened.Remote(); err != nil || !ok || cfg != validRemoteConfig() {
		t.Fatalf("Remote() after reopen = %#v, %v, %v", cfg, ok, err)
	}
}

func TestSetRemoteBranchTrackingPersistsAndRoundTripsThroughReopen(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	if err := repo.SetRemoteBranchTracking("main", "main", "abc123"); err != nil {
		t.Fatalf("SetRemoteBranchTracking: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}
	closeTestRepository(t, reopened)
	tracking, ok, err := reopened.RemoteBranchTracking("main")
	if err != nil {
		t.Fatalf("RemoteBranchTracking: %v", err)
	}
	if !ok {
		t.Fatal("RemoteBranchTracking ok = false, want true after reopen")
	}
	want := RemoteBranchTracking{RemoteBranch: "main", RemoteHeadCommit: "abc123"}
	if tracking != want {
		t.Fatalf("RemoteBranchTracking() = %#v, want %#v", tracking, want)
	}
}

func TestSetRemoteBranchTrackingOverwritesPriorEntry(t *testing.T) {
	repo, err := NewSeedRepository()
	if err != nil {
		t.Fatalf("NewSeedRepository: %v", err)
	}
	if err := repo.SetRemoteBranchTracking("main", "main", "abc123"); err != nil {
		t.Fatalf("SetRemoteBranchTracking: %v", err)
	}
	if err := repo.SetRemoteBranchTracking("main", "main", "def456"); err != nil {
		t.Fatalf("SetRemoteBranchTracking: %v", err)
	}
	tracking, ok, err := repo.RemoteBranchTracking("main")
	if err != nil || !ok {
		t.Fatalf("RemoteBranchTracking: ok=%v, err=%v", ok, err)
	}
	if tracking.RemoteHeadCommit != "def456" {
		t.Fatalf("RemoteHeadCommit = %q, want def456", tracking.RemoteHeadCommit)
	}
}

func TestRemoteBranchTrackingUnknownBranchReturnsNotOK(t *testing.T) {
	repo, err := NewSeedRepository()
	if err != nil {
		t.Fatalf("NewSeedRepository: %v", err)
	}
	if _, ok, err := repo.RemoteBranchTracking("missing"); err != nil || ok {
		t.Fatalf("RemoteBranchTracking() = ok=%v, err=%v, want ok=false", ok, err)
	}
}

func TestSetRemoteBranchTrackingRejectsInvalidEntryWithoutPersisting(t *testing.T) {
	tests := map[string]struct {
		localBranch      string
		remoteBranch     string
		remoteHeadCommit string
	}{
		"empty local branch":       {localBranch: "", remoteBranch: "main", remoteHeadCommit: "abc123"},
		"invalid local branch":     {localBranch: "..", remoteBranch: "main", remoteHeadCommit: "abc123"},
		"empty remote branch":      {localBranch: "main", remoteBranch: "", remoteHeadCommit: "abc123"},
		"invalid remote branch":    {localBranch: "main", remoteBranch: "/leading-slash", remoteHeadCommit: "abc123"},
		"empty remote head commit": {localBranch: "main", remoteBranch: "main", remoteHeadCommit: ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			repo, err := NewSeedRepository()
			if err != nil {
				t.Fatalf("NewSeedRepository: %v", err)
			}
			err = repo.SetRemoteBranchTracking(tc.localBranch, tc.remoteBranch, tc.remoteHeadCommit)
			if !errors.Is(err, ErrInvalidRemoteBranchTracking) {
				t.Fatalf("SetRemoteBranchTracking() err = %v, want ErrInvalidRemoteBranchTracking", err)
			}
			if _, ok, err := repo.RemoteBranchTracking(tc.localBranch); err != nil || ok {
				t.Fatalf("RemoteBranchTracking() = ok=%v, err=%v, want ok=false after rejected write", ok, err)
			}
		})
	}
}

func TestRemoteConfigBackwardCompatibleWithoutRemoteTable(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(stateDir, "config.toml"))
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	if strings.Contains(string(data), "[remote]") {
		t.Fatalf("config.toml unexpectedly contains [remote] table: %s", data)
	}

	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}
	closeTestRepository(t, reopened)
	if _, ok, err := reopened.Remote(); err != nil || ok {
		t.Fatalf("Remote() on legacy config = ok=%v, err=%v, want ok=false", ok, err)
	}
}
