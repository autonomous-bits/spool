package repository

import (
	"errors"
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
	if resolved.Node.Title != "EDG walking skeleton" {
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
	repo := NewSeedRepository()
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
	repo := NewSeedRepository()
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

func impactRepository(t *testing.T) *Repository {
	t.Helper()
	repo := NewSeedRepository()
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
