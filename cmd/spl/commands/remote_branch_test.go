package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
)

func setTestRemote(t *testing.T, repo *repository.Repository, endpoint string) {
	t.Helper()
	if err := repo.SetRemote(repository.RemoteConfig{Endpoint: endpoint, RepoID: "acme", AuthMode: repository.RemoteAuthModeBearer}); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	t.Setenv("SPOOL_RACK_TOKEN", "test-token")
}

func TestRemoteBranchCreateReportsResultAndUpdatesTracking(t *testing.T) {
	repo := newTestSeedRepository(t)
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"name": "feature", "headCommit": "abc123"})
	}))
	defer server.Close()
	setTestRemote(t, repo, server.URL)

	var output bytes.Buffer
	command := newRemoteBranchCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"create", "feature", "--from-branch", "main"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var result remoteBranchResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.Name != "feature" || result.HeadCommit != "abc123" {
		t.Fatalf("result = %#v", result)
	}
	if gotBody["name"] != "feature" || gotBody["sourceBranch"] != "main" {
		t.Fatalf("request body = %#v", gotBody)
	}

	tracking, ok, err := repo.RemoteBranchTracking("feature")
	if err != nil || !ok {
		t.Fatalf("RemoteBranchTracking: ok=%v, err=%v", ok, err)
	}
	if tracking.RemoteBranch != "feature" || tracking.RemoteHeadCommit != "abc123" {
		t.Fatalf("tracking = %#v", tracking)
	}
}

func TestRemoteBranchCreateAlreadyExistsEmitsErrorEnvelope(t *testing.T) {
	repo := newTestSeedRepository(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "branch_already_exists", "message": "branch already exists", "correlationId": "corr-1",
		})
	}))
	defer server.Close()
	setTestRemote(t, repo, server.URL)

	var output bytes.Buffer
	command := newRemoteBranchCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"create", "feature", "--from-branch", "main"})
	if err := command.Execute(); err == nil {
		t.Fatal("expected error")
	}

	var envelope remoteErrorEnvelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if envelope.Error != "branch_already_exists" || envelope.CorrelationID != "corr-1" {
		t.Fatalf("envelope = %#v", envelope)
	}
	if _, ok, _ := repo.RemoteBranchTracking("feature"); ok {
		t.Fatal("tracking should not be recorded after a failed create")
	}
}

func TestRemoteBranchCreateWithoutConfiguredRemoteFails(t *testing.T) {
	repo := newTestSeedRepository(t)
	var output bytes.Buffer
	command := newRemoteBranchCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"create", "feature", "--from-branch", "main"})
	if err := command.Execute(); err != repository.ErrRemoteNotConfigured {
		t.Fatalf("err = %v, want ErrRemoteNotConfigured", err)
	}
}

func TestRemoteBranchListReportsBranches(t *testing.T) {
	repo := newTestSeedRepository(t)
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"branches": []map[string]any{
				{"name": "main", "headCommit": "abc123", "default": true},
				{"name": "feature", "headCommit": "def456"},
			},
		})
	}))
	defer server.Close()
	setTestRemote(t, repo, server.URL)

	var output bytes.Buffer
	command := newRemoteBranchCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"list"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var result remoteBranchListResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(result.Branches) != 2 || result.Branches[0].Name != "main" || !result.Branches[0].Default {
		t.Fatalf("result = %#v", result)
	}
	if gotPath != "/api/v1/repos/acme/branches" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestRemoteBranchDefaultReportsResult(t *testing.T) {
	repo := newTestSeedRepository(t)
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"name": "main", "headCommit": "abc123"})
	}))
	defer server.Close()
	setTestRemote(t, repo, server.URL)

	var output bytes.Buffer
	command := newRemoteBranchCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"default"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var result remoteBranchResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.Name != "main" || result.HeadCommit != "abc123" {
		t.Fatalf("result = %#v", result)
	}
	if gotPath != "/api/v1/repos/acme/branches/default" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestRemoteBranchDeleteReportsSuccess(t *testing.T) {
	repo := newTestSeedRepository(t)
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	setTestRemote(t, repo, server.URL)

	var output bytes.Buffer
	command := newRemoteBranchCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"delete", "feature"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var result remoteBranchDeleteResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !result.Deleted || result.Name != "feature" {
		t.Fatalf("result = %#v", result)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/repos/acme/branches/feature" {
		t.Fatalf("method = %q, path = %q", gotMethod, gotPath)
	}
}

func TestRemoteBranchDeleteProtectedEmitsErrorEnvelope(t *testing.T) {
	repo := newTestSeedRepository(t)
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
	setTestRemote(t, repo, server.URL)

	var output bytes.Buffer
	command := newRemoteBranchCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"delete", "main"})
	if err := command.Execute(); err == nil {
		t.Fatal("expected error deleting protected branch")
	}

	var envelope remoteErrorEnvelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if envelope.Error != "branch_protected" || envelope.Message != "default branch cannot be deleted" || envelope.CorrelationID != "corr-protected" {
		t.Fatalf("envelope = %#v", envelope)
	}
}

func TestRemoteBranchDeleteNotFoundEmitsErrorEnvelope(t *testing.T) {
	repo := newTestSeedRepository(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "branch_not_found", "message": "no such branch"})
	}))
	defer server.Close()
	setTestRemote(t, repo, server.URL)

	var output bytes.Buffer
	command := newRemoteBranchCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"delete", "missing"})
	if err := command.Execute(); err == nil {
		t.Fatal("expected error")
	}

	var envelope remoteErrorEnvelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if envelope.Error != "branch_not_found" {
		t.Fatalf("envelope = %#v", envelope)
	}
}

func TestRemoteBranchDeleteRequiresExactlyOneArg(t *testing.T) {
	repo := newTestSeedRepository(t)
	var output bytes.Buffer
	command := newRemoteBranchCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"delete"})
	if err := command.Execute(); err == nil {
		t.Fatal("expected argument count error")
	}
}
