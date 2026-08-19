// Package integrity exposes the durable repository integrity-check use case.
package integrity

import (
	"context"

	"github.com/autonomous-bits/spool/internal/repository"
)

// Service checks one durable repository state directory.
type Service struct {
	stateDir string
}

// NewService returns an integrity-check service for stateDir.
func NewService(stateDir string) Service {
	return Service{stateDir: stateDir}
}

// Check honors cancellation and returns the complete integrity report.
func (s Service) Check(ctx context.Context) (repository.FsckResult, error) {
	if err := ctx.Err(); err != nil {
		return repository.FsckResult{}, err
	}
	return repository.FsckRepository(s.stateDir)
}
