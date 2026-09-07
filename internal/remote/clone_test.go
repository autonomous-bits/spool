package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestCloneDirectEndpointSuccess(t *testing.T) {
	t.Parallel()

	pack := []byte("pack data")
	head := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	envelope := buildTestPullEnvelope(t, head, [][]byte{pack})

	var compressed bytes.Buffer
	enc, err := zstd.NewWriter(&compressed)
	if err != nil {
		t.Fatalf("new zstd writer: %v", err)
	}
	if _, err := enc.Write(envelope); err != nil {
		t.Fatalf("compress envelope: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("close zstd writer: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workspaces/ws-1/clone" {
			t.Errorf("path = %q, want /api/v1/workspaces/ws-1/clone", r.URL.Path)
		}
		if r.Header.Get("X-Tenant-ID") != "tenant-1" {
			t.Errorf("X-Tenant-ID = %q, want tenant-1", r.Header.Get("X-Tenant-ID"))
		}
		if r.Header.Get("Authorization") != "Bearer dev-token" {
			t.Errorf("Authorization = %q, want Bearer dev-token", r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "application/vnd.spool-rack.pull-envelope")
		w.Header().Set("Content-Encoding", "zstd")
		w.Header().Set("X-Spool-Pull-Format", "2")
		w.Header().Set("X-Spool-Head-Commit", head)
		w.Header().Set("X-Spool-Branch", "main")
		w.Header().Set("X-Spool-Default-Branch", "main")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(compressed.Bytes())
	}))
	defer server.Close()

	cfg := Config{
		Endpoint:    server.URL,
		TenantID:    "tenant-1",
		WorkspaceID: "ws-1",
		AuthMode:    AuthModeBearer,
	}

	res, err := Clone(context.Background(), nil, cfg, "dev-token", "")
	if err != nil {
		t.Fatalf("Clone() error = %v", err)
	}
	if res.Branch != "main" {
		t.Errorf("Branch = %q, want main", res.Branch)
	}
	if res.DefaultBranch != "main" {
		t.Errorf("DefaultBranch = %q, want main", res.DefaultBranch)
	}
	if res.HeadCommit != head {
		t.Errorf("HeadCommit = %q, want %q", res.HeadCommit, head)
	}
	if len(res.Packs) != 1 || !bytes.Equal(res.Packs[0], pack) {
		t.Errorf("Packs = %q, want [%q]", res.Packs, pack)
	}
	if res.Empty {
		t.Errorf("Empty = true, want false")
	}
}

func TestCloneDirectEndpointEmptyWorkspace(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Spool-Empty", "true")
		w.Header().Set("X-Spool-Branch", "main")
		w.Header().Set("X-Spool-Default-Branch", "main")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := Config{
		Endpoint:    server.URL,
		TenantID:    "tenant-1",
		WorkspaceID: "ws-1",
		AuthMode:    AuthModeBearer,
	}

	res, err := Clone(context.Background(), nil, cfg, "dev-token", "")
	if err != nil {
		t.Fatalf("Clone() error = %v", err)
	}
	if !res.Empty {
		t.Errorf("Empty = false, want true")
	}
	if res.Branch != "main" {
		t.Errorf("Branch = %q, want main", res.Branch)
	}
}

func TestCloneFallbackWhenCloneEndpointNotFound(t *testing.T) {
	t.Parallel()

	pack := []byte("fallback pack")
	head := "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	envelope := buildTestPullEnvelope(t, head, [][]byte{pack})

	var compressed bytes.Buffer
	enc, _ := zstd.NewWriter(&compressed)
	_, _ = enc.Write(envelope)
	_ = enc.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/workspaces/ws-1/clone":
			w.WriteHeader(http.StatusNotFound)
		case "/api/v1/workspaces/ws-1/branches/default":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(BranchResult{Name: "main", HeadCommit: head})
		case "/api/v1/workspaces/ws-1/pull":
			if r.URL.Query().Get("branch") != "main" {
				t.Errorf("pull branch = %q, want main", r.URL.Query().Get("branch"))
			}
			w.Header().Set("Content-Type", "application/vnd.spool-rack.pull-envelope")
			w.Header().Set("Content-Encoding", "zstd")
			w.Header().Set("X-Spool-Pull-Format", "2")
			w.Header().Set("X-Spool-Head-Commit", head)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(compressed.Bytes())
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cfg := Config{
		Endpoint:    server.URL,
		TenantID:    "tenant-1",
		WorkspaceID: "ws-1",
		AuthMode:    AuthModeBearer,
	}

	res, err := Clone(context.Background(), nil, cfg, "dev-token", "")
	if err != nil {
		t.Fatalf("Clone() fallback error = %v", err)
	}
	if res.Branch != "main" || res.HeadCommit != head || len(res.Packs) != 1 {
		t.Errorf("Clone() result = %+v, want branch:main head:%s", res, head)
	}
}

func TestCloneStructured404DoesNotFallback(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/workspaces/ws-missing/clone" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "not_found",
				"message": "workspace not found",
			})
			return
		}
		t.Errorf("unexpected path hit: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := Config{
		Endpoint:    server.URL,
		TenantID:    "tenant-1",
		WorkspaceID: "ws-missing",
		AuthMode:    AuthModeBearer,
	}

	_, err := Clone(context.Background(), nil, cfg, "dev-token", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrPullRejected) {
		t.Errorf("expected ErrPullRejected, got %v", err)
	}
}
