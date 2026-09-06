package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/graphcontract"
	"github.com/autonomous-bits/spool/internal/repository"
)

func TestRemoteSetPersistsConfigurationAndPrintsNoCredential(t *testing.T) {
	repo := newTestSeedRepository(t)
	var output bytes.Buffer

	command := NewRemoteCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"set", "--endpoint", "https://rack.example.com", "--repo-id", "acme-prod", "--auth-mode", "bearer"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute remote set: %v", err)
	}

	var result remoteConfigResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	if result.Endpoint != "https://rack.example.com" || result.RepoID != "acme-prod" || result.AuthMode != "bearer" {
		t.Fatalf("result = %#v", result)
	}
	cfg, ok, err := repo.Remote()
	if err != nil || !ok {
		t.Fatalf("Remote() = %#v, %v, %v", cfg, ok, err)
	}
}

func TestRemoteSetRejectsMissingRequiredFlags(t *testing.T) {
	repo := newTestSeedRepository(t)
	var output bytes.Buffer

	command := NewRemoteCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"set", "--endpoint", "https://rack.example.com"})
	if err := command.Execute(); err == nil {
		t.Fatal("execute remote set = nil error, want error for missing required flags")
	}
}

func TestRemoteSetRejectsSecretLikeRepoIDAndDoesNotPersist(t *testing.T) {
	repo := newTestSeedRepository(t)
	var output bytes.Buffer

	command := NewRemoteCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"set", "--endpoint", "https://rack.example.com", "--repo-id", "ghp_1234567890abcdefghijklmnopqrstuvwxyz", "--auth-mode", "bearer"})
	if err := command.Execute(); !errors.Is(err, repository.ErrRemoteSecretLikeValue) {
		t.Fatalf("execute remote set error = %v, want ErrRemoteSecretLikeValue", err)
	}
	if output.Len() != 0 {
		t.Fatalf("remote set wrote output despite rejection: %q", output.String())
	}
	if _, ok, err := repo.Remote(); err != nil || ok {
		t.Fatalf("Remote() after rejected set = ok=%v, err=%v, want ok=false", ok, err)
	}
}

func TestRemoteShowReportsNotConfiguredError(t *testing.T) {
	repo := newTestSeedRepository(t)
	var output bytes.Buffer

	command := NewRemoteCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"show"})
	if err := command.Execute(); !errors.Is(err, repository.ErrRemoteNotConfigured) {
		t.Fatalf("execute remote show error = %v, want ErrRemoteNotConfigured", err)
	}
}

func TestRemoteShowNegotiatesVersionsAndOmitsCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]uint32{
			"packFormatVersion":         graphcontract.PackFormatVersion,
			"packIndexFormatVersion":    graphcontract.PackIndexFormatVersion,
			"packManifestFormatVersion": graphcontract.PackManifestFormatVersion,
		})
	}))
	defer server.Close()

	repo := newTestSeedRepository(t)
	if err := repo.SetRemote(repository.RemoteConfig{Endpoint: server.URL, RepoID: "acme-prod", AuthMode: repository.RemoteAuthModeBearer}); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	var output bytes.Buffer

	command := NewRemoteCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"show"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute remote show: %v", err)
	}

	var result remoteShowResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	if result.VersionStatus != "negotiated" || result.Versions == nil {
		t.Fatalf("result = %#v", result)
	}
	if strings.Contains(output.String(), "SPOOL_RACK_TOKEN") {
		t.Fatalf("output unexpectedly references credential env var: %q", output.String())
	}
}

func TestRemoteShowReportsUnreachableWithoutCrashing(t *testing.T) {
	repo := newTestSeedRepository(t)
	if err := repo.SetRemote(repository.RemoteConfig{Endpoint: "http://127.0.0.1:1", RepoID: "acme-prod", AuthMode: repository.RemoteAuthModeBearer}); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	var output bytes.Buffer

	command := NewRemoteCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"show"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute remote show: %v", err)
	}

	var result remoteShowResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	if result.VersionStatus != "unreachable" || result.Versions != nil {
		t.Fatalf("result = %#v", result)
	}
}

func TestRemoteRemoveReportsWhetherRemoteExisted(t *testing.T) {
	repo := newTestSeedRepository(t)
	if err := repo.SetRemote(repository.RemoteConfig{Endpoint: "https://rack.example.com", RepoID: "acme-prod", AuthMode: repository.RemoteAuthModeBearer}); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	var output bytes.Buffer

	command := NewRemoteCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"remove"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute remote remove: %v", err)
	}
	var result struct {
		Removed bool `json:"removed"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	if !result.Removed {
		t.Fatalf("result = %#v, want removed=true", result)
	}
	if _, ok, err := repo.Remote(); err != nil || ok {
		t.Fatalf("Remote() after remove = ok=%v, err=%v, want ok=false", ok, err)
	}
}

func TestRemoteRemoveIsIdempotent(t *testing.T) {
	repo := newTestSeedRepository(t)
	var output bytes.Buffer

	command := NewRemoteCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"remove"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute remote remove: %v", err)
	}
	var result struct {
		Removed bool `json:"removed"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	if result.Removed {
		t.Fatalf("result = %#v, want removed=false", result)
	}
}
