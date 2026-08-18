// Package initialization exposes the repository initialization use case.
package initialization

import "github.com/autonomous-bits/spool/internal/repository"

// Initialize creates and durably stores a seeded repository.
func Initialize(stateDir string) (*repository.Repository, error) {
	return repository.InitializeRepository(stateDir)
}
