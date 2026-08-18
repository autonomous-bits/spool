package initialization

import (
	"errors"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
)

func closeTestRepository(t *testing.T, repo *repository.Repository) {
	t.Helper()
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Errorf("Close repository: %v", err)
		}
	})
}

func TestInitializeCreatesRepositoryWithActiveMainBranch(t *testing.T) {
	repo, err := Initialize(t.TempDir())
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	closeTestRepository(t, repo)

	result, err := repo.Initialization()
	if err != nil {
		t.Fatalf("Initialization: %v", err)
	}
	if result != (repository.Initialization{DefaultBranch: "main", ActiveBranch: "main"}) {
		t.Fatalf("initialization = %#v", result)
	}
}

func TestInitializeRejectsExistingRepository(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := Initialize(stateDir)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := Initialize(stateDir); !errors.Is(err, repository.ErrRepositoryAlreadyInitialized) {
		t.Fatalf("Initialize error = %v, want ErrRepositoryAlreadyInitialized", err)
	}
}
