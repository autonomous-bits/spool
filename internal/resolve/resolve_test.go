package resolve

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/autonomous-bits/spool/internal/repository"
)

func TestResolvePinsBranchCommitForRequest(t *testing.T) {
	repo := newTestSeedRepository(t)
	resolver := NewResolver(repo)

	initialCommit, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}

	resolver.afterBranchResolved = func() {
		if _, err := repo.AdvanceBranch("main"); err != nil {
			t.Fatalf("AdvanceBranch: %v", err)
		}
	}

	got, err := resolver.Resolve(context.Background(), SnapshotSelector{Branch: "main"}, repository.SeedNodeID)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Snapshot.Commit != string(initialCommit) {
		t.Fatalf("pinned commit = %q, want %q", got.Snapshot.Commit, initialCommit)
	}

	currentCommit, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch after advance: %v", err)
	}
	if currentCommit == initialCommit {
		t.Fatal("branch did not advance during resolution")
	}
}

func TestResolverRejectsCancellationAfterPinning(t *testing.T) {
	repo := newTestSeedRepository(t)
	resolver := NewResolver(repo)
	ctx, cancel := context.WithCancel(context.Background())
	resolver.afterBranchResolved = cancel

	_, err := resolver.Resolve(ctx, SnapshotSelector{Branch: "main"}, repository.SeedNodeID)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Resolve error = %v, want context.Canceled", err)
	}
}

func TestResolveToolRejectsCanceledQuery(t *testing.T) {
	repo := newTestSeedRepository(t)
	tool := NewResolveTool(repo)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := tool.SPLResolve(ctx, ResolveRequest{
		Selector: SnapshotSelector{Branch: "main"},
		NodeID:   repository.SeedNodeID,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SPLResolve error = %v, want context.Canceled", err)
	}
}

func TestSPLResolveReturnsSnapshotAndProjectionMetadata(t *testing.T) {
	tool := NewResolveTool(newTestSeedRepository(t))

	got, err := tool.SPLResolve(context.Background(), ResolveRequest{
		Selector: SnapshotSelector{Branch: "main"},
		NodeID:   repository.SeedNodeID,
	})
	if err != nil {
		t.Fatalf("SPLResolve: %v", err)
	}

	if got.Node.ID != repository.SeedNodeID {
		t.Fatalf("node ID = %q, want %q", got.Node.ID, repository.SeedNodeID)
	}
	if got.Snapshot.Repository == "" || got.Snapshot.Branch != "main" || got.Snapshot.Commit == "" || got.Snapshot.Root == "" || got.Budget != DefaultQueryBudget() {
		t.Fatalf("missing resolution metadata: %#v", got)
	}
	if got.Projection.State != "unavailable" || got.Projection.NodeRoot != "" || got.Projection.SchemaVersion != "" {
		t.Fatalf("in-memory projection metadata = %#v, want unavailable without watermark", got.Projection)
	}
}

func TestSPLResolveReportsMatchingBranchHeadProjection(t *testing.T) {
	repo, err := repository.InitializeRepository(t.TempDir())
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	head, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}
	status, err := repo.ProjectionStatus()
	if err != nil {
		t.Fatalf("ProjectionStatus: %v", err)
	}

	got, err := NewResolveTool(repo).SPLResolve(context.Background(), ResolveRequest{
		Selector: SnapshotSelector{Branch: "main"},
		NodeID:   repository.SeedNodeID,
	})
	if err != nil {
		t.Fatalf("SPLResolve: %v", err)
	}

	if got.Snapshot.Repository != repo.RepositoryID() || got.Snapshot.Branch != "main" ||
		got.Snapshot.Commit != string(head) || got.Snapshot.Root == "" {
		t.Fatalf("snapshot metadata = %#v", got.Snapshot)
	}
	if got.Projection.State != status.State || got.Projection.NodeRoot != string(status.NodeRoot) ||
		got.Projection.SchemaVersion != "v1" {
		t.Fatalf("projection metadata = %#v, status = %#v", got.Projection, status)
	}
	validation, err := NewResolveTool(repo).SPLValidateSchema(context.Background(), SchemaValidationRequest{
		Selector: SnapshotSelector{Branch: "main"},
	})
	if err != nil {
		t.Fatalf("SPLValidateSchema: %v", err)
	}
	if validation.Snapshot != got.Snapshot {
		t.Fatalf("validation snapshot metadata = %#v, want %#v", validation.Snapshot, got.Snapshot)
	}
	if validation.Projection != got.Projection {
		t.Fatalf("validation projection metadata = %#v, want %#v", validation.Projection, got.Projection)
	}
}

func TestSPLResolveReportsHistoricalProjectionAsUnavailable(t *testing.T) {
	repo, err := repository.InitializeRepository(t.TempDir())
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	historical, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}
	head, err := repo.AdvanceBranch("main")
	if err != nil {
		t.Fatalf("AdvanceBranch: %v", err)
	}
	status, err := repo.ProjectionStatus()
	if err != nil {
		t.Fatalf("ProjectionStatus: %v", err)
	}
	if status.Commit != head {
		t.Fatalf("projection commit = %q, want current head %q", status.Commit, head)
	}
	commit := string(historical)

	got, err := NewResolveTool(repo).SPLResolve(context.Background(), ResolveRequest{
		Selector: SnapshotSelector{Branch: "main", Commit: &commit},
		NodeID:   repository.SeedNodeID,
	})
	if err != nil {
		t.Fatalf("SPLResolve: %v", err)
	}

	if got.Snapshot.Commit != commit || got.Node.ID != repository.SeedNodeID {
		t.Fatalf("historical canonical resolution = %#v", got)
	}
	if got.Projection.State != "unavailable" || got.Projection.NodeRoot != "" || got.Projection.SchemaVersion != "" {
		t.Fatalf("historical projection metadata = %#v, want unavailable without watermark", got.Projection)
	}
}

