package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"
)

func TestImpactAppliesDeltaInMemoryAndTraversesOutgoingDependencies(t *testing.T) {
	repo := impactRepository(t)
	before, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}

	result, err := repo.Impact(ImpactRequest{
		Commit: before,
		Delta: []MutationOperation{
			{Action: "update", Entity: "node", ID: SeedNodeID, Title: "Hypothetical"},
		},
		MaxDepth: 3, MaxVisited: 10,
	})
	if err != nil {
		t.Fatalf("Impact: %v", err)
	}
	if result.Commit != before || result.Snapshot != repo.commits[before].Snapshot {
		t.Fatalf("provenance = %#v", result)
	}
	got := make([]string, len(result.Impacts))
	for i, impact := range result.Impacts {
		got[i] = impact.Node.ID
	}
	want := []string{SeedNodeID, "node-2", "node-3", "node-4", "node-6"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("impacts = %#v, want %#v", got, want)
	}
	if path := result.Impacts[3].Path; !reflect.DeepEqual(path, []string{SeedNodeID, "node-3", "node-4"}) {
		t.Fatalf("node-4 path = %#v", path)
	}
	for _, impact := range result.Impacts {
		if impact.Node.ID == "node-5" {
			t.Fatalf("inbound-only dependency was traversed: %#v", result.Impacts)
		}
		if impact.Node.ID == "node-6" && !reflect.DeepEqual(impact.Path, []string{SeedNodeID, "node-2", "node-6"}) {
			t.Fatalf("canonical equal-length path = %#v", impact.Path)
		}
	}
	resolved, err := repo.ResolvePinned(before, SeedNodeID)
	if err != nil {
		t.Fatalf("ResolvePinned: %v", err)
	}
	if resolved.Node.Title != "SPL walking skeleton" {
		t.Fatalf("hypothetical delta persisted: %#v", resolved.Node)
	}
}

func TestImpactUsesCanonicalPathsAndBoundsTraversal(t *testing.T) {
	repo := impactRepository(t)
	base, _ := repo.PinBranch("main")
	result, err := repo.Impact(ImpactRequest{
		Commit:   base,
		Delta:    []MutationOperation{{Action: "update", Entity: "node", ID: SeedNodeID, Title: "Changed"}},
		MaxDepth: 1, MaxVisited: 10,
	})
	if err != nil {
		t.Fatalf("Impact: %v", err)
	}
	if len(result.Impacts) != 3 {
		t.Fatalf("impacts = %#v", result.Impacts)
	}
	for _, impact := range result.Impacts {
		if impact.Node.ID == "node-4" {
			t.Fatalf("depth or visited bound failed: %#v", result.Impacts)
		}
	}
	if !reflect.DeepEqual(result.Impacts[2].Path, []string{SeedNodeID, "node-3"}) {
		t.Fatalf("canonical path = %#v", result.Impacts[2].Path)
	}
}

func TestImpactRejectsMissingDeltaAndInvalidBudget(t *testing.T) {
	repo := newTestSeedRepository(t)
	for _, request := range []ImpactRequest{
		{Commit: repo.branches["main"], MaxDepth: 1, MaxVisited: 1},
		{Commit: repo.branches["main"], Delta: []MutationOperation{{Action: "update", Entity: "node", ID: SeedNodeID, Title: "Changed"}}, MaxDepth: -1, MaxVisited: 1},
	} {
		_, err := repo.Impact(request)
		if !errors.Is(err, ErrMissingImpactDelta) && !errors.Is(err, ErrInvalidImpactBudget) {
			t.Fatalf("Impact(%#v) error = %v", request, err)
		}
	}
}

func TestImpactRejectsInvalidTypedProperties(t *testing.T) {
	repo := newTestSeedRepository(t)
	head, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}
	if _, err := repo.Impact(ImpactRequest{
		Commit: head,
		Delta: []MutationOperation{{
			Action: "update", Entity: "node", ID: SeedNodeID, Title: "Changed",
			Properties: map[string]PropertyValue{"invalid": FloatPropertyValue(math.NaN())},
		}},
		MaxDepth: 1, MaxVisited: 10,
	}); !errors.Is(err, ErrInvalidPropertyValue) {
		t.Fatalf("Impact invalid property error = %v, want ErrInvalidPropertyValue", err)
	}
}

func TestImpactSeedsBothEndpointsOfAnUpdatedEdge(t *testing.T) {
	repo := impactRepository(t)
	base, _ := repo.PinBranch("main")
	result, err := repo.Impact(ImpactRequest{
		Commit: base,
		Delta: []MutationOperation{
			{Action: "update", Entity: "edge", ID: "edge-1", Source: "node-3", Target: "node-4"},
		},
		MaxDepth: 0, MaxVisited: 10,
	})
	if err != nil {
		t.Fatalf("Impact: %v", err)
	}

	got := make([]string, len(result.Impacts))
	for i, impact := range result.Impacts {
		got[i] = impact.Node.ID
	}
	want := []string{SeedNodeID, "node-2", "node-3", "node-4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("updated-edge seeds = %#v, want %#v", got, want)
	}
}

