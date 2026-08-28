package repository

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestPruneProtectedDefaultBranchWithoutForceFails(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "repo")
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	defer func() { _ = repo.Close() }()

	_, err = repo.Prune(PruneRequest{
		Branch: "main",
		Force:  false,
	})
	if !errors.Is(err, ErrProtectedBranch) {
		t.Fatalf("expected ErrProtectedBranch, got %v", err)
	}
}

func TestPruneMissingBranchReturnsErrBranchNotFound(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "repo")
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	defer func() { _ = repo.Close() }()

	_, err = repo.Prune(PruneRequest{
		Branch: "non-existent-branch",
		Force:  true,
	})
	if !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("expected ErrBranchNotFound, got %v", err)
	}
}

func TestPruneWithUncommittedStagedChangesFails(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "repo")
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	defer func() { _ = repo.Close() }()

	_, err = repo.StageMutationBatch(StageMutationRequest{
		Branch: "main",
		Operations: []MutationOperation{
			{Action: "add", Entity: "node", ID: "temp-1", Title: "Temp", Labels: []string{"Architecture", "Ephemeral"}},
		},
	})
	if err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}

	_, err = repo.Prune(PruneRequest{
		Branch: "main",
		Force:  true,
	})
	if !errors.Is(err, ErrUncommittedStagedChanges) {
		t.Fatalf("expected ErrUncommittedStagedChanges, got %v", err)
	}
}

func TestPruneWithSchemaOnlyStagedMigrationFails(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "repo")
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	defer func() { _ = repo.Close() }()

	schemaTOML := []byte(`
version = 1
permissive = true
`)
	_, err = repo.StageSchemaMigration(SchemaMigrationRequest{
		Branch:     "main",
		SchemaTOML: schemaTOML,
		Operations: nil,
	})
	if err != nil {
		t.Fatalf("StageSchemaMigration: %v", err)
	}

	_, err = repo.Prune(PruneRequest{
		Branch: "main",
		Force:  true,
	})
	if !errors.Is(err, ErrUncommittedStagedChanges) {
		t.Fatalf("expected ErrUncommittedStagedChanges for schema-only migration, got %v", err)
	}
}

func TestPruneZeroMatchIdempotentNoOp(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "repo")
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	defer func() { _ = repo.Close() }()

	headBefore := repo.branches["main"]
	res, err := repo.Prune(PruneRequest{
		Branch: "main",
		Force:  true,
	})
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if res.PrunedNodesCount != 0 || res.PrunedEdgesCount != 0 {
		t.Fatalf("expected 0 pruned, got %d nodes, %d edges", res.PrunedNodesCount, res.PrunedEdgesCount)
	}
	if len(res.PrunedNodeIDs) != 0 || len(res.OrphanedDurableNodes) != 0 {
		t.Fatalf("expected empty slices, got nodes=%v orphans=%v", res.PrunedNodeIDs, res.OrphanedDurableNodes)
	}
	if res.Commit != string(headBefore) {
		t.Fatalf("expected commit %q, got %q", headBefore, res.Commit)
	}
	if repo.branches["main"] != headBefore {
		t.Fatalf("expected branch head unchanged at %q, got %q", headBefore, repo.branches["main"])
	}
}

func TestPruneDryRunSimulation(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "repo")
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	defer func() { _ = repo.Close() }()

	// Add durable nodes, ephemeral nodes, and connecting edges
	_, err = repo.StageMutationBatch(StageMutationRequest{
		Branch: "main",
		Operations: []MutationOperation{
			{Action: "add", Entity: "node", ID: "durable-1", Title: "Durable 1", Labels: []string{"Architecture", "Component"}},
			{Action: "add", Entity: "node", ID: "durable-2", Title: "Durable 2", Labels: []string{"Architecture", "Component"}},
			{Action: "add", Entity: "node", ID: "durable-orphan", Title: "Durable Orphan", Labels: []string{"Architecture", "Component"}},
			{Action: "add", Entity: "node", ID: "ephemeral-1", Title: "Ephemeral 1", Labels: []string{"Architecture", "Ephemeral"}},
			{Action: "add", Entity: "edge", ID: "edge-d1-to-d2", Source: "durable-1", Target: "durable-2", Type: "DEPENDS_ON"},
			{Action: "add", Entity: "edge", ID: "edge-durable-to-eph", Source: "durable-1", Target: "ephemeral-1", Type: "DEPENDS_ON"},
			{Action: "add", Entity: "edge", ID: "edge-eph-to-orphan", Source: "ephemeral-1", Target: "durable-orphan", Type: "DEPENDS_ON"},
		},
	})
	if err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	commitRes, err := repo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("CommitStagedMutations: %v", err)
	}

	res, err := repo.Prune(PruneRequest{
		Branch: "main",
		Force:  true,
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("Prune (DryRun): %v", err)
	}
	if !res.DryRun {
		t.Fatal("expected DryRun = true")
	}
	if res.PrunedNodesCount != 1 || len(res.PrunedNodeIDs) != 1 || res.PrunedNodeIDs[0] != "ephemeral-1" {
		t.Fatalf("unexpected pruned nodes: %+v", res)
	}
	if res.PrunedEdgesCount != 2 {
		t.Fatalf("expected 2 pruned edges, got %d", res.PrunedEdgesCount)
	}
	if len(res.OrphanedDurableNodes) != 1 || res.OrphanedDurableNodes[0] != "durable-orphan" {
		t.Fatalf("expected durable-orphan, got %v", res.OrphanedDurableNodes)
	}
	if repo.branches["main"] != commitRes.Commit {
		t.Fatalf("expected branch head unchanged from %q, got %q", commitRes.Commit, repo.branches["main"])
	}
}