func TestReadEnvelopesReportTargetProjectionProvenance(t *testing.T) {
	repo, err := repository.InitializeRepository(t.TempDir())
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	base, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch base: %v", err)
	}
	target, err := repo.AdvanceBranch("main")
	if err != nil {
		t.Fatalf("AdvanceBranch: %v", err)
	}
	status, err := repo.ProjectionStatus()
	if err != nil {
		t.Fatalf("ProjectionStatus: %v", err)
	}
	record, err := repo.PinnedSnapshotRecord(target)
	if err != nil {
		t.Fatalf("PinnedSnapshotRecord: %v", err)
	}
	wantSnapshot := SnapshotMetadata{
		Repository: repo.RepositoryID(), Branch: "main", Commit: string(target), Root: string(record.Snapshot),
	}
	wantProjection := ProjectionMetadata{
		NodeRoot: string(status.NodeRoot), State: status.State, SchemaVersion: "v1",
	}
	tool := NewResolveTool(repo)
	baseCommit := string(base)
	diff, err := tool.SPLDiff(context.Background(), DiffRequest{
		Base:   SnapshotSelector{Branch: "main", Commit: &baseCommit},
		Target: SnapshotSelector{Branch: "main"},
	})
	if err != nil {
		t.Fatalf("SPLDiff: %v", err)
	}
	if diff.Base.Commit != baseCommit || diff.Target != wantSnapshot || diff.Projection != wantProjection {
		t.Fatalf("diff provenance = %#v, want base %q and target %#v / %#v", diff, base, wantSnapshot, wantProjection)
	}
	history, err := tool.SPLHistory(context.Background(), HistoryRequest{
		Selector: SnapshotSelector{Branch: "main"}, EntityID: repository.SeedNodeID,
	})
	if err != nil {
		t.Fatalf("SPLHistory: %v", err)
	}
	impact, err := tool.SPLImpact(context.Background(), ImpactRequest{
		Selector: SnapshotSelector{Branch: "main"},
		Request: repository.ImpactRequest{
			Delta: []repository.MutationOperation{
				{Action: "update", Entity: "node", ID: repository.SeedNodeID, Title: "Changed"},
			},
		},
	})
	if err != nil {
		t.Fatalf("SPLImpact: %v", err)
	}
	validation, err := tool.SPLValidateSchema(context.Background(), SchemaValidationRequest{
		Selector: SnapshotSelector{Branch: "main"},
	})
	if err != nil {
		t.Fatalf("SPLValidateSchema: %v", err)
	}
	for _, result := range []struct {
		name   string
		value  any
		fields []string
	}{
		{name: "diff", value: diff, fields: []string{"base", "target", "projection", "baseCommit", "targetCommit"}},
		{name: "history", value: history, fields: []string{"snapshot", "projection", "entries"}},
		{name: "impact", value: impact, fields: []string{"snapshot", "projection", "commit", "impacts"}},
		{name: "validation", value: validation, fields: []string{"snapshot", "projection", "schema", "valid", "violations"}},
	} {
		t.Run(result.name, func(t *testing.T) {
			encoded, err := json.Marshal(result.value)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var object map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &object); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			for _, field := range result.fields {
				if _, ok := object[field]; !ok {
					t.Errorf("JSON omitted %q: %s", field, encoded)
				}
			}
		})
	}
	if history.Snapshot != wantSnapshot || history.Projection != wantProjection ||
		impact.Snapshot != wantSnapshot || impact.Projection != wantProjection ||
		validation.Snapshot != wantSnapshot || validation.Projection != wantProjection {
		t.Fatalf("read envelope provenance = history %#v impact %#v validation %#v, want %#v / %#v",
			history, impact, validation, wantSnapshot, wantProjection)
	}
}

