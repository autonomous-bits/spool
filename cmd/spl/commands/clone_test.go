package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/klauspost/compress/zstd"
)

func TestCloneCommandWithURL(t *testing.T) {
	ctx := context.Background()
	source := newTestSeedRepository(t)
	stageAndCommit(t, source, "idea-101", "Distributed Ideas", "alice", "seed distributed idea")

	fullPack, err := source.BuildPushPack(ctx, "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack: %v", err)
	}

	envelope := buildTestPullEnvelope(t, fullPack.TargetCommit, [][]byte{fullPack.PackData})
	var compressed bytes.Buffer
	enc, err := zstd.NewWriter(&compressed)
	if err != nil {
		t.Fatalf("new zstd writer: %v", err)
	}
	if _, err := enc.Write(envelope); err != nil {
		t.Fatalf("write envelope: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("close zstd writer: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workspaces/ws-backend/clone" {
			t.Errorf("path = %q, want /api/v1/workspaces/ws-backend/clone", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/vnd.spool-rack.pull-envelope")
		w.Header().Set("Content-Encoding", "zstd")
		w.Header().Set("X-Spool-Pull-Format", "2")
		w.Header().Set("X-Spool-Head-Commit", fullPack.TargetCommit)
		w.Header().Set("X-Spool-Branch", "main")
		w.Header().Set("X-Spool-Default-Branch", "main")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(compressed.Bytes())
	}))
	defer server.Close()

	destDir := filepath.Join(t.TempDir(), "cloned-workspace")

	var output bytes.Buffer
	cmd := NewCloneCommand()
	cmd.SetOut(&output)
	cmd.SetArgs([]string{
		server.URL + "/api/v1/workspaces/ws-backend",
		destDir,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute clone: %v", err)
	}

	var res cloneResult
	if err := json.Unmarshal(output.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal clone result: %v", err)
	}
	if !res.Cloned {
		t.Errorf("Cloned = false, want true")
	}
	if res.Branch != "main" {
		t.Errorf("Branch = %q, want main", res.Branch)
	}
	if res.HeadCommit != fullPack.TargetCommit {
		t.Errorf("HeadCommit = %q, want %q", res.HeadCommit, fullPack.TargetCommit)
	}
	if res.CommitsInstalled != 1 {
		t.Errorf("CommitsInstalled = %d, want 1", res.CommitsInstalled)
	}

	// Verify opened repo in destDir works
	repo, err := repository.OpenRepository(filepath.Join(destDir, ".spl"))
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}
	defer repo.Close()

	commitID, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}
	resolution, err := repo.ResolvePinned(commitID, "idea-101")
	if err != nil {
		t.Fatalf("ResolvePinned: %v", err)
	}
	if resolution.Node.Title != "Distributed Ideas" {
		t.Fatalf("resolution node title = %q, want Distributed Ideas", resolution.Node.Title)
	}

	// Verify the cloned repo can stage and commit new ideas immediately
	stageAndCommit(t, repo, "idea-102", "Second Idea", "carol", "commit in clone")
	newCommitID, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch after commit: %v", err)
	}
	if newCommitID == commitID {
		t.Fatalf("expected new commit ID, got same %s", newCommitID)
	}
	newRes, err := repo.ResolvePinned(newCommitID, "idea-102")
	if err != nil {
		t.Fatalf("ResolvePinned idea-102: %v", err)
	}
	if newRes.Node.Title != "Second Idea" {
		t.Fatalf("resolution node title = %q, want Second Idea", newRes.Node.Title)
	}

	// Verify the cloned repo can generate push packs to push back to remote
	pushPack, err := repo.BuildPushPack(ctx, "main", fullPack.TargetCommit)
	if err != nil {
		t.Fatalf("BuildPushPack from clone: %v", err)
	}
	if pushPack.TargetCommit == "" {
		t.Fatalf("push pack has empty target commit")
	}
}