func TestPruneExecutionExcisesEphemeralAndCascadesEdges(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "repo")
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	defer func() { _ = repo.Close() }()

	// Add durable nodes, ephemeral nodes, and connecting edges
	_, err = repo.StageMutationBatch(StageMutationRequest{
		Branch: "main",
		Operations: []MutationOperation{
			{Action: "add", Entity: "node", ID: "durable-1", Title: "Durable 1", Labels: []string{"Architecture", "Component"}},
			{Action: "add", Entity: "node", ID: "durable-2", Title: "Durable 2", Labels: []string{"Architecture", "Component"}},
			{Action: "add", Entity: "node", ID: "durable-orphan", Title: "Durable Orphan", Labels: []string{"Architecture", "Component"}},
			{Action: "add", Entity: "node", ID: "ephemeral-1", Title: "Ephemeral 1", Labels: []string{"Architecture", "Ephemeral"}},
			{Action: "add", Entity: "node", ID: "ephemeral-2", Title: "Ephemeral 2", Labels: []string{"Architecture", "Ephemeral"}},
			{Action: "add", Entity: "edge", ID: "edge-d1-to-d2", Source: "durable-1", Target: "durable-2", Type: "DEPENDS_ON"},
			{Action: "add", Entity: "edge", ID: "edge-d1-to-e1", Source: "durable-1", Target: "ephemeral-1", Type: "DEPENDS_ON"},
			{Action: "add", Entity: "edge", ID: "edge-e1-to-e2", Source: "ephemeral-1", Target: "ephemeral-2", Type: "DEPENDS_ON"},
			{Action: "add", Entity: "edge", ID: "edge-e2-to-orphan", Source: "ephemeral-2", Target: "durable-orphan", Type: "DEPENDS_ON"},
		},
	})
	if err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	commitRes, err := repo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("CommitStagedMutations: %v", err)
	}

	pruneRes, err := repo.Prune(PruneRequest{
		Branch:  "main",
		Force:   true,
		DryRun:  false,
		Author:  "bob",
		Message: "Pruning temporary scaffold",
	})
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}

	if pruneRes.PrunedNodesCount != 2 {
		t.Fatalf("expected 2 pruned nodes, got %d", pruneRes.PrunedNodesCount)
	}
	if pruneRes.PrunedEdgesCount != 3 {
		t.Fatalf("expected 3 pruned edges, got %d", pruneRes.PrunedEdgesCount)
	}
	if len(pruneRes.OrphanedDurableNodes) != 1 || pruneRes.OrphanedDurableNodes[0] != "durable-orphan" {
		t.Fatalf("expected [durable-orphan], got %v", pruneRes.OrphanedDurableNodes)
	}
	if pruneRes.Commit == string(commitRes.Commit) {
		t.Fatalf("expected new commit ID, got same as before %q", pruneRes.Commit)
	}

	// Verify post-prune state
	newHead := repo.branches["main"]
	if string(newHead) != pruneRes.Commit {
		t.Fatalf("branch head = %q, want %q", newHead, pruneRes.Commit)
	}
	snapshotID := repo.commits[newHead].Snapshot
	snapshot := repo.snapshots[snapshotID]
	nodes := repo.projections[snapshot.NodeRoot]
	edges := repo.edgeProjections[snapshotID]

	// Ephemeral nodes must not exist
	if _, exists := nodes["ephemeral-1"]; exists {
		t.Fatal("ephemeral-1 should have been excised")
	}
	if _, exists := nodes["ephemeral-2"]; exists {
		t.Fatal("ephemeral-2 should have been excised")
	}

	// Durable nodes must still exist
	if _, exists := nodes["durable-1"]; !exists {
		t.Fatal("durable-1 should remain")
	}
	if _, exists := nodes["durable-2"]; !exists {
		t.Fatal("durable-2 should remain")
	}
	if _, exists := nodes["durable-orphan"]; !exists {
		t.Fatal("durable-orphan should remain")
	}

	// Only edge-d1-to-d2 should remain
	if _, exists := edges["edge-d1-to-d2"]; !exists {
		t.Fatal("edge-d1-to-d2 should remain")
	}
	if _, exists := edges["edge-d1-to-e1"]; exists {
		t.Fatal("edge-d1-to-e1 should have been cascaded")
	}
	if _, exists := edges["edge-e1-to-e2"]; exists {
		t.Fatal("edge-e1-to-e2 should have been cascaded")
	}
	if _, exists := edges["edge-e2-to-orphan"]; exists {
		t.Fatal("edge-e2-to-orphan should have been cascaded")
	}

	// Validate schema conformance (zero dangling edge references)
	schema, err := repo.schemaSnapshotLocked(snapshot.SchemaRoot)
	if err != nil {
		t.Fatalf("schemaSnapshotLocked: %v", err)
	}
	if err := ValidateSchemaSnapshot(schema, nodes, edges); err != nil {
		t.Fatalf("ValidateSchemaSnapshot failed: %v", err)
	}
}