func TestSPLDiffBoundsPublicEnvelope(t *testing.T) {
	repo := newTestSeedRepository(t)
	base, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch base: %v", err)
	}
	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{
		Branch: "main",
		Operations: []repository.MutationOperation{
			{Action: "add", Entity: "node", ID: "node-2", Title: "Second"},
			{Action: "add", Entity: "node", ID: "node-3", Title: "Third"},
			{Action: "add", Entity: "node", ID: "node-4", Title: "Fourth"},
		},
	}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	target, err := repo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("CommitStagedMutations: %v", err)
	}

	tool := NewResolveTool(repo)
	baseCommit, targetCommit := string(base), string(target.Commit)
	one, generous := 1, 1<<20
	page, err := tool.SPLDiff(context.Background(), DiffRequest{
		Base:   SnapshotSelector{Branch: "main", Commit: &baseCommit},
		Target: SnapshotSelector{Branch: "main", Commit: &targetCommit},
		Budget: QueryBudgetRequest{MaxRows: &one, MaxResponseBytes: &generous},
	})
	if err != nil {
		t.Fatalf("SPLDiff baseline: %v", err)
	}
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("Marshal baseline: %v", err)
	}

	// Reserve room for the conservative elapsed-time metadata used while
	// calculating the public payload budget.
	rows, maxBytes := 3, len(encoded)+16
	got, err := tool.SPLDiff(context.Background(), DiffRequest{
		Base:   SnapshotSelector{Branch: "main", Commit: &baseCommit},
		Target: SnapshotSelector{Branch: "main", Commit: &targetCommit},
		Budget: QueryBudgetRequest{MaxRows: &rows, MaxResponseBytes: &maxBytes},
	})
	if err != nil {
		t.Fatalf("SPLDiff bounded: %v", err)
	}
	encoded, err = json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal bounded result: %v", err)
	}
	if len(encoded) > maxBytes {
		t.Fatalf("serialized diff size = %d, want at most %d", len(encoded), maxBytes)
	}
	if len(got.Changes) != 1 || got.ContinuationToken == "" {
		t.Fatalf("bounded result = %#v, want one change and continuation", got)
	}
	if !got.Completion.Truncated || got.Completion.Complete || got.Completion.TimedOut ||
		got.Completion.Visited != len(got.Changes)+len(got.Context) || got.Completion.ResponseBytes != len(encoded) {
		t.Fatalf("bounded completion = %#v", got.Completion)
	}
	next, err := tool.SPLDiff(context.Background(), DiffRequest{
		Base:              SnapshotSelector{Branch: "main", Commit: &baseCommit},
		Target:            SnapshotSelector{Branch: "main", Commit: &targetCommit},
		Budget:            QueryBudgetRequest{MaxRows: &rows, MaxResponseBytes: &maxBytes},
		ContinuationToken: got.ContinuationToken,
	})
	if err != nil {
		t.Fatalf("SPLDiff continuation: %v", err)
	}
	if len(next.Changes) != 1 || next.Changes[0].ID == got.Changes[0].ID {
		t.Fatalf("continuation result = %#v, want next single change", next)
	}

	tooSmall := 1
	_, err = tool.SPLDiff(context.Background(), DiffRequest{
		Base:   SnapshotSelector{Branch: "main", Commit: &baseCommit},
		Target: SnapshotSelector{Branch: "main", Commit: &targetCommit},
		Budget: QueryBudgetRequest{MaxResponseBytes: &tooSmall},
	})
	if !errors.Is(err, repository.ErrResponseBudgetTooSmall) {
		t.Fatalf("too-small response budget error = %v, want %v", err, repository.ErrResponseBudgetTooSmall)
	}
}

func TestSPLImpactNormalizesTraversalBudget(t *testing.T) {
	repo := newTestSeedRepository(t)
	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{Branch: "main", Operations: []repository.MutationOperation{
		{Action: "add", Entity: "node", ID: "node-2", Title: "Second"},
		{Action: "add", Entity: "node", ID: "node-3", Title: "Third"},
		{Action: "add", Entity: "edge", ID: "edge-1", Source: repository.SeedNodeID, Target: "node-2"},
		{Action: "add", Entity: "edge", ID: "edge-2", Source: "node-2", Target: "node-3"},
	}}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	if _, err := repo.CommitStagedMutations("main"); err != nil {
		t.Fatalf("CommitStagedMutations: %v", err)
	}
	one, ten := 1, 10
	before, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}
	result, err := NewResolveTool(repo).SPLImpact(context.Background(), ImpactRequest{
		Selector: SnapshotSelector{Branch: "main"},
		Request: repository.ImpactRequest{
			Delta: []repository.MutationOperation{
				{Action: "update", Entity: "node", ID: repository.SeedNodeID, Title: "Changed"},
			},
		},
		Budget: QueryBudgetRequest{MaxDepth: &one, MaxVisited: &ten},
	})
	if err != nil {
		t.Fatalf("SPLImpact: %v", err)
	}
	if len(result.Impacts) != 2 ||
		result.Impacts[0].Node.ID != repository.SeedNodeID ||
		result.Impacts[1].Node.ID != "node-2" {
		t.Fatalf("normalized impact result = %#v", result)
	}
	if result.Snapshot.Repository == "" || result.Snapshot.Branch != "main" ||
		result.Snapshot.Commit != string(before) || result.Snapshot.Root == "" ||
		result.Projection.State != "unavailable" || result.Projection.NodeRoot != "" {
		t.Fatalf("impact envelope = %#v", result)
	}
	after, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch after impact: %v", err)
	}
	if after != before {
		t.Fatalf("impact moved branch from %q to %q", before, after)
	}
}

func TestSPLHistoryReturnsSnapshotAndProjectionMetadata(t *testing.T) {
	repo := newTestSeedRepository(t)
	head, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}

	result, err := NewResolveTool(repo).SPLHistory(context.Background(), HistoryRequest{
		Selector: SnapshotSelector{Branch: "main"}, EntityID: repository.SeedNodeID,
	})
	if err != nil {
		t.Fatalf("SPLHistory: %v", err)
	}
	if len(result.Entries) == 0 || result.Snapshot.Repository == "" ||
		result.Snapshot.Branch != "main" || result.Snapshot.Commit != string(head) ||
		result.Snapshot.Root == "" || result.Projection.State != "unavailable" ||
		result.Projection.NodeRoot != "" {
		t.Fatalf("history envelope = %#v", result)
	}
}