func TestImpactPagesResponsesAndReportsVisitedCapacity(t *testing.T) {
	repo := impactRepository(t)
	base, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}
	request := ImpactRequest{
		Commit: base, Delta: []MutationOperation{{Action: "update", Entity: "node", ID: SeedNodeID, Title: "Changed"}},
		MaxDepth: 3, MaxVisited: 10, MaxRows: 1, MaxResponseBytes: 1 << 20,
	}
	first, err := repo.ImpactContext(context.Background(), request)
	if err != nil {
		t.Fatalf("ImpactContext first: %v", err)
	}
	if len(first.Impacts) != 1 || first.ContinuationToken == "" {
		t.Fatalf("first impact page = %#v", first)
	}
	request.ContinuationToken = first.ContinuationToken
	second, err := repo.ImpactContext(context.Background(), request)
	if err != nil || len(second.Impacts) != 1 {
		t.Fatalf("second impact page/error = %#v/%v", second, err)
	}
	request.MaxRows = 2
	if _, err := repo.ImpactContext(context.Background(), request); !errors.Is(err, ErrInvalidContinuation) {
		t.Fatalf("mismatched impact token error = %v", err)
	}
	if _, err := repo.ImpactContext(context.Background(), ImpactRequest{
		Commit: base, Delta: request.Delta, MaxDepth: 3, MaxVisited: 10, MaxRows: 1, MaxResponseBytes: 1,
	}); !errors.Is(err, ErrResponseBudgetTooSmall) {
		t.Fatalf("small impact budget error = %v", err)
	}
	capacity, err := repo.ImpactContext(context.Background(), ImpactRequest{
		Commit: base, Delta: request.Delta, MaxDepth: 3, MaxVisited: 2, MaxRows: 2, MaxResponseBytes: 1 << 20,
	})
	if err != nil || len(capacity.Impacts) != 2 || !capacity.CapacityExhausted {
		t.Fatalf("capacity-limited impact/error = %#v/%v", capacity, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := repo.ImpactContext(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled impact error = %v", err)
	}
}

func TestImpactContextReturnsPrefixWhenDeadlineFiresDuringTraversal(t *testing.T) {
	const length = 32
	repo := newTestSeedRepository(t)
	operations := make([]MutationOperation, 0, 2*length)
	previous := SeedNodeID
	for i := 0; i < length; i++ {
		nodeID := fmt.Sprintf("node-%02d", i)
		operations = append(operations,
			MutationOperation{Action: "add", Entity: "node", ID: nodeID, Title: nodeID},
			MutationOperation{Action: "add", Entity: "edge", ID: fmt.Sprintf("edge-%02d", i), Source: previous, Target: nodeID},
		)
		previous = nodeID
	}
	if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: operations}); err != nil {
		t.Fatalf("stage chain: %v", err)
	}
	if _, err := repo.CommitStagedMutations("main"); err != nil {
		t.Fatalf("commit chain: %v", err)
	}
	head, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("pin chain head: %v", err)
	}

	request := ImpactRequest{
		Commit: head,
		Delta: []MutationOperation{
			{Action: "update", Entity: "node", ID: SeedNodeID, Title: "Changed"},
		},
		MaxDepth: length, MaxVisited: length + 1, MaxRows: length + 1, MaxResponseBytes: 1 << 20,
	}
	result, err := repo.ImpactContext(&deadlineAfterChecks{remaining: 4*length + 6}, request)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ImpactContext deadline error = %v, want context.DeadlineExceeded", err)
	}
	if len(result.Impacts) != 1 || result.Impacts[0].Node.ID != SeedNodeID || result.ContinuationToken == "" {
		t.Fatalf("deadline impact prefix = %#v, want seed prefix with continuation", result)
	}
	request.ContinuationToken = result.ContinuationToken
	continued, err := repo.ImpactContext(context.Background(), request)
	if err != nil || len(continued.Impacts) == 0 || continued.Impacts[0].Node.ID != "node-00" {
		t.Fatalf("continued deadline impact = %#v/%v, want node-00 prefix", continued, err)
	}
}

func impactRepository(t *testing.T) *Repository {
	t.Helper()
	repo := newTestSeedRepository(t)
	operations := []MutationOperation{
		{Action: "add", Entity: "node", ID: "node-2", Title: "Second"},
		{Action: "add", Entity: "node", ID: "node-3", Title: "Third"},
		{Action: "add", Entity: "node", ID: "node-4", Title: "Fourth"},
		{Action: "add", Entity: "node", ID: "node-5", Title: "Inbound only"},
		{Action: "add", Entity: "node", ID: "node-6", Title: "Shared dependent"},
		{Action: "add", Entity: "edge", ID: "edge-1", Source: SeedNodeID, Target: "node-2"},
		{Action: "add", Entity: "edge", ID: "edge-2", Source: SeedNodeID, Target: "node-3"},
		{Action: "add", Entity: "edge", ID: "edge-3", Source: "node-2", Target: "node-3"},
		{Action: "add", Entity: "edge", ID: "edge-cycle", Source: "node-3", Target: SeedNodeID},
		{Action: "add", Entity: "edge", ID: "edge-4", Source: "node-3", Target: "node-4"},
		{Action: "add", Entity: "edge", ID: "edge-inbound", Source: "node-5", Target: SeedNodeID},
		{Action: "add", Entity: "edge", ID: "edge-5", Source: "node-2", Target: "node-6"},
		{Action: "add", Entity: "edge", ID: "edge-6", Source: "node-3", Target: "node-6"},
	}
	if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: operations}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	if _, err := repo.CommitStagedMutations("main"); err != nil {
		t.Fatalf("CommitStagedMutations: %v", err)
	}
	return repo
}
