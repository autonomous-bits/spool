package cherrypick

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository/branch"
)

type fakeStore struct {
	result Result
	err    error
	called bool
	req    Request
}

func (f *fakeStore) CherryPick(request Request) (Result, error) {
	f.called = true
	f.req = request
	return f.result, f.err
}

func TestServiceCherryPickValidatesRequiredFields(t *testing.T) {
	service := NewService(&fakeStore{})

	_, err := service.CherryPick(context.Background(), Request{
		Commit:       "",
		TargetBranch: "main",
	})
	if !errors.Is(err, ErrCommitRequired) {
		t.Fatalf("expected ErrCommitRequired, got %v", err)
	}

	_, err = service.CherryPick(context.Background(), Request{
		Commit:       "commit-123",
		TargetBranch: "",
	})
	if !errors.Is(err, ErrTargetBranchRequired) {
		t.Fatalf("expected ErrTargetBranchRequired, got %v", err)
	}
}

func TestServiceCherryPickHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	store := &fakeStore{}
	service := NewService(store)
	_, err := service.CherryPick(ctx, Request{
		Commit:       "commit-123",
		TargetBranch: "main",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if store.called {
		t.Fatal("expected store not to be called on canceled context")
	}
}

func TestServiceCherryPickDelegatesToStore(t *testing.T) {
	expectedResult := Result{
		TargetBranch: "main",
		SourceCommit: "commit-123",
		Commit:       "commit-456",
		DryRun:       false,
		Changes: []Change{
			{Entity: "node", ID: "node-1", Change: "added"},
		},
		Conflicts:  []Conflict{},
		Violations: []SchemaViolation{},
	}
	store := &fakeStore{result: expectedResult}
	service := NewService(store)

	req := Request{
		Commit:       "commit-123",
		TargetBranch: "main",
		DryRun:       false,
		Author:       "alice",
		Message:      "cherry pick feature",
	}
	res, err := service.CherryPick(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !store.called {
		t.Fatal("expected store to be called")
	}
	if !reflect.DeepEqual(store.req, req) {
		t.Fatalf("store received req %+v, want %+v", store.req, req)
	}
	if !reflect.DeepEqual(res, expectedResult) {
		t.Fatalf("result = %+v, want %+v", res, expectedResult)
	}
}

func TestCherryPickSentinelErrors(t *testing.T) {
	for _, sentinel := range []error{
		ErrCommitRequired,
		ErrTargetBranchRequired,
		ErrConflicts,
	} {
		if sentinel == nil {
			t.Fatal("expected non-nil sentinel error")
		}
		wrapped := fmt.Errorf("operation failed: %w", sentinel)
		if !errors.Is(wrapped, sentinel) {
			t.Fatalf("expected errors.Is to match wrapped error for %v", sentinel)
		}
	}
	if !errors.Is(ErrTargetBranchRequired, branch.ErrRequired) {
		t.Fatalf("expected ErrTargetBranchRequired to match branch.ErrRequired")
	}
}