func TestQueryEnvelopesReportCompletionAndFullResponseBytes(t *testing.T) {
	repo := newTestSeedRepository(t)
	base, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch base: %v", err)
	}
	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{
		Branch: "main",
		Operations: []repository.MutationOperation{
			{Action: "add", Entity: "node", ID: "node-2", Title: "Second"},
			{Action: "add", Entity: "edge", ID: "edge-1", Source: repository.SeedNodeID, Target: "node-2"},
		},
	}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	target, err := repo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("CommitStagedMutations: %v", err)
	}
	if _, err := repo.CreateBranch("feature", repository.BranchSource{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	tool := NewResolveTool(repo)
	baseCommit, targetCommit := string(base), string(target.Commit)

	resolved, err := tool.SPLResolve(context.Background(), ResolveRequest{
		Selector: SnapshotSelector{Branch: "main"}, NodeID: repository.SeedNodeID,
	})
	if err != nil {
		t.Fatalf("SPLResolve: %v", err)
	}
	diff, err := tool.SPLDiff(context.Background(), DiffRequest{
		Base:   SnapshotSelector{Branch: "main", Commit: &baseCommit},
		Target: SnapshotSelector{Branch: "main", Commit: &targetCommit},
	})
	if err != nil {
		t.Fatalf("SPLDiff: %v", err)
	}
	history, err := tool.SPLHistory(context.Background(), HistoryRequest{
		Selector: SnapshotSelector{Branch: "main"}, EntityID: repository.SeedNodeID,
	})
	if err != nil {
		t.Fatalf("SPLHistory: %v", err)
	}
	branches, err := tool.SPLBranchesContaining(context.Background(), ContainmentSelector{EntityID: repository.SeedNodeID})
	if err != nil {
		t.Fatalf("SPLBranchesContaining: %v", err)
	}
	impact, err := tool.SPLImpact(context.Background(), ImpactRequest{
		Selector: SnapshotSelector{Branch: "main"},
		Request: repository.ImpactRequest{Delta: []repository.MutationOperation{
			{Action: "update", Entity: "node", ID: repository.SeedNodeID, Title: "Changed"},
		}},
	})
	if err != nil {
		t.Fatalf("SPLImpact: %v", err)
	}

	for _, query := range []struct {
		name       string
		envelope   any
		budget     QueryBudget
		completion QueryCompletionMetadata
		visited    int
	}{
		{"resolve", resolved, resolved.Budget, resolved.Completion, 1},
		{"diff", diff, diff.Budget, diff.Completion, len(diff.Changes) + len(diff.Context)},
		{"history", history, history.Budget, history.Completion, len(history.Entries)},
		{"branches", branches, branches.Budget, branches.Completion, len(branches.Branches)},
		{"impact", impact, impact.Budget, impact.Completion, len(impact.Impacts)},
	} {
		t.Run(query.name, func(t *testing.T) {
			encoded, err := json.Marshal(query.envelope)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if query.completion.ResponseBytes != len(encoded) {
				t.Fatalf("response bytes = %d, want %d", query.completion.ResponseBytes, len(encoded))
			}
			if query.completion.Visited != query.visited || query.completion.Truncated ||
				query.completion.TimedOut || !query.completion.Complete {
				t.Fatalf("completion = %#v, visited = %d", query.completion, query.visited)
			}
			if query.budget != DefaultQueryBudget() {
				t.Fatalf("budget = %#v, want %#v", query.budget, DefaultQueryBudget())
			}
			var object map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &object); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			for _, field := range []string{"budget", "completion"} {
				if _, ok := object[field]; !ok {
					t.Errorf("JSON omitted %q: %s", field, encoded)
				}
			}
		})
	}
}

