package commands

import (
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
)

func newTestSeedRepository(t testing.TB) *repository.Repository {
	t.Helper()
	repo, err := repository.NewSeedRepository()
	if err != nil {
		t.Fatalf("NewSeedRepository: %v", err)
	}
	return repo
}
