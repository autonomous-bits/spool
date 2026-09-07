package remote

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateBranchSuccessDecodesResult(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(BranchResult{Name: "feature", HeadCommit: "abc123"})
	}))
	defer server.Close()

	result, err := CreateBranch(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "secret-token",
		BranchCreateRequest{Name: "feature", SourceBranch: "main"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if result.Name != "feature" || result.HeadCommit != "abc123" {
		t.Fatalf("result = %#v", result)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/repos/acme/branches" {
		t.Fatalf("method = %q, path = %q", gotMethod, gotPath)
	}
	if gotBody["name"] != "feature" || gotBody["sourceBranch"] != "main" {
		t.Fatalf("body = %#v", gotBody)
	}
}

func TestCreateBranchAlreadyExists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "branch_already_exists", "message": "branch already exists"})
	}))
	defer server.Close()

	_, err := CreateBranch(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "",
		BranchCreateRequest{Name: "feature", SourceBranch: "main"})
	if !errors.Is(err, ErrBranchAlreadyExists) {
		t.Fatalf("err = %v, want ErrBranchAlreadyExists", err)
	}
}

func TestCreateBranchSourceNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "branch_source_not_found", "message": "source not found"})
	}))
	defer server.Close()

	_, err := CreateBranch(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "",
		BranchCreateRequest{Name: "feature", SourceBranch: "missing"})
	if !errors.Is(err, ErrBranchSourceNotFound) {
		t.Fatalf("err = %v, want ErrBranchSourceNotFound", err)
	}
}

func TestCreateBranchRejectsMissingSourceWithoutContactingServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be contacted when the request fails local validation")
	}))
	defer server.Close()

	_, err := CreateBranch(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "",
		BranchCreateRequest{Name: "feature"})
	if !errors.Is(err, ErrBranchSourceRequired) {
		t.Fatalf("err = %v, want ErrBranchSourceRequired", err)
	}
}

func TestCreateBranchRejectsAmbiguousSourceWithoutContactingServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be contacted when the request fails local validation")
	}))
	defer server.Close()

	_, err := CreateBranch(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "",
		BranchCreateRequest{Name: "feature", SourceBranch: "main", SourceCommit: "abc123"})
	if !errors.Is(err, ErrBranchSourceAmbiguous) {
		t.Fatalf("err = %v, want ErrBranchSourceAmbiguous", err)
	}
}

func TestListBranchesSuccess(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(BranchListResult{Branches: []BranchListEntry{
			{Name: "main", HeadCommit: "abc123", Default: true},
			{Name: "feature", HeadCommit: "def456"},
		}})
	}))
	defer server.Close()

	result, err := ListBranches(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "")
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(result.Branches) != 2 || result.Branches[0].Name != "main" || !result.Branches[0].Default {
		t.Fatalf("result = %#v", result)
	}
	if gotPath != "/api/v1/repos/acme/branches" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestListBranchesRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := ListBranches(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "")
	if !errors.Is(err, ErrBranchRejected) {
		t.Fatalf("err = %v, want ErrBranchRejected", err)
	}
}

func TestDefaultBranchSuccess(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(BranchResult{Name: "main", HeadCommit: "abc123"})
	}))
	defer server.Close()

	result, err := DefaultBranch(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "")
	if err != nil {
		t.Fatalf("DefaultBranch: %v", err)
	}
	if result.Name != "main" || result.HeadCommit != "abc123" {
		t.Fatalf("result = %#v", result)
	}
	if gotPath != "/api/v1/repos/acme/branches/default" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestDeleteBranchSuccess(t *testing.T) {
	var gotPath, gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	err := DeleteBranch(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "", "feature")
	if err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/repos/acme/branches/feature" {
		t.Fatalf("method = %q, path = %q", gotMethod, gotPath)
	}
}

func TestDeleteBranchProtectedReturnsTypedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":         "branch_protected",
			"message":       "default branch cannot be deleted",
			"correlationId": "corr-protected",
		})
	}))
	defer server.Close()

	err := DeleteBranch(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "", "main")
	var protectedErr *ProtectedBranchError
	if !errors.As(err, &protectedErr) {
		t.Fatalf("err = %v, want *ProtectedBranchError", err)
	}
	if protectedErr.Guidance != "default branch cannot be deleted" || protectedErr.CorrelationID != "corr-protected" {
		t.Fatalf("protectedErr = %#v", protectedErr)
	}
}

func TestDeleteBranchEscapesSlashInName(t *testing.T) {
	var gotPath, gotRawPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotRawPath = r.URL.Path, r.URL.EscapedPath()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	err := DeleteBranch(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "", "feature/foo")
	if err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	if gotPath != "/api/v1/repos/acme/branches/feature/foo" {
		t.Fatalf("decoded path = %q", gotPath)
	}
	if gotRawPath != "/api/v1/repos/acme/branches/feature%2Ffoo" {
		t.Fatalf("raw path = %q, want branch name to be a single escaped path segment", gotRawPath)
	}
}

func TestDeleteBranchNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "branch_not_found", "message": "no such branch"})
	}))
	defer server.Close()

	err := DeleteBranch(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "", "missing")
	if !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("err = %v, want ErrBranchNotFound", err)
	}
}

func TestBranchRequestsSendCorrelationIDHeader(t *testing.T) {
	var received string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Header.Get(CorrelationIDHeader)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(BranchListResult{})
	}))
	defer server.Close()

	client := NewClient()
	if _, err := ListBranches(context.Background(), client, Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, ""); err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if received == "" || received != client.CorrelationID {
		t.Fatalf("correlation ID header = %q, want %q", received, client.CorrelationID)
	}
}

func TestCreateBranchUnreachableRemote(t *testing.T) {
	_, err := CreateBranch(context.Background(), NewClient(), Config{Endpoint: "http://127.0.0.1:1", RepoID: "acme", AuthMode: AuthModeBearer}, "",
		BranchCreateRequest{Name: "feature", SourceBranch: "main"})
	if !errors.Is(err, ErrRemoteUnreachable) {
		t.Fatalf("err = %v, want ErrRemoteUnreachable", err)
	}
}

func TestDeleteBranchCredentialNeverAppearsInErrorText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "internal", "message": "credential super-secret-token was rejected"})
	}))
	defer server.Close()

	err := DeleteBranch(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "super-secret-token", "feature")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "super-secret-token") {
		t.Fatalf("error %q leaked credential", err.Error())
	}
}

func TestBranchOperationsWithTenantAndWorkspaceRouteToWorkspacesAndSetTenantHeader(t *testing.T) {
	var gotPath, gotTenant, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotTenant = r.Header.Get("X-Tenant-ID")
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(BranchResult{Name: "feature", HeadCommit: "abc123"})
		case http.MethodGet:
			if strings.HasSuffix(r.URL.Path, "/default") {
				_ = json.NewEncoder(w).Encode(BranchResult{Name: "main", HeadCommit: "abc123"})
			} else {
				_ = json.NewEncoder(w).Encode(BranchListResult{Branches: []BranchListEntry{{Name: "main", HeadCommit: "abc123", Default: true}}})
			}
		case http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cfg := Config{
		Endpoint:    server.URL,
		TenantID:    "tenant-xyz",
		WorkspaceID: "ws-123",
		AuthMode:    AuthModeBearer,
	}
	client := NewClient()

	// 1. CreateBranch
	_, err := CreateBranch(context.Background(), client, cfg, "secret-token", BranchCreateRequest{Name: "feature", SourceBranch: "main"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if gotPath != "/api/v1/workspaces/ws-123/branches" {
		t.Fatalf("CreateBranch path = %q, want /api/v1/workspaces/ws-123/branches", gotPath)
	}
	if gotTenant != "tenant-xyz" {
		t.Fatalf("CreateBranch X-Tenant-ID = %q, want tenant-xyz", gotTenant)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("CreateBranch Authorization = %q, want Bearer secret-token", gotAuth)
	}

	// 2. ListBranches
	_, err = ListBranches(context.Background(), client, cfg, "secret-token")
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if gotPath != "/api/v1/workspaces/ws-123/branches" {
		t.Fatalf("ListBranches path = %q, want /api/v1/workspaces/ws-123/branches", gotPath)
	}
	if gotTenant != "tenant-xyz" {
		t.Fatalf("ListBranches X-Tenant-ID = %q, want tenant-xyz", gotTenant)
	}

	// 3. DefaultBranch
	_, err = DefaultBranch(context.Background(), client, cfg, "secret-token")
	if err != nil {
		t.Fatalf("DefaultBranch: %v", err)
	}
	if gotPath != "/api/v1/workspaces/ws-123/branches/default" {
		t.Fatalf("DefaultBranch path = %q, want /api/v1/workspaces/ws-123/branches/default", gotPath)
	}
	if gotTenant != "tenant-xyz" {
		t.Fatalf("DefaultBranch X-Tenant-ID = %q, want tenant-xyz", gotTenant)
	}

	// 4. DeleteBranch
	err = DeleteBranch(context.Background(), client, cfg, "secret-token", "feature")
	if err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	if gotPath != "/api/v1/workspaces/ws-123/branches/feature" {
		t.Fatalf("DeleteBranch path = %q, want /api/v1/workspaces/ws-123/branches/feature", gotPath)
	}
	if gotTenant != "tenant-xyz" {
		t.Fatalf("DeleteBranch X-Tenant-ID = %q, want tenant-xyz", gotTenant)
	}
}