func TestPagedQueryCompletionReportsRowAndVisitedTruncation(t *testing.T) {
	repo := newTestSeedRepository(t)
	base, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch base: %v", err)
	}
	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{
		Branch: "main",
		Operations: []repository.MutationOperation{
			{Action: "add", Entity: "node", ID: "node-2", Title: "Second"},
			{Action: "add", Entity: "node", ID: "node-3", Title: "Third"},
			{Action: "add", Entity: "edge", ID: "edge-1", Source: repository.SeedNodeID, Target: "node-2"},
			{Action: "add", Entity: "edge", ID: "edge-2", Source: "node-2", Target: "node-3"},
		},
	}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	target, err := repo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("CommitStagedMutations: %v", err)
	}
	if _, err := repo.CreateBranch("feature", repository.BranchSource{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	one, generous := 1, 1<<20
	tool := NewResolveTool(repo)
	baseCommit, targetCommit := string(base), string(target.Commit)

	diff, err := tool.SPLDiff(context.Background(), DiffRequest{
		Base:   SnapshotSelector{Branch: "main", Commit: &baseCommit},
		Target: SnapshotSelector{Branch: "main", Commit: &targetCommit},
		Budget: QueryBudgetRequest{MaxRows: &one, MaxResponseBytes: &generous},
	})
	if err != nil {
		t.Fatalf("SPLDiff: %v", err)
	}
	if !diff.Completion.Truncated || diff.Completion.Complete || diff.ContinuationToken == "" || diff.Completion.Visited != 1 {
		t.Fatalf("diff completion = %#v, result = %#v", diff.Completion, diff.DiffResult)
	}

	contextRepo := newTestSeedRepository(t)
	if _, err := contextRepo.StageMutationBatch(repository.StageMutationRequest{
		Branch: "main",
		Operations: []repository.MutationOperation{
			{Action: "add", Entity: "node", ID: "node-2", Title: "Second"},
			{Action: "add", Entity: "edge", ID: "edge-1", Source: repository.SeedNodeID, Target: "node-2"},
		},
	}); err != nil {
		t.Fatalf("stage context graph: %v", err)
	}
	if _, err := contextRepo.CommitStagedMutations("main"); err != nil {
		t.Fatalf("commit context graph: %v", err)
	}
	contextBase, err := contextRepo.PinBranch("main")
	if err != nil {
		t.Fatalf("pin context base: %v", err)
	}
	if _, err := contextRepo.StageMutationBatch(repository.StageMutationRequest{
		Branch: "main",
		Operations: []repository.MutationOperation{
			{Action: "update", Entity: "node", ID: repository.SeedNodeID, Title: "Updated"},
		},
	}); err != nil {
		t.Fatalf("stage context change: %v", err)
	}
	contextTarget, err := contextRepo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("commit context change: %v", err)
	}
	contextBaseCommit, contextTargetCommit := string(contextBase), string(contextTarget.Commit)
	contextPage, err := NewResolveTool(contextRepo).SPLDiff(context.Background(), DiffRequest{
		Base:          SnapshotSelector{Branch: "main", Commit: &contextBaseCommit},
		Target:        SnapshotSelector{Branch: "main", Commit: &contextTargetCommit},
		Filter:        repository.DiffFilter{NodeIDs: []string{repository.SeedNodeID}},
		IncludeOneHop: true,
		Budget:        QueryBudgetRequest{MaxRows: &one, MaxResponseBytes: &generous},
	})
	if err != nil {
		t.Fatalf("SPLDiff one-hop context: %v", err)
	}
	if !contextPage.ContextTruncated || contextPage.ContinuationToken != "" ||
		!contextPage.Completion.Truncated || contextPage.Completion.Complete {
		t.Fatalf("context page = %#v", contextPage)
	}

	branches, err := tool.SPLBranchesContainingPage(context.Background(), BranchesContainingRequest{
		Selector: ContainmentSelector{EntityID: repository.SeedNodeID},
		Budget:   QueryBudgetRequest{MaxRows: &one, MaxResponseBytes: &generous},
	})
	if err != nil {
		t.Fatalf("SPLBranchesContainingPage: %v", err)
	}
	if !branches.Completion.Truncated || branches.Completion.Complete ||
		branches.ContinuationToken == "" || branches.Completion.Visited != 1 {
		t.Fatalf("branch completion = %#v, result = %#v", branches.Completion, branches.BranchContainmentResult)
	}

	impact, err := tool.SPLImpact(context.Background(), ImpactRequest{
		Selector: SnapshotSelector{Branch: "main"},
		Request: repository.ImpactRequest{Delta: []repository.MutationOperation{
			{Action: "update", Entity: "node", ID: repository.SeedNodeID, Title: "Changed"},
		}},
		Budget: QueryBudgetRequest{MaxRows: &generous, MaxVisited: &one, MaxResponseBytes: &generous},
	})
	if err != nil {
		t.Fatalf("SPLImpact: %v", err)
	}
	if !impact.CapacityExhausted || !impact.Completion.Truncated || impact.Completion.Complete ||
		impact.Completion.Visited != 1 {
		t.Fatalf("impact completion = %#v, result = %#v", impact.Completion, impact.ImpactResult)
	}
}

