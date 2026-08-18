package resolve

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/repository/branch"
)

func TestResolvePinsBranchCommitForRequest(t *testing.T) {
	repo := repository.NewSeedRepository()
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
	repo := repository.NewSeedRepository()
	resolver := NewResolver(repo)
	ctx, cancel := context.WithCancel(context.Background())
	resolver.afterBranchResolved = cancel

	_, err := resolver.Resolve(ctx, SnapshotSelector{Branch: "main"}, repository.SeedNodeID)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Resolve error = %v, want context.Canceled", err)
	}
}

func TestResolveToolRejectsCanceledMutationWithoutChangingRepository(t *testing.T) {
	repo := repository.NewSeedRepository()
	tool := NewResolveTool(repo)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := tool.EDGStageMutationBatch(ctx, repository.StageMutationRequest{
		Branch: "main",
		Operations: []repository.MutationOperation{
			{Action: "add", Entity: "node", ID: "node-2", Title: "Second"},
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("EDGStageMutationBatch error = %v, want context.Canceled", err)
	}
	status, err := repo.BranchStagingStatus("main")
	if err != nil {
		t.Fatalf("BranchStagingStatus: %v", err)
	}
	if status.Operations != 0 {
		t.Fatalf("canceled mutation staged %d operations", status.Operations)
	}
}

func TestEDGResolveReturnsSnapshotAndProjectionMetadata(t *testing.T) {
	tool := NewResolveTool(repository.NewSeedRepository())

	got, err := tool.EDGResolve(context.Background(), ResolveRequest{
		Selector: SnapshotSelector{Branch: "main"},
		NodeID:   repository.SeedNodeID,
	})
	if err != nil {
		t.Fatalf("EDGResolve: %v", err)
	}

	if got.Node.ID != repository.SeedNodeID {
		t.Fatalf("node ID = %q, want %q", got.Node.ID, repository.SeedNodeID)
	}
	if got.Snapshot.Commit == "" || got.Snapshot.Root == "" || got.Projection.NodeRoot == "" || got.Budget != DefaultQueryBudget() {
		t.Fatalf("missing resolution metadata: %#v", got)
	}
}

func TestEDGImpactNormalizesTraversalBudget(t *testing.T) {
	repo := repository.NewSeedRepository()
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
	result, err := NewResolveTool(repo).EDGImpact(context.Background(), ImpactRequest{
		Request: repository.ImpactRequest{
			Selector: repository.DiffSelector{Branch: "main"},
			Delta: []repository.MutationOperation{
				{Action: "update", Entity: "node", ID: repository.SeedNodeID, Title: "Changed"},
			},
		},
		Budget: QueryBudgetRequest{MaxDepth: &one, MaxVisited: &ten},
	})
	if err != nil {
		t.Fatalf("EDGImpact: %v", err)
	}
	if len(result.Impacts) != 2 ||
		result.Impacts[0].Node.ID != repository.SeedNodeID ||
		result.Impacts[1].Node.ID != "node-2" {
		t.Fatalf("normalized impact result = %#v", result)
	}
	after, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch after impact: %v", err)
	}
	if after != before {
		t.Fatalf("impact moved branch from %q to %q", before, after)
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

func TestEDGResolveRejectsMissingBranch(t *testing.T) {
	tool := NewResolveTool(repository.NewSeedRepository())

	_, err := tool.EDGResolve(context.Background(), ResolveRequest{
		Selector: SnapshotSelector{},
		NodeID:   repository.SeedNodeID,
	})
	if !errors.Is(err, ErrMissingBranch) {
		t.Fatalf("EDGResolve error = %v, want ErrMissingBranch", err)
	}
}

func TestEDGResolveUsesExplicitReachableCommit(t *testing.T) {
	repo := repository.NewSeedRepository()
	olderCommit, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}
	if _, err := repo.AdvanceBranch("main"); err != nil {
		t.Fatalf("AdvanceBranch: %v", err)
	}

	commit := string(olderCommit)
	got, err := NewResolveTool(repo).EDGResolve(context.Background(), ResolveRequest{
		Selector: SnapshotSelector{Branch: "main", Commit: &commit},
		NodeID:   repository.SeedNodeID,
	})
	if err != nil {
		t.Fatalf("EDGResolve: %v", err)
	}
	if got.Snapshot.Commit != commit {
		t.Fatalf("selected commit = %q, want %q", got.Snapshot.Commit, commit)
	}
}

func TestEDGResolveTraversesAllMergeParentsForExplicitCommit(t *testing.T) {
	repo := repository.NewSeedRepository()
	base, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}
	if _, err := repo.CreateBranch("feature", branch.Source{Branch: "main"}); err != nil {
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
	got, err := NewResolveTool(repo).EDGResolve(context.Background(), ResolveRequest{
		Selector: SnapshotSelector{Branch: "main", Commit: &commit},
		NodeID:   repository.SeedNodeID,
	})
	if err != nil {
		t.Fatalf("EDGResolve: %v", err)
	}
	if got.Snapshot.Commit != commit {
		t.Fatalf("selected commit = %q, want second-parent commit %q", got.Snapshot.Commit, commit)
	}
}

func TestEDGResolveRejectsUnreachableExplicitCommitUnlessDetachedAccessAllowed(t *testing.T) {
	repo := repository.NewSeedRepository()
	if _, err := repo.CreateBranch("feature", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	featureCommit, err := repo.AdvanceBranch("feature")
	if err != nil {
		t.Fatalf("AdvanceBranch feature: %v", err)
	}
	commit := string(featureCommit)
	request := ResolveRequest{Selector: SnapshotSelector{Branch: "main", Commit: &commit}, NodeID: repository.SeedNodeID}

	if _, err := NewResolveTool(repo).EDGResolve(context.Background(), request); !errors.Is(err, ErrUnsupportedCommit) {
		t.Fatalf("default policy error = %v, want ErrUnsupportedCommit", err)
	}
	got, err := NewResolveToolWithOptions(repo, Options{AllowDetachedCommit: true}).EDGResolve(context.Background(), request)
	if err != nil {
		t.Fatalf("EDGResolve with detached access: %v", err)
	}
	if got.Snapshot.Commit != commit {
		t.Fatalf("selected commit = %q, want %q", got.Snapshot.Commit, commit)
	}
}

func TestEDGResolveRejectsUnknownExplicitCommitWithoutUnsupportedCategory(t *testing.T) {
	commit := "unknown"
	request := ResolveRequest{Selector: SnapshotSelector{Branch: "main", Commit: &commit}, NodeID: repository.SeedNodeID}
	for _, options := range []Options{{}, {AllowDetachedCommit: true}} {
		_, err := NewResolveToolWithOptions(repository.NewSeedRepository(), options).EDGResolve(context.Background(), request)
		if !errors.Is(err, repository.ErrCommitNotFound) || errors.Is(err, ErrUnsupportedCommit) {
			t.Fatalf("options %#v error = %v, want ErrCommitNotFound without ErrUnsupportedCommit", options, err)
		}
	}
}

func TestEDGResolveValidatesBranchWhenDetachedAccessAllowed(t *testing.T) {
	repo := repository.NewSeedRepository()
	commit, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}
	selected := string(commit)
	_, err = NewResolveToolWithOptions(repo, Options{AllowDetachedCommit: true}).EDGResolve(context.Background(), ResolveRequest{
		Selector: SnapshotSelector{Branch: "missing", Commit: &selected},
		NodeID:   repository.SeedNodeID,
	})
	if !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("EDGResolve error = %v, want ErrBranchNotFound", err)
	}
}

func TestEDGCreateBranchUsesExplicitSource(t *testing.T) {
	repo := repository.NewSeedRepository()
	tool := NewResolveTool(repo)
	mainHead, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}

	result, err := tool.EDGCreateBranch(context.Background(), branch.CreateRequest{
		Name:   "feature",
		Source: branch.Source{Branch: "main"},
	})
	if err != nil {
		t.Fatalf("EDGCreateBranch: %v", err)
	}
	if result != (branch.CreateResult{Name: "feature", Commit: string(mainHead)}) {
		t.Fatalf("result = %#v, want name feature at %q", result, mainHead)
	}
}

