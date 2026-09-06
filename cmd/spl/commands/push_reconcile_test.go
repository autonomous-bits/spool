package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/autonomous-bits/spool/internal/remote"
	"github.com/autonomous-bits/spool/internal/repository"
)

// TestPushReconcileRetriesAfterCleanMerge simulates the scenario the goal
// targets: Rack's branch has diverged (independent commits on both sides),
// so the first push attempt is rejected as non-fast-forward. With
// --reconcile, push should fetch Rack's full current history via a pull
// request, rebase the local branch's independent commit onto it, and retry
// the push, which this fake Rack accepts once the retried push's base
// matches Rack's real current head.
func TestPushReconcileRetriesAfterCleanMerge(t *testing.T) {
	local := newTestSeedRepository(t)
	rack := newTestSeedRepository(t)

	seedPack, err := local.BuildPushPack(context.Background(), "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack (seed): %v", err)
	}

	// Rack advances independently of the local repository.
	stageAndCommit(t, rack, "push-reconcile-rack-node", "Rack side", "rack-author", "rack advances")
	rackPack, err := rack.BuildPushPack(context.Background(), "main", seedPack.TargetCommit)
	if err != nil {
		t.Fatalf("BuildPushPack (rack): %v", err)
	}

	// The local repository also advances independently, on a disjoint node,
	// so the eventual merge is clean (no conflicting field writes).
	stageAndCommit(t, local, "push-reconcile-local-node", "Local side", "local-author", "local advances")

	pushAttempts := 0
	var correlationIDs []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationIDs = append(correlationIDs, r.Header.Get(remote.CorrelationIDHeader))
		switch r.Method {
		case http.MethodGet:
			if got := r.URL.Query().Get("knownCommit"); got != "" {
				t.Fatalf("pull knownCommit = %q, want empty (full history fetch)", got)
			}
			envelope := buildTestPullEnvelope(t, rackPack.TargetCommit, [][]byte{seedPack.PackData, rackPack.PackData})
			writeTestZstdPullResponse(t, w, envelope)
		default:
			pushAttempts++
			metadata := decodePushMetadata(t, r)
			w.Header().Set("Content-Type", "application/json")
			if metadata["baseCommit"] != rackPack.TargetCommit {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error":       "conflict",
					"message":     "branch has moved",
					"currentHead": rackPack.TargetCommit,
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{
				"branch":     "main",
				"headCommit": metadata["targetCommit"].(string),
			})
		}
	}))
	defer server.Close()

	if err := local.SetRemote(repository.RemoteConfig{Endpoint: server.URL, RepoID: "acme", AuthMode: repository.RemoteAuthModeBearer}); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	t.Setenv("SPOOL_RACK_TOKEN", "test-token")

	var output bytes.Buffer
	command := NewPushCommand(func() (*repository.Repository, error) { return local, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main", "--base-commit", seedPack.TargetCommit, "--reconcile"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute push: %v", err)
	}

	var result pushResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	if !result.Pushed || !result.Reconciled || result.Rejected {
		t.Fatalf("result = %#v, want Pushed=true Reconciled=true", result)
	}
	if pushAttempts != 2 {
		t.Fatalf("pushAttempts = %d, want 2 (initial rejection + reconciled retry)", pushAttempts)
	}

	tracking, ok, err := local.RemoteBranchTracking("main")
	if err != nil {
		t.Fatalf("RemoteBranchTracking: %v", err)
	}
	if !ok || tracking.RemoteHeadCommit != result.HeadCommit {
		t.Fatalf("tracking = %#v, want remoteHeadCommit=%s", tracking, result.HeadCommit)
	}

	// The initial push, the reconciliation pull, and the retried push
	// should all share one remote.Client (and therefore one
	// CorrelationID), so Rack's audit log can correlate them as a single
	// `spl push --reconcile` invocation.
	if len(correlationIDs) != 3 {
		t.Fatalf("saw %d remote requests, want 3 (push, pull, retried push)", len(correlationIDs))
	}
	if correlationIDs[0] == "" {
		t.Fatal("first remote request sent an empty CorrelationID")
	}
	for i, id := range correlationIDs {
		if id != correlationIDs[0] {
			t.Fatalf("correlationIDs[%d] = %q, want %q (all requests in one invocation should share a CorrelationID)", i, id, correlationIDs[0])
		}
	}

	reconciliationBranch := repository.ReconciliationBranchName("main")
	branches, err := local.ListBranches()
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	found := false
	for _, name := range branches.Branches {
		if name == reconciliationBranch {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("branches = %#v, want reconciliation branch %q to exist after reconciliation", branches.Branches, reconciliationBranch)
	}
}

// TestPushWithoutReconcileFlagReportsRejectionWithoutFetching proves push
// does not attempt any reconciliation (and never issues a pull) unless
// --reconcile is set, matching TestPushNonFastForwardReportsRejection's
// existing plain-rejection behavior.
func TestPushWithoutReconcileFlagReportsRejectionWithoutFetching(t *testing.T) {
	local := newTestSeedRepository(t)
	stageAndCommit(t, local, "push-reconcile-local-node", "Local side", "local-author", "local advances")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			t.Fatal("push without --reconcile issued a pull request")
		}
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

	if err := local.SetRemote(repository.RemoteConfig{Endpoint: server.URL, RepoID: "acme", AuthMode: repository.RemoteAuthModeBearer}); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	t.Setenv("SPOOL_RACK_TOKEN", "test-token")

	var output bytes.Buffer
	command := NewPushCommand(func() (*repository.Repository, error) { return local, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute push: %v", err)
	}

	var result pushResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	if result.Pushed || !result.Rejected || result.Reconciled {
		t.Fatalf("result = %#v, want Pushed=false Rejected=true Reconciled=false", result)
	}
}

// TestPushLocalBuildErrorIsNotWrappedInRemoteEnvelope proves a local
// failure (BuildPushPack, before any remote call is made - here, a
// nonexistent branch) is returned as a plain error rather than routed
// through the remote JSON error envelope, which is reserved for failures
// that actually came back from Rack.
func TestPushLocalBuildErrorIsNotWrappedInRemoteEnvelope(t *testing.T) {
	local := newTestSeedRepository(t)

	contacted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contacted = true
	}))
	defer server.Close()

	if err := local.SetRemote(repository.RemoteConfig{Endpoint: server.URL, RepoID: "acme", AuthMode: repository.RemoteAuthModeBearer}); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	t.Setenv("SPOOL_RACK_TOKEN", "test-token")

	var output bytes.Buffer
	command := NewPushCommand(func() (*repository.Repository, error) { return local, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "does-not-exist"})
	err := command.Execute()
	if err == nil {
		t.Fatal("execute push: want an error for a nonexistent branch, got nil")
	}
	if !errors.Is(err, repository.ErrBranchNotFound) {
		t.Fatalf("err = %v, want it to wrap repository.ErrBranchNotFound", err)
	}
	if contacted {
		t.Fatal("push contacted the remote for a local BuildPushPack failure")
	}
	if output.Len() != 0 {
		t.Fatalf("output = %q, want no remote error envelope written for a local failure", output.String())
	}
}
