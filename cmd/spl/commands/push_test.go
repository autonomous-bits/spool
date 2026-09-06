package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
)

func stageAndCommit(t *testing.T, repo *repository.Repository, id, title, author, message string) {
	t.Helper()
	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{
		Branch: "main",
		Operations: []repository.MutationOperation{
			{Action: "add", Entity: "node", ID: id, Title: title, Labels: []string{"Fixture"}},
		},
	}); err != nil {
		t.Fatalf("stage %s: %v", id, err)
	}
	if _, err := repo.CommitStagedMutationBatch(repository.CommitStagedMutationRequest{
		Branch: "main", Author: author, Message: message,
	}); err != nil {
		t.Fatalf("commit %s: %v", id, err)
	}
}

func decodePushMetadata(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		t.Fatalf("content type = %q, err = %v", r.Header.Get("Content-Type"), err)
	}
	reader := multipart.NewReader(r.Body, params["boundary"])
	var metadata map[string]any
	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}
		if part.FormName() == "metadata" {
			if err := json.NewDecoder(part).Decode(&metadata); err != nil {
				t.Fatalf("decode metadata: %v", err)
			}
		}
		_ = part.Close()
	}
	if metadata == nil {
		t.Fatal("request did not include a metadata part")
	}
	return metadata
}

func TestPushSendsPackAndReportsSuccess(t *testing.T) {
	repo := newTestSeedRepository(t)
	stageAndCommit(t, repo, "push-cli-node-1", "First", "alice", "first commit")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metadata := decodePushMetadata(t, r)
		if metadata["branch"] != "main" {
			t.Fatalf("branch = %v", metadata["branch"])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"branch":     "main",
			"headCommit": metadata["targetCommit"].(string),
		})
	}))
	defer server.Close()

	if err := repo.SetRemote(repository.RemoteConfig{Endpoint: server.URL, RepoID: "acme", AuthMode: repository.RemoteAuthModeBearer}); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	t.Setenv("SPOOL_RACK_TOKEN", "test-token")

	var output bytes.Buffer
	command := NewPushCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute push: %v", err)
	}

	var result pushResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	if !result.Pushed || result.Branch != "main" || result.HeadCommit == "" {
		t.Fatalf("result = %#v", result)
	}
}

func TestPushWithoutConfiguredRemoteFails(t *testing.T) {
	repo := newTestSeedRepository(t)
	stageAndCommit(t, repo, "push-cli-node-1", "First", "alice", "first commit")

	var output bytes.Buffer
	command := NewPushCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main"})
	if err := command.Execute(); err != repository.ErrRemoteNotConfigured {
		t.Fatalf("err = %v, want ErrRemoteNotConfigured", err)
	}
}

func TestPushNothingToPushReportsCleanlyWithoutContactingRemote(t *testing.T) {
	repo := newTestSeedRepository(t)
	stageAndCommit(t, repo, "push-cli-node-1", "First", "alice", "first commit")

	contacted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contacted = true
	}))
	defer server.Close()
	if err := repo.SetRemote(repository.RemoteConfig{Endpoint: server.URL, RepoID: "acme", AuthMode: repository.RemoteAuthModeBearer}); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	t.Setenv("SPOOL_RACK_TOKEN", "test-token")

	pack, err := repo.BuildPushPack(context.Background(), "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack: %v", err)
	}

	var output bytes.Buffer
	command := NewPushCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main", "--base-commit", pack.TargetCommit})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute push: %v", err)
	}

	var result pushResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	if result.Pushed {
		t.Fatalf("result = %#v, want Pushed=false", result)
	}
	if contacted {
		t.Fatal("push contacted the remote when there was nothing to push")
	}
}

func TestPushUnauthorizedEmitsUnifiedErrorEnvelope(t *testing.T) {
	repo := newTestSeedRepository(t)
	stageAndCommit(t, repo, "push-cli-node-1", "First", "alice", "first commit")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodePushMetadata(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":         "unauthorized",
			"message":       "credential was rejected",
			"correlationId": "corr-unauthorized",
		})
	}))
	defer server.Close()
	if err := repo.SetRemote(repository.RemoteConfig{Endpoint: server.URL, RepoID: "acme", AuthMode: repository.RemoteAuthModeBearer}); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	t.Setenv("SPOOL_RACK_TOKEN", "test-token")

	var output bytes.Buffer
	command := NewPushCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main"})
	if err := command.Execute(); err == nil {
		t.Fatal("execute push: expected error")
	}

	var envelope remoteErrorEnvelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Error != "unauthorized" || envelope.Message != "credential was rejected" || envelope.CorrelationID != "corr-unauthorized" {
		t.Fatalf("envelope = %#v", envelope)
	}
}

