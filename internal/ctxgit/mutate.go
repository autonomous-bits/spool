package ctxgit

import (
	"context"
	"strings"

	"github.com/autonomous-bits/spool/internal/repository"
)

// MutateRequest is one bound node/edge mutation batch.
type MutateRequest struct {
	Operations []repository.MutationOperation
	Author     string
	Message    string
}

// MutateResult is the short-lived branch + PR from a bound graph mutation.
type MutateResult struct {
	WriteResult
	Operations int `json:"operations"`
}

// Mutate applies one mutation-operation batch to the bound context graph and
// opens a short-lived branch + PR via Stage then Commit. Bound-only: FindBind
// fail-closed. Empty batches are rejected. This is not schema migrate.
func (s *Session) Mutate(ctx context.Context, request MutateRequest) (MutateResult, error) {
	if s == nil || s.CodeRoot == "" {
		return MutateResult{}, UnboundError()
	}
	if _, _, _, err := FindBind(s.CodeRoot); err != nil {
		return MutateResult{}, err
	}
	if !s.Bound() {
		return MutateResult{}, UnboundError()
	}
	if err := ctx.Err(); err != nil {
		return MutateResult{}, err
	}
	staged, err := s.Stage(request.Operations)
	if err != nil {
		return MutateResult{}, err
	}
	message := strings.TrimSpace(request.Message)
	if message == "" {
		message = "Mutate context graph"
	}
	write, err := s.Commit(ctx, request.Author, message)
	if err != nil {
		return MutateResult{}, err
	}
	return MutateResult{WriteResult: write, Operations: staged.Operations}, nil
}
