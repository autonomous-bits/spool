package prune

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeStore struct {
	result Result
	err    error
	called bool
	req    Request
}

func (f *fakeStore) Prune(request Request) (Result, error) {
	f.called = true
	f.req = request
	return f.result, f.err
}

func TestServicePruneValidatesBranchRequired(t *testing.T) {
	service := NewService(&fakeStore{})
	_, err := service.Prune(context.Background(), Request{Branch: ""})
	if !errors.Is(err, ErrBranchRequired) {
		t.Fatalf("expected ErrBranchRequired, got %v", err)
	}
}

func TestServicePruneHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	store := &fakeStore{}
	service := NewService(store)
	_, err := service.Prune(ctx, Request{Branch: "feature"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if store.called {
		t.Fatal("expected store not to be called on canceled context")
	}
}

func TestServicePruneDelegatesToStore(t *testing.T) {
	expectedResult := Result{
		Branch:               "feature",
		Commit:               "commit-123",
		PrunedNodesCount:     2,
		PrunedEdgesCount:     3,
		PrunedNodeIDs:        []string{"node-1", "node-2"},
		OrphanedDurableNodes: []string{"node-3"},
	}
	store := &fakeStore{result: expectedResult}
	service := NewService(store)

	req := Request{
		Branch:  "feature",
		DryRun:  true,
		Force:   false,
		Author:  "alice",
		Message: "prune temp notes",
	}
	res, err := service.Prune(context.Background(), req)
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