func TestEDGCreateBranchRejectsMissingSourceBeforeDuplicateName(t *testing.T) {
	tool := NewResolveTool(repository.NewSeedRepository())

	_, err := tool.EDGCreateBranch(context.Background(), branch.CreateRequest{
		Name:   "main",
		Source: branch.Source{Branch: "missing"},
	})
	if !errors.Is(err, branch.ErrSourceNotFound) {
		t.Fatalf("EDGCreateBranch error = %v, want ErrSourceNotFound", err)
	}

}

func TestEDGListBranchesReturnsSortedLocalBranchNames(t *testing.T) {
	repo := repository.NewSeedRepository()
	if _, err := repo.CreateBranch("zebra", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch zebra: %v", err)
	}

	if _, err := repo.CreateBranch("alpha", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch alpha: %v", err)
	}

	result, err := NewResolveTool(repo).EDGListBranches(context.Background())
	if err != nil {
		t.Fatalf("EDGListBranches: %v", err)
	}
	want := []string{"alpha", "main", "zebra"}
	if !reflect.DeepEqual(result.Branches, want) {
		t.Fatalf("branches = %#v, want %#v", result.Branches, want)
	}
}

func TestEDGDeleteBranchDeletesExistingNonDefaultBranch(t *testing.T) {
	repo := repository.NewSeedRepository()
	if _, err := repo.CreateBranch("feature", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	result, err := NewResolveTool(repo).EDGDeleteBranch(context.Background(), branch.DeleteRequest{Name: "feature"})
	if err != nil {
		t.Fatalf("EDGDeleteBranch: %v", err)
	}
	if result != (branch.DeleteResult{Name: "feature"}) {
		t.Fatalf("result = %#v", result)
	}
	if _, err := repo.PinBranch("feature"); !errors.Is(err, repository.ErrBranchNotFound) {
		t.Fatalf("PinBranch after delete error = %v, want ErrBranchNotFound", err)
	}
}

func TestEDGDeleteBranchRejectsDefaultAndMissingBranches(t *testing.T) {
	tool := NewResolveTool(repository.NewSeedRepository())
	for _, testCase := range []struct {
		name string
		want error
	}{
		{name: "main", want: branch.ErrDefaultProtected},
		{name: "missing", want: branch.ErrNotFound},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := tool.EDGDeleteBranch(context.Background(), branch.DeleteRequest{Name: testCase.name})
			if !errors.Is(err, testCase.want) {
				t.Fatalf("EDGDeleteBranch error = %v, want %v", err, testCase.want)
			}

		})
	}
}

