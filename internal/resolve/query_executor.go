package resolve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrResponseBudgetExceeded reports a response envelope exceeding its byte budget.
	ErrResponseBudgetExceeded = errors.New("query response exceeds byte budget")
	// ErrResponseEncoding reports an envelope that cannot be JSON encoded.
	ErrResponseEncoding = errors.New("query response cannot be JSON encoded")
)

// QueryExecutionMetadata describes the limits and timing established for a query.
// It is intended for embedding in future query response envelopes.
type QueryExecutionMetadata struct {
	// Budget is the normalized limit set enforced for the query.
	Budget QueryBudget `json:"budget"`
	// StartedAt is when execution began.
	StartedAt time.Time `json:"startedAt"`
	// Deadline is the earlier of the caller and budget deadlines.
	Deadline time.Time `json:"deadline"`
}

// QueryCompletionMetadata describes a completed query response. ResponseBytes is
// finalized against the complete JSON envelope, including this metadata.
type QueryCompletionMetadata struct {
	// Complete reports whether every matching result was returned.
	Complete bool `json:"complete"`
	// Truncated reports whether a query budget omitted matching results.
	Truncated bool `json:"truncated"`
	// TimedOut reports whether the effective query deadline ended a paged query.
	TimedOut bool `json:"timedOut"`
	// Visited reports the number of result entries examined for this response.
	Visited int `json:"visited"`
	// ElapsedMs is the non-negative elapsed execution duration in milliseconds.
	ElapsedMs int64 `json:"elapsedMs"`
	// CompletedAt is when query execution completed.
	CompletedAt time.Time `json:"-"`
	// Duration is the non-negative elapsed execution duration.
	Duration time.Duration `json:"-"`
	// ResponseBytes is the exact size of the JSON response envelope.
	ResponseBytes int `json:"responseBytes"`
}

// EffectiveQueryDeadline returns the earlier of the budget deadline calculated
// from now and the caller's existing deadline. Zero and negative budget timeouts
// deliberately produce an already-expired deadline.
func EffectiveQueryDeadline(ctx context.Context, budget QueryBudget, now time.Time) time.Time {
	deadline := now.Add(budget.Timeout)
	if callerDeadline, ok := ctx.Deadline(); ok && callerDeadline.Before(deadline) {
		return callerDeadline
	}
	return deadline
}

// EffectiveQueryContext derives a context bounded by both the caller's deadline
// and the effective query budget deadline. Call cancel when the query completes.
func EffectiveQueryContext(ctx context.Context, budget QueryBudget) (context.Context, context.CancelFunc) {
	return context.WithDeadline(ctx, EffectiveQueryDeadline(ctx, budget, time.Now()))
}

// BeginQuery establishes an effective query context and returns metadata for the
// execution. Call the returned cancel function when the query completes.
func BeginQuery(ctx context.Context, budget QueryBudget) (context.Context, QueryExecutionMetadata, context.CancelFunc) {
	startedAt := time.Now()
	deadline := EffectiveQueryDeadline(ctx, budget, startedAt)
	effective, cancel := context.WithDeadline(ctx, deadline)
	return effective, QueryExecutionMetadata{
		Budget:    budget,
		StartedAt: startedAt,
		Deadline:  deadline,
	}, cancel
}

// CompleteQuery returns completion metadata for an execution at completedAt.
func CompleteQuery(execution QueryExecutionMetadata, completedAt time.Time) QueryCompletionMetadata {
	duration := completedAt.Sub(execution.StartedAt)
	if duration < 0 {
		duration = 0
	}
	return QueryCompletionMetadata{
		CompletedAt: completedAt,
		Duration:    duration,
		ElapsedMs:   duration.Milliseconds(),
	}
}

// FinalizeQueryResponse JSON-encodes envelope, records its exact byte count in
// completion when supplied, and verifies that the completed envelope fits
// maxResponseBytes. It returns the verified bytes to avoid a later re-encoding
// changing the accounting.
func FinalizeQueryResponse(envelope any, completion *QueryCompletionMetadata, maxResponseBytes int) ([]byte, error) {
	for attempts := 0; attempts < 16; attempts++ {
		data, err := json.Marshal(envelope)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrResponseEncoding, err)
		}
		if completion == nil || completion.ResponseBytes == len(data) {
			if len(data) > maxResponseBytes {
				return nil, ErrResponseBudgetExceeded
			}
			return data, nil
		}
		completion.ResponseBytes = len(data)
	}
	return nil, fmt.Errorf("%w: response byte accounting did not converge", ErrResponseEncoding)
}