func TestQueryDeadlineWithoutListPrefixReturnsError(t *testing.T) {
	zero := time.Duration(0)
	_, err := NewResolveTool(newTestSeedRepository(t)).SPLResolve(context.Background(), ResolveRequest{
		Selector: SnapshotSelector{Branch: "main"},
		NodeID:   repository.SeedNodeID,
		Budget:   QueryBudgetRequest{Timeout: &zero},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SPLResolve deadline error = %v, want context.DeadlineExceeded", err)
	}
}

func TestResolveToolReadAPIsRejectUnreachableExplicitCommits(t *testing.T) {
	repo := newTestSeedRepository(t)
	if _, err := repo.CreateBranch("feature", repository.BranchSource{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	featureCommit, err := repo.AdvanceBranch("feature")
	if err != nil {
		t.Fatalf("AdvanceBranch feature: %v", err)
	}
	commit := string(featureCommit)
	selector := SnapshotSelector{Branch: "main", Commit: &commit}
	tool := NewResolveTool(repo)

	requests := []struct {
		name string
		call func() error
	}{
		{
			name: "resolve",
			call: func() error {
				_, err := tool.SPLResolve(context.Background(), ResolveRequest{
					Selector: selector, NodeID: repository.SeedNodeID,
				})
				return err
			},
		},
		{
			name: "validate",
			call: func() error {
				_, err := tool.SPLValidateSchema(context.Background(), SchemaValidationRequest{Selector: selector})
				return err
			},
		},
		{
			name: "diff",
			call: func() error {
				_, err := tool.SPLDiff(context.Background(), DiffRequest{
					Base: SnapshotSelector{Branch: "main"}, Target: selector,
				})
				return err
			},
		},
		{
			name: "history",
			call: func() error {
				_, err := tool.SPLHistory(context.Background(), HistoryRequest{
					Selector: selector, EntityID: repository.SeedNodeID,
				})
				return err
			},
		},
		{
			name: "impact",
			call: func() error {
				_, err := tool.SPLImpact(context.Background(), ImpactRequest{
					Selector: selector,
					Request: repository.ImpactRequest{
						Delta: []repository.MutationOperation{
							{Action: "update", Entity: "node", ID: repository.SeedNodeID, Title: "Changed"},
						},
					},
				})
				return err
			},
		},
	}
	for _, request := range requests {
		t.Run(request.name, func(t *testing.T) {
			if err := request.call(); !errors.Is(err, ErrUnsupportedCommit) {
				t.Fatalf("error = %v, want ErrUnsupportedCommit", err)
			}
		})
	}
}

func TestResolveToolReadAPIsAllowDetachedCommitsOnlyWhenConfigured(t *testing.T) {
	repo := newTestSeedRepository(t)
	if _, err := repo.CreateBranch("feature", repository.BranchSource{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	featureCommit, err := repo.AdvanceBranch("feature")
	if err != nil {
		t.Fatalf("AdvanceBranch feature: %v", err)
	}
	commit := string(featureCommit)
	selector := SnapshotSelector{Branch: "main", Commit: &commit}
	tool := NewResolveToolWithOptions(repo, Options{AllowDetachedCommit: true})

	resolved, err := tool.SPLResolve(context.Background(), ResolveRequest{
		Selector: selector, NodeID: repository.SeedNodeID,
	})
	if err != nil {
		t.Fatalf("SPLResolve with detached access: %v", err)
	}
	if resolved.Snapshot.Commit != commit || resolved.Snapshot.Branch != "main" {
		t.Fatalf("detached resolve envelope = %#v", resolved)
	}
	validation, err := tool.SPLValidateSchema(context.Background(), SchemaValidationRequest{Selector: selector})
	if err != nil {
		t.Fatalf("SPLValidateSchema with detached access: %v", err)
	}
	if validation.Snapshot.Commit != commit || validation.Snapshot.Branch != "main" {
		t.Fatalf("detached validation envelope = %#v", validation)
	}
	if _, err := tool.SPLDiff(context.Background(), DiffRequest{
		Base: SnapshotSelector{Branch: "main"}, Target: selector,
	}); err != nil {
		t.Fatalf("SPLDiff with detached access: %v", err)
	}
	if _, err := tool.SPLHistory(context.Background(), HistoryRequest{
		Selector: selector, EntityID: repository.SeedNodeID,
	}); err != nil {
		t.Fatalf("SPLHistory with detached access: %v", err)
	}
	if _, err := tool.SPLImpact(context.Background(), ImpactRequest{
		Selector: selector,
		Request: repository.ImpactRequest{
			Delta: []repository.MutationOperation{
				{Action: "update", Entity: "node", ID: repository.SeedNodeID, Title: "Changed"},
			},
		},
	}); err != nil {
		t.Fatalf("SPLImpact with detached access: %v", err)
	}
}

func TestResolveToolSnapshotReadAPIsRequireBranch(t *testing.T) {
	tool := NewResolveTool(newTestSeedRepository(t))
	requests := []struct {
		name string
		call func() error
	}{
		{
			name: "resolve",
			call: func() error {
				_, err := tool.SPLResolve(context.Background(), ResolveRequest{NodeID: repository.SeedNodeID})
				return err
			},
		},
		{
			name: "validate",
			call: func() error {
				_, err := tool.SPLValidateSchema(context.Background(), SchemaValidationRequest{})
				return err
			},
		},
		{
			name: "diff base",
			call: func() error {
				_, err := tool.SPLDiff(context.Background(), DiffRequest{
					Base: SnapshotSelector{}, Target: SnapshotSelector{Branch: "main"},
				})
				return err
			},
		},
		{
			name: "diff target",
			call: func() error {
				_, err := tool.SPLDiff(context.Background(), DiffRequest{
					Base: SnapshotSelector{Branch: "main"}, Target: SnapshotSelector{},
				})
				return err
			},
		},
		{
			name: "history",
			call: func() error {
				_, err := tool.SPLHistory(context.Background(), HistoryRequest{EntityID: repository.SeedNodeID})
				return err
			},
		},
		{
			name: "impact",
			call: func() error {
				_, err := tool.SPLImpact(context.Background(), ImpactRequest{
					Request: repository.ImpactRequest{
						Delta: []repository.MutationOperation{
							{Action: "update", Entity: "node", ID: repository.SeedNodeID, Title: "Changed"},
						},
					},
				})
				return err
			},
		},
	}
	for _, request := range requests {
		t.Run(request.name, func(t *testing.T) {
			if err := request.call(); !errors.Is(err, ErrMissingBranch) {
				t.Fatalf("error = %v, want ErrMissingBranch", err)
			}
		})
	}
}

func TestResolveToolSingleSnapshotReadAPIsRemainPinnedAfterBranchAdvances(t *testing.T) {
	reads := []struct {
		name string
		call func(*ResolveTool) (SnapshotMetadata, error)
	}{
		{
			name: "resolve",
			call: func(tool *ResolveTool) (SnapshotMetadata, error) {
				result, err := tool.SPLResolve(context.Background(), ResolveRequest{
					Selector: SnapshotSelector{Branch: "main"}, NodeID: repository.SeedNodeID,
				})
				return result.Snapshot, err
			},
		},
		{
			name: "validate",
			call: func(tool *ResolveTool) (SnapshotMetadata, error) {
				result, err := tool.SPLValidateSchema(context.Background(), SchemaValidationRequest{
					Selector: SnapshotSelector{Branch: "main"},
				})
				return result.Snapshot, err
			},
		},
		{
			name: "history",
			call: func(tool *ResolveTool) (SnapshotMetadata, error) {
				result, err := tool.SPLHistory(context.Background(), HistoryRequest{
					Selector: SnapshotSelector{Branch: "main"}, EntityID: repository.SeedNodeID,
				})
				return result.Snapshot, err
			},
		},
		{
			name: "impact",
			call: func(tool *ResolveTool) (SnapshotMetadata, error) {
				result, err := tool.SPLImpact(context.Background(), ImpactRequest{
					Selector: SnapshotSelector{Branch: "main"},
					Request: repository.ImpactRequest{
						Delta: []repository.MutationOperation{
							{Action: "update", Entity: "node", ID: repository.SeedNodeID, Title: "Changed"},
						},
					},
				})
				return result.Snapshot, err
			},
		},
	}
	for _, read := range reads {
		t.Run(read.name, func(t *testing.T) {
			repo := newTestSeedRepository(t)
			pinned, err := repo.PinBranch("main")
			if err != nil {
				t.Fatalf("PinBranch: %v", err)
			}
			tool := NewResolveTool(repo)
			var advanceErr error
			tool.resolver.afterBranchResolved = func() {
				_, advanceErr = repo.AdvanceBranch("main")
			}

			snapshot, err := read.call(tool)
			if advanceErr != nil {
				t.Fatalf("AdvanceBranch: %v", advanceErr)
			}
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if snapshot.Commit != string(pinned) || snapshot.Branch != "main" || snapshot.Root == "" {
				t.Fatalf("pinned snapshot = %#v, want main at %q", snapshot, pinned)
			}
			current, err := repo.PinBranch("main")
			if err != nil {
				t.Fatalf("PinBranch after read: %v", err)
			}
			if current == pinned {
				t.Fatal("branch did not advance during read")
			}
		})
	}
}

func TestResolveToolReadAPIsPreserveUnknownSelectorErrors(t *testing.T) {
	tool := NewResolveTool(newTestSeedRepository(t))
	if _, err := tool.SPLDiff(context.Background(), DiffRequest{
		Base: SnapshotSelector{Branch: "missing"}, Target: SnapshotSelector{Branch: "main"},
	}); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("SPLDiff unknown branch error = %v, want ErrBranchNotFound", err)
	}

	unknown := "unknown"
	if _, err := tool.SPLHistory(context.Background(), HistoryRequest{
		Selector: SnapshotSelector{Branch: "main", Commit: &unknown}, EntityID: repository.SeedNodeID,
	}); !errors.Is(err, repository.ErrCommitNotFound) {
		t.Fatalf("SPLHistory unknown commit error = %v, want ErrCommitNotFound", err)
	}
	if _, err := tool.SPLImpact(context.Background(), ImpactRequest{
		Selector: SnapshotSelector{Branch: "missing"},
		Request: repository.ImpactRequest{
			Delta: []repository.MutationOperation{
				{Action: "update", Entity: "node", ID: repository.SeedNodeID, Title: "Changed"},
			},
		},
	}); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("SPLImpact unknown branch error = %v, want ErrBranchNotFound", err)
	}
}

func TestSPLDiffPinsBothEndpointsIndependently(t *testing.T) {
	repo := newTestSeedRepository(t)
	initial, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}
	tool := NewResolveTool(repo)
	calls := 0
	tool.resolver.afterBranchResolved = func() {
		calls++
		if calls == 1 {
			if _, err := repo.AdvanceBranch("main"); err != nil {
				t.Fatalf("AdvanceBranch: %v", err)
			}
		}
	}

	result, err := tool.SPLDiff(context.Background(), DiffRequest{
		Base: SnapshotSelector{Branch: "main"}, Target: SnapshotSelector{Branch: "main"},
	})
	if err != nil {
		t.Fatalf("SPLDiff: %v", err)
	}
	current, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch after diff: %v", err)
	}
	if calls != 2 || result.BaseCommit != initial || result.TargetCommit != current {
		t.Fatalf("diff resolution = %#v after %d selections, want %q -> %q", result, calls, initial, current)
	}
	if result.Base.Repository == "" || result.Base.Branch != "main" ||
		result.Base.Commit != string(initial) || result.Base.Root == "" ||
		result.Target.Repository == "" || result.Target.Branch != "main" ||
		result.Target.Commit != string(current) || result.Target.Root == "" ||
		result.Projection.State != "unavailable" || result.Projection.NodeRoot != "" {
		t.Fatalf("diff envelope = %#v", result)
	}
}

func TestNormalizeQueryBudgetHonorsLowerRequestsWithoutExceedingConfiguration(t *testing.T) {
	configured := QueryBudget{
		MaxRows:          100,
		MaxResponseBytes: 1_000,
		MaxDepth:         10,
		MaxVisited:       500,
		Timeout:          time.Second,
	}
	zero := 0
	lowerRows := 25
	lowerBytes := 200
	lowerDepth := 3
	lowerVisited := 100
	lowerTimeout := 100 * time.Millisecond

	got := NormalizeQueryBudget(QueryBudgetRequest{
		MaxRows:          &lowerRows,
		MaxResponseBytes: &lowerBytes,
		MaxDepth:         &lowerDepth,
		MaxVisited:       &lowerVisited,
		Timeout:          &lowerTimeout,
	}, &configured)
	want := QueryBudget{
		MaxRows:          lowerRows,
		MaxResponseBytes: lowerBytes,
		MaxDepth:         lowerDepth,
		MaxVisited:       lowerVisited,
		Timeout:          lowerTimeout,
	}
	if got != want {
		t.Fatalf("normalized budget = %#v, want %#v", got, want)
	}

	overRows, overBytes, overDepth, overVisited := 200, 2_000, 20, 1_000
	overTimeout := 2 * time.Second
	got = NormalizeQueryBudget(QueryBudgetRequest{
		MaxRows:          &overRows,
		MaxResponseBytes: &overBytes,
		MaxDepth:         &overDepth,
		MaxVisited:       &overVisited,
		Timeout:          &overTimeout,
	}, &configured)
	if got.MaxRows > configured.MaxRows ||
		got.MaxResponseBytes > configured.MaxResponseBytes ||
		got.MaxDepth > configured.MaxDepth ||
		got.MaxVisited > configured.MaxVisited ||
		got.Timeout > configured.Timeout {
		t.Fatalf("effective budget exceeds configuration: %#v", got)
	}

	got = NormalizeQueryBudget(QueryBudgetRequest{MaxRows: &zero}, &configured)
	if got.MaxRows != 0 || got.MaxDepth != configured.MaxDepth {
		t.Fatalf("explicit zero or omitted fields not preserved: %#v", got)
	}
}

func TestSPLResolveRejectsMissingBranch(t *testing.T) {
	tool := NewResolveTool(newTestSeedRepository(t))

	_, err := tool.SPLResolve(context.Background(), ResolveRequest{
		Selector: SnapshotSelector{},
		NodeID:   repository.SeedNodeID,
	})
	if !errors.Is(err, ErrMissingBranch) {
		t.Fatalf("SPLResolve error = %v, want ErrMissingBranch", err)
	}
}

func TestSPLResolveUsesExplicitReachableCommit(t *testing.T) {
	repo := newTestSeedRepository(t)
	olderCommit, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}
	if _, err := repo.AdvanceBranch("main"); err != nil {
		t.Fatalf("AdvanceBranch: %v", err)
	}

	commit := string(olderCommit)
	got, err := NewResolveTool(repo).SPLResolve(context.Background(), ResolveRequest{
		Selector: SnapshotSelector{Branch: "main", Commit: &commit},
		NodeID:   repository.SeedNodeID,
	})
	if err != nil {
		t.Fatalf("SPLResolve: %v", err)
	}
	if got.Snapshot.Commit != commit {
		t.Fatalf("selected commit = %q, want %q", got.Snapshot.Commit, commit)
	}
}

