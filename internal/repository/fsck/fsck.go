// Package fsck exposes the durable repository integrity-check use case and service.
package fsck

import (
	"context"

	"github.com/autonomous-bits/spool/internal/repository"
)

// Result is the complete report produced by an integrity check.
type Result = repository.FsckResult

// Diagnostic describes one deterministic integrity or maintenance finding.
type Diagnostic = repository.FsckDiagnostic

// Error carries the structured report for a corrupt repository.
type Error = repository.FsckError

// ErrCorrupt reports that Fsck found one or more integrity violations.
var ErrCorrupt = repository.ErrFsckCorrupt

// Store provides the repository-level fsck operations.
type Store interface {
	Fsck() (Result, error)
}

// Service checks one durable repository state directory.
type Service struct {
	stateDir string
}

// NewService returns an integrity-check service for stateDir.
func NewService(stateDir string) Service {
	return Service{stateDir: stateDir}
}

// Check honors cancellation and returns the complete integrity report.
func (s Service) Check(ctx context.Context) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	return repository.FsckRepository(s.stateDir)
}

// CheckRepository traverses every durable branch reference and checks the
// reachable immutable objects and mutable control state without repairing it.
func CheckRepository(stateDir string) (Result, error) {
	return repository.FsckRepository(stateDir)
}
