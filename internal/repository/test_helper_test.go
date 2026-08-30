package repository

import "testing"

func newTestSeedRepository(t testing.TB) *Repository {
	t.Helper()
	repo, err := NewSeedRepository()
	if err != nil {
		t.Fatalf("NewSeedRepository: %v", err)
	}
	return repo
}