func TestSPLResolveTraversesAllMergeParentsForExplicitCommit(t *testing.T) {
	repo := newTestSeedRepository(t)
	base, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}
	if _, err := repo.CreateBranch("feature", repository.BranchSource{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{
		Branch: "feature",
		Operations: []repository.MutationOperation{
			{Action: "add", Entity: "node", ID: "feature-node", Title: "Feature node"},
		},
	}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	featureResult, err := repo.CommitStagedMutations("feature")
	if err != nil {
		t.Fatalf("CommitStagedMutations feature: %v", err)
	}
	featureCommit := featureResult.Commit
	mainCommit, err := repo.AdvanceBranch("main")
	if err != nil {
		t.Fatalf("AdvanceBranch main: %v", err)
	}
	if _, err := repo.ApplyCleanBoundMerge("feature", "main", "test-merge", repository.MergePreviewBinding{
		MergeBase: base, SourceCommit: featureCommit, TargetCommit: mainCommit,
	}); err != nil {
		t.Fatalf("ApplyCleanBoundMerge: %v", err)
	}

	commit := string(featureCommit)
	got, err := NewResolveTool(repo).SPLResolve(context.Background(), ResolveRequest{
		Selector: SnapshotSelector{Branch: "main", Commit: &commit},
		NodeID:   repository.SeedNodeID,
	})
	if err != nil {
		t.Fatalf("SPLResolve: %v", err)
	}
	if got.Snapshot.Commit != commit {
		t.Fatalf("selected commit = %q, want second-parent commit %q", got.Snapshot.Commit, commit)
	}
}

func TestSPLResolveRejectsUnreachableExplicitCommitUnlessDetachedAccessAllowed(t *testing.T) {
	repo := newTestSeedRepository(t)
	if _, err := repo.CreateBranch("feature", repository.BranchSource{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	featureCommit, err := repo.AdvanceBranch("feature")
	if err != nil {
		t.Fatalf("AdvanceBranch feature: %v", err)
	}
	commit := string(featureCommit)
	request := ResolveRequest{Selector: SnapshotSelector{Branch: "main", Commit: &commit}, NodeID: repository.SeedNodeID}

	if _, err := NewResolveTool(repo).SPLResolve(context.Background(), request); !errors.Is(err, ErrUnsupportedCommit) {
		t.Fatalf("default policy error = %v, want ErrUnsupportedCommit", err)
	}
	got, err := NewResolveToolWithOptions(repo, Options{AllowDetachedCommit: true}).SPLResolve(context.Background(), request)
	if err != nil {
		t.Fatalf("SPLResolve with detached access: %v", err)
	}
	if got.Snapshot.Commit != commit {
		t.Fatalf("selected commit = %q, want %q", got.Snapshot.Commit, commit)
	}
}

func TestSPLResolveRejectsUnknownExplicitCommitWithoutUnsupportedCategory(t *testing.T) {
	commit := "unknown"
	request := ResolveRequest{Selector: SnapshotSelector{Branch: "main", Commit: &commit}, NodeID: repository.SeedNodeID}
	for _, options := range []Options{{}, {AllowDetachedCommit: true}} {
		_, err := NewResolveToolWithOptions(newTestSeedRepository(t), options).SPLResolve(context.Background(), request)
		if !errors.Is(err, repository.ErrCommitNotFound) || errors.Is(err, ErrUnsupportedCommit) {
			t.Fatalf("options %#v error = %v, want ErrCommitNotFound without ErrUnsupportedCommit", options, err)
		}
	}
}

func TestSPLResolveValidatesBranchWhenDetachedAccessAllowed(t *testing.T) {
	repo := newTestSeedRepository(t)
	commit, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}
	selected := string(commit)
	_, err = NewResolveToolWithOptions(repo, Options{AllowDetachedCommit: true}).SPLResolve(context.Background(), ResolveRequest{
		Selector: SnapshotSelector{Branch: "missing", Commit: &selected},
		NodeID:   repository.SeedNodeID,
	})
	if !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("SPLResolve error = %v, want ErrBranchNotFound", err)
	}
}