func TestPushBadRequestEmitsUnifiedErrorEnvelope(t *testing.T) {
	repo := newTestSeedRepository(t)
	stageAndCommit(t, repo, "push-cli-node-1", "First", "alice", "first commit")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodePushMetadata(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":         "bad_request",
			"message":       "packHash must be 64 lowercase hex characters",
			"correlationId": "corr-bad-request",
		})
	}))
	defer server.Close()
	if err := repo.SetRemote(repository.RemoteConfig{Endpoint: server.URL, RepoID: "acme", AuthMode: repository.RemoteAuthModeBearer}); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	t.Setenv("SPOOL_RACK_TOKEN", "test-token")

	var output bytes.Buffer
	command := NewPushCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main"})
	if err := command.Execute(); err == nil {
		t.Fatal("execute push: expected error")
	}

	var envelope remoteErrorEnvelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Error != "bad_request" || envelope.CorrelationID != "corr-bad-request" {
		t.Fatalf("envelope = %#v", envelope)
	}
}

func TestPushInternalErrorEmitsUnifiedErrorEnvelope(t *testing.T) {
	repo := newTestSeedRepository(t)
	stageAndCommit(t, repo, "push-cli-node-1", "First", "alice", "first commit")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodePushMetadata(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":         "internal_error",
			"message":       "unexpected failure",
			"correlationId": "corr-internal",
		})
	}))
	defer server.Close()
	if err := repo.SetRemote(repository.RemoteConfig{Endpoint: server.URL, RepoID: "acme", AuthMode: repository.RemoteAuthModeBearer}); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	t.Setenv("SPOOL_RACK_TOKEN", "test-token")

	var output bytes.Buffer
	command := NewPushCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main"})
	if err := command.Execute(); err == nil {
		t.Fatal("execute push: expected error")
	}

	var envelope remoteErrorEnvelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Error != "internal_error" || envelope.CorrelationID != "corr-internal" {
		t.Fatalf("envelope = %#v", envelope)
	}
}

func TestPushUnreachableRemoteEmitsUnifiedErrorEnvelope(t *testing.T) {
	repo := newTestSeedRepository(t)
	stageAndCommit(t, repo, "push-cli-node-1", "First", "alice", "first commit")

	if err := repo.SetRemote(repository.RemoteConfig{Endpoint: "http://127.0.0.1:1", RepoID: "acme", AuthMode: repository.RemoteAuthModeBearer}); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	t.Setenv("SPOOL_RACK_TOKEN", "test-token")

	var output bytes.Buffer
	command := NewPushCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main"})
	if err := command.Execute(); err == nil {
		t.Fatal("execute push: expected error")
	}

	var envelope remoteErrorEnvelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Error != "remote_unreachable" {
		t.Fatalf("envelope = %#v", envelope)
	}
	if strings.Contains(envelope.Message, "test-token") {
		t.Fatalf("envelope leaked credential: %#v", envelope)
	}
}

func TestPushSuccessUpdatesRemoteBranchTracking(t *testing.T) {
	repo := newTestSeedRepository(t)
	stageAndCommit(t, repo, "push-cli-node-1", "First", "alice", "first commit")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metadata := decodePushMetadata(t, r)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"branch":     "main",
			"headCommit": metadata["targetCommit"].(string),
		})
	}))
	defer server.Close()

	if err := repo.SetRemote(repository.RemoteConfig{Endpoint: server.URL, RepoID: "acme", AuthMode: repository.RemoteAuthModeBearer}); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	t.Setenv("SPOOL_RACK_TOKEN", "test-token")

	var output bytes.Buffer
	command := NewPushCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute push: %v", err)
	}

	var result pushResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	tracking, ok, err := repo.RemoteBranchTracking("main")
	if err != nil {
		t.Fatalf("RemoteBranchTracking: %v", err)
	}
	if !ok {
		t.Fatal("RemoteBranchTracking ok = false, want true after successful push")
	}
	if tracking.RemoteBranch != "main" || tracking.RemoteHeadCommit != result.HeadCommit {
		t.Fatalf("tracking = %#v, want remoteBranch=main remoteHeadCommit=%s", tracking, result.HeadCommit)
	}
}

func TestPushNonFastForwardReportsRejection(t *testing.T) {
	repo := newTestSeedRepository(t)
	stageAndCommit(t, repo, "push-cli-node-1", "First", "alice", "first commit")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodePushMetadata(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":       "conflict",
			"message":     "branch has moved",
			"currentHead": "some-other-head",
		})
	}))
	defer server.Close()
	if err := repo.SetRemote(repository.RemoteConfig{Endpoint: server.URL, RepoID: "acme", AuthMode: repository.RemoteAuthModeBearer}); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	t.Setenv("SPOOL_RACK_TOKEN", "test-token")

	var output bytes.Buffer
	command := NewPushCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute push: %v", err)
	}

	var result pushResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	if result.Pushed || !result.Rejected || result.ActualHead != "some-other-head" {
		t.Fatalf("result = %#v", result)
	}
}
