package repository

import (
	"context"
	"path/filepath"
	"testing"
)

func TestInitializeClonedRepositoryWithPacks(t *testing.T) {
	ctx := context.Background()
	source := newTestSeedRepository(t)

	commitTestMutation(t, source, "idea-1", "First Idea", "alice", "create first idea")
	commitTestMutation(t, source, "idea-2", "Second Idea", "bob", "create second idea")

	fullPack, err := source.BuildPushPack(ctx, "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack (full): %v", err)
	}

	cloneDir := filepath.Join(t.TempDir(), ".spl")
	remoteCfg := RemoteConfig{
		Endpoint:    "http://127.0.0.1:8080",
		TenantID:    "tenant-1",
		WorkspaceID: "ws-1",
		AuthMode:    RemoteAuthModeBearer,
	}

	cloned, err := InitializeClonedRepository(cloneDir, remoteCfg, "main", fullPack.TargetCommit, [][]byte{fullPack.PackData})
	if err != nil {
		t.Fatalf("InitializeClonedRepository: %v", err)
	}
	defer func() {
		if err := cloned.Close(); err != nil {
			t.Fatalf("close cloned repo: %v", err)
		}
	}()

	// Verify nodes are resolved in cloned repository
	for _, id := range []string{"idea-1", "idea-2"} {
		head := cloned.branches["main"]
		resolution, err := cloned.ResolvePinned(head, id)
		if err != nil {
			t.Fatalf("ResolvePinned(%s): %v", id, err)
		}
		if resolution.Node.ID != id {
			t.Fatalf("resolved node ID = %q, want %q", resolution.Node.ID, id)
		}
	}

	// Verify remote configuration
	cfg, ok, err := cloned.Remote()
	if err != nil || !ok {
		t.Fatalf("Remote() ok = %v, err = %v", ok, err)
	}
	if cfg.Endpoint != remoteCfg.Endpoint || cfg.WorkspaceID != remoteCfg.WorkspaceID {
		t.Fatalf("Remote() cfg = %+v, want %+v", cfg, remoteCfg)
	}

	// Verify remote branch tracking
	tracking, ok, err := cloned.RemoteBranchTracking("main")
	if err != nil || !ok {
		t.Fatalf("RemoteBranchTracking(main) ok = %v, err = %v", ok, err)
	}
	if tracking.RemoteBranch != "main" || tracking.RemoteHeadCommit != fullPack.TargetCommit {
		t.Fatalf("tracking = %+v, want main and %q", tracking, fullPack.TargetCommit)
	}

	// Verify new commit can be added and pushed on top of cloned history
	commitTestMutation(t, cloned, "idea-3", "Third Idea", "carol", "create third idea")
	newPack, err := cloned.BuildPushPack(ctx, "main", fullPack.TargetCommit)
	if err != nil {
		t.Fatalf("BuildPushPack from cloned head: %v", err)
	}
	if newPack.BaseCommit != fullPack.TargetCommit {
		t.Fatalf("newPack.BaseCommit = %q, want %q", newPack.BaseCommit, fullPack.TargetCommit)
	}
	if len(newPack.Commits) != 1 {
		t.Fatalf("newPack.Commits len = %d, want 1", len(newPack.Commits))
	}
}

func TestInitializeClonedRepositoryEmpty(t *testing.T) {
	cloneDir := filepath.Join(t.TempDir(), ".spl")
	remoteCfg := RemoteConfig{
		Endpoint:    "http://127.0.0.1:8080",
		TenantID:    "tenant-1",
		WorkspaceID: "ws-empty",
		AuthMode:    RemoteAuthModeBearer,
	}

	cloned, err := InitializeClonedRepository(cloneDir, remoteCfg, "main", "", nil)
	if err != nil {
		t.Fatalf("InitializeClonedRepository empty: %v", err)
	}
	defer func() {
		if err := cloned.Close(); err != nil {
			t.Fatalf("close cloned repo: %v", err)
		}
	}()

	cfg, ok, err := cloned.Remote()
	if err != nil || !ok {
		t.Fatalf("Remote() ok = %v, err = %v", ok, err)
	}
	if cfg.WorkspaceID != "ws-empty" {
		t.Fatalf("cfg.WorkspaceID = %q, want ws-empty", cfg.WorkspaceID)
	}
	if cloned.defaultBranch != "main" {
		t.Fatalf("defaultBranch = %q, want main", cloned.defaultBranch)
	}
}