func TestEDGSwitchBranchMakesExistingInactiveBranchActive(t *testing.T) {
	repo := repository.NewSeedRepository()
	if _, err := repo.CreateBranch("feature", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	result, err := NewResolveTool(repo).EDGSwitchBranch(context.Background(), branch.SwitchRequest{Name: "feature"})
	if err != nil {
		t.Fatalf("EDGSwitchBranch: %v", err)
	}
	if result != (branch.SwitchResult{ActiveBranch: "feature"}) {
		t.Fatalf("result = %#v", result)
	}
	initialization, err := repo.Initialization()
	if err != nil {
		t.Fatalf("Initialization: %v", err)
	}
	if initialization.ActiveBranch != "feature" {
		t.Fatalf("active branch = %q, want feature", initialization.ActiveBranch)
	}
}

func TestEDGSwitchBranchRejectsMissingBranchWithoutChangingActiveBranch(t *testing.T) {
	repo := repository.NewSeedRepository()
	tool := NewResolveTool(repo)
	originalActiveBranch, err := repo.Initialization()
	if err != nil {
		t.Fatalf("Initialization before switch: %v", err)
	}

	if _, err := tool.EDGSwitchBranch(context.Background(), branch.SwitchRequest{Name: "missing"}); !errors.Is(err, branch.ErrNotFound) {
		t.Fatalf("EDGSwitchBranch error = %v, want ErrNotFound", err)
	}
	current, err := repo.Initialization()
	if err != nil {
		t.Fatalf("Initialization after switch: %v", err)
	}
	if current.ActiveBranch != originalActiveBranch.ActiveBranch {
		t.Fatalf("active branch = %q, want unchanged %q", current.ActiveBranch, originalActiveBranch.ActiveBranch)
	}
}

func TestEDGSwitchBranchSucceedsWhenBranchIsAlreadyActive(t *testing.T) {
	repo := repository.NewSeedRepository()

	result, err := NewResolveTool(repo).EDGSwitchBranch(context.Background(), branch.SwitchRequest{Name: "main"})
	if err != nil {
		t.Fatalf("EDGSwitchBranch: %v", err)
	}

	if result != (branch.SwitchResult{ActiveBranch: "main"}) {
		t.Fatalf("result = %#v", result)
	}
	initialization, err := repo.Initialization()
	if err != nil {
		t.Fatalf("Initialization: %v", err)
	}
	if initialization.ActiveBranch != "main" {
		t.Fatalf("active branch = %q, want main", initialization.ActiveBranch)
	}
}

func TestEDGBranchStagingStatusReportsStagedDelta(t *testing.T) {
	repo := repository.NewSeedRepository()
	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{
		Branch: "main",
		Operations: []repository.MutationOperation{
			{Action: "add", Entity: "node", ID: "node-2", Title: "Second node"},
		},
	}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}

	status, err := NewResolveTool(repo).EDGBranchStagingStatus(context.Background(), "main")
	if err != nil {
		t.Fatalf("EDGBranchStagingStatus: %v", err)
	}
	if status.Branch != "main" || status.BaseCommit == "" || status.Operations != 1 {
		t.Fatalf("status = %#v", status)
	}
}

func TestEDGCommitStagedMutationsRejectsStaleBaseWithoutChangingBranchOrStaging(t *testing.T) {
	repo := repository.NewSeedRepository()
	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{
		Branch:     "main",
		Operations: []repository.MutationOperation{{Action: "add", Entity: "node", ID: "node-2", Title: "Second node"}},
	}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	if _, err := repo.AdvanceBranch("main"); err != nil {
		t.Fatalf("AdvanceBranch: %v", err)
	}
	head, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch before commit: %v", err)
	}
	staging, err := repo.BranchStagingStatus("main")
	if err != nil {
		t.Fatalf("BranchStagingStatus before commit: %v", err)
	}

	if _, err := NewResolveTool(repo).EDGCommitStagedMutations(context.Background(), "main"); !errors.Is(err, repository.ErrStaleStagedBase) {
		t.Fatalf("EDGCommitStagedMutations error = %v, want ErrStaleStagedBase", err)
	}

	currentHead, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch after commit: %v", err)
	}
	currentStaging, err := repo.BranchStagingStatus("main")
	if err != nil {
		t.Fatalf("BranchStagingStatus after commit: %v", err)
	}
	if currentHead != head || currentStaging != staging {
		t.Fatalf("branch/staging = %q/%#v, want unchanged %q/%#v", currentHead, currentStaging, head, staging)
	}
}
