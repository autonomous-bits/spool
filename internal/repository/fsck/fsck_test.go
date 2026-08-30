package fsck

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
)

type fakeFsckStore struct {
	result Result
	err    error
	called bool
}

func (f *fakeFsckStore) Fsck() (Result, error) {
	f.called = true
	return f.result, f.err
}

func TestServiceCheckHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	service := NewService(t.TempDir())
	_, err := service.Check(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestServiceCheckAndCheckRepository(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "repo")
	repo, err := repository.InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	service := NewService(stateDir)
	result, err := service.Check(context.Background())
	if err != nil {
		t.Fatalf("service.Check failed: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected result.Valid to be true, got %#v", result)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(result.Diagnostics))
	}

	directResult, err := CheckRepository(stateDir)
	if err != nil {
		t.Fatalf("CheckRepository failed: %v", err)
	}
	if !directResult.Valid {
		t.Fatalf("expected directResult.Valid to be true, got %#v", directResult)
	}
}

func TestFsckSentinelAliases(t *testing.T) {
	if !errors.Is(ErrCorrupt, repository.ErrFsckCorrupt) {
		t.Fatalf("expected ErrCorrupt to match repository.ErrFsckCorrupt")
	}
}

func TestStoreInterface(t *testing.T) {
	expected := Result{
		Valid: true,
	}
	fake := &fakeFsckStore{result: expected}
	var store Store = fake

	res, err := store.Fsck()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fake.called {
		t.Fatal("expected Store.Fsck to be called")
	}
	if !res.Valid {
		t.Fatal("expected res.Valid to be true")
	}
}