func TestCloneCommandWithFlags(t *testing.T) {
	ctx := context.Background()
	source := newTestSeedRepository(t)
	stageAndCommit(t, source, "idea-201", "Flag Idea", "bob", "flag idea")

	fullPack, err := source.BuildPushPack(ctx, "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack: %v", err)
	}

	envelope := buildTestPullEnvelope(t, fullPack.TargetCommit, [][]byte{fullPack.PackData})
	var compressed bytes.Buffer
	enc, _ := zstd.NewWriter(&compressed)
	_, _ = enc.Write(envelope)
	_ = enc.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Tenant-ID") != "tenant-acme" {
			t.Errorf("X-Tenant-ID = %q, want tenant-acme", r.Header.Get("X-Tenant-ID"))
		}
		w.Header().Set("Content-Type", "application/vnd.spool-rack.pull-envelope")
		w.Header().Set("Content-Encoding", "zstd")
		w.Header().Set("X-Spool-Pull-Format", "2")
		w.Header().Set("X-Spool-Head-Commit", fullPack.TargetCommit)
		w.Header().Set("X-Spool-Branch", "main")
		w.Header().Set("X-Spool-Default-Branch", "main")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(compressed.Bytes())
	}))
	defer server.Close()

	destDir := filepath.Join(t.TempDir(), "cloned-by-flags")

	var output bytes.Buffer
	cmd := NewCloneCommand()
	cmd.SetOut(&output)
	cmd.SetArgs([]string{
		"--endpoint", server.URL,
		"--tenant-id", "tenant-acme",
		"--workspace-id", "ws-core",
		destDir,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute clone with flags: %v", err)
	}

	var res cloneResult
	if err := json.Unmarshal(output.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal clone result: %v", err)
	}
	if res.WorkspaceID != "ws-core" || res.TenantID != "tenant-acme" {
		t.Fatalf("res = %+v", res)
	}
}

func TestCloneCommandRejectsNonEmptyDestination(t *testing.T) {
	destDir := filepath.Join(t.TempDir(), "non-empty-dir")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destDir, "some-file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewCloneCommand()
	cmd.SetArgs([]string{
		"http://127.0.0.1:8080/api/v1/workspaces/ws-1",
		destDir,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when destination is not empty, got nil")
	}
}

func TestWorkspaceCloneSubcommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Spool-Empty", "true")
		w.Header().Set("X-Spool-Branch", "main")
		w.Header().Set("X-Spool-Default-Branch", "main")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	destDir := filepath.Join(t.TempDir(), "cloned-via-workspace-cmd")

	var output bytes.Buffer
	wsCmd := NewWorkspaceCommandDefault()
	wsCmd.SetOut(&output)
	wsCmd.SetArgs([]string{
		"clone",
		server.URL + "/workspaces/ws-subcommand",
		destDir,
	})

	if err := wsCmd.Execute(); err != nil {
		t.Fatalf("execute workspace clone: %v", err)
	}

	var res cloneResult
	if err := json.Unmarshal(output.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal clone result: %v", err)
	}
	if !res.Cloned || !res.Empty {
		t.Fatalf("res = %+v", res)
	}
}

func TestParseURLPathAndQuery(t *testing.T) {
	tests := []struct {
		rawURL      string
		wantTenant  string
		wantWs      string
		wantRepo    string
		wantBranch  string
		wantAuth    string
	}{
		{
			rawURL:     "http://localhost:8080/api/v1/workspaces/ws-1",
			wantWs:     "ws-1",
		},
		{
			rawURL:     "http://localhost:8080/workspaces/ws-2?branch=feature&auth_mode=api_key",
			wantWs:     "ws-2",
			wantBranch: "feature",
			wantAuth:   "api_key",
		},
		{
			rawURL:     "http://localhost:8080/tenants/t-1/workspaces/ws-3",
			wantTenant: "t-1",
			wantWs:     "ws-3",
		},
		{
			rawURL:     "http://localhost:8080/api/v1/repos/repo-4?tenant=t-4",
			wantTenant: "t-4",
			wantRepo:   "repo-4",
		},
	}

	for _, tc := range tests {
		u, err := url.Parse(tc.rawURL)
		if err != nil {
			t.Fatalf("url.Parse(%s): %v", tc.rawURL, err)
		}
		var tenant, ws, repo, branch, auth string
		parseURLPathAndQuery(u, &tenant, &ws, &repo, &branch, &auth)
		if tenant != tc.wantTenant || ws != tc.wantWs || repo != tc.wantRepo || branch != tc.wantBranch || auth != tc.wantAuth {
			t.Errorf("parseURLPathAndQuery(%s) = (%q, %q, %q, %q, %q), want (%q, %q, %q, %q, %q)",
				tc.rawURL, tenant, ws, repo, branch, auth,
				tc.wantTenant, tc.wantWs, tc.wantRepo, tc.wantBranch, tc.wantAuth)
		}
	}
}
