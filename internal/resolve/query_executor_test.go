package resolve

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestEffectiveQueryDeadlineUsesEarlierCallerOrBudgetDeadline(t *testing.T) {
	now := time.Date(2026, time.August, 20, 10, 0, 0, 0, time.UTC)
	callerDeadline := now.Add(time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), callerDeadline)
	t.Cleanup(cancel)

	if got := EffectiveQueryDeadline(ctx, QueryBudget{Timeout: 2 * time.Second}, now); !got.Equal(callerDeadline) {
		t.Fatalf("caller deadline = %s, want %s", got, callerDeadline)
	}
	budgetDeadline := now.Add(100 * time.Millisecond)
	if got := EffectiveQueryDeadline(ctx, QueryBudget{Timeout: 100 * time.Millisecond}, now); !got.Equal(budgetDeadline) {
		t.Fatalf("budget deadline = %s, want %s", got, budgetDeadline)
	}
	if got := EffectiveQueryDeadline(context.Background(), QueryBudget{}, now); !got.Equal(now) {
		t.Fatalf("zero timeout deadline = %s, want %s", got, now)
	}
}

func TestEffectiveQueryContextPreservesCallerCancellation(t *testing.T) {
	parent, parentCancel := context.WithCancel(context.Background())
	ctx, cancel := EffectiveQueryContext(parent, QueryBudget{Timeout: time.Hour})
	t.Cleanup(cancel)
	parentCancel()

	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatalf("context error = %v, want context.Canceled", ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("query context did not preserve caller cancellation")
	}
}

func TestBeginAndCompleteQueryMetadata(t *testing.T) {
	ctx, execution, cancel := BeginQuery(context.Background(), QueryBudget{Timeout: time.Second})
	t.Cleanup(cancel)
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("query context has no deadline")
	}
	if execution.Budget.Timeout != time.Second || !execution.Deadline.Equal(deadline) {
		t.Fatalf("execution metadata = %#v, context deadline = %s", execution, deadline)
	}

	completedAt := execution.StartedAt.Add(25 * time.Millisecond)
	completion := CompleteQuery(execution, completedAt)
	if completion.CompletedAt != completedAt || completion.Duration != 25*time.Millisecond || completion.ElapsedMs != 25 {
		t.Fatalf("completion metadata = %#v", completion)
	}
	if got := CompleteQuery(execution, execution.StartedAt.Add(-time.Second)).Duration; got != 0 {
		t.Fatalf("negative duration = %s, want 0", got)
	}
}

func TestFinalizeQueryResponseAccountsForCompleteEnvelope(t *testing.T) {
	completion := &QueryCompletionMetadata{
		Complete: true, Visited: 2,
	}
	envelope := struct {
		Payload    string                   `json:"payload"`
		Completion *QueryCompletionMetadata `json:"completion"`
	}{
		Payload:    "response",
		Completion: completion,
	}

	data, err := FinalizeQueryResponse(envelope, completion, 1<<20)
	if err != nil {
		t.Fatalf("FinalizeQueryResponse: %v", err)
	}
	if completion.ResponseBytes != len(data) {
		t.Fatalf("response bytes = %d, want %d", completion.ResponseBytes, len(data))
	}
	reencoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("Marshal finalized envelope: %v", err)
	}
	if string(data) != string(reencoded) {
		t.Fatalf("finalized bytes = %s, re-encoded bytes = %s", data, reencoded)
	}
	var encoded struct {
		Completion map[string]json.RawMessage `json:"completion"`
	}
	if err := json.Unmarshal(data, &encoded); err != nil {
		t.Fatalf("Unmarshal finalized envelope: %v", err)
	}
	for _, field := range []string{"complete", "truncated", "timedOut", "visited", "elapsedMs", "responseBytes"} {
		if _, ok := encoded.Completion[field]; !ok {
			t.Errorf("completion omitted %q: %s", field, data)
		}
	}

	_, err = FinalizeQueryResponse(envelope, completion, len(data)-1)
	if !errors.Is(err, ErrResponseBudgetExceeded) {
		t.Fatalf("over-budget error = %v, want ErrResponseBudgetExceeded", err)
	}
}

func TestFinalizeQueryResponseReportsEncodingFailures(t *testing.T) {
	_, err := FinalizeQueryResponse(make(chan int), nil, 1<<20)
	if !errors.Is(err, ErrResponseEncoding) {
		t.Fatalf("encoding error = %v, want ErrResponseEncoding", err)
	}
}
