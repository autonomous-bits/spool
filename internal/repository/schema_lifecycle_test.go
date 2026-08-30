package repository

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenRepositoryIgnoresUnreachableSchemaMismatchedSnapshot(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}

	addSchemaLifecycleSnapshot(t, repo, schemaLifecyclePeopleSchema(t), map[string]Node{
		SeedNodeID: {ID: SeedNodeID, Title: "Alice", Labels: []string{"Person"}},
	}, nil)
	if err := repo.persistRepositoryLocked(); err != nil {
		t.Fatalf("persistRepositoryLocked: %v", err)
	}
	statePath := filepath.Join(stateDir, "refs", "heads", "main")
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile before open: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository rejected an unreachable snapshot: %v", err)
	}
	closeTestRepository(t, reopened)
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile after open: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("OpenRepository mutated durable control state")
	}
}

func TestOpenRepositoryAcceptsHistoricalPermissiveDataOutsideNewIngestionLimits(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}

	base := repo.branches["main"]
	baseSnapshot := repo.snapshots[repo.commits[base].Snapshot]
	nodes := cloneNodes(repo.projections[baseSnapshot.NodeRoot])
	nodes[SeedNodeID] = Node{
		ID:         SeedNodeID,
		Title:      "historical node",
		Labels:     []string{"legacy/label"},
		Properties: map[string]PropertyValue{"legacy/key": StringPropertyValue("value")},
	}
	snapshot, err := repo.materializeSnapshotLocked(nodes, map[string]Edge{}, baseSnapshot.SchemaRoot)
	if err != nil {
		t.Fatalf("materializeSnapshotLocked: %v", err)
	}
	snapshotID := repo.store("graph-snapshot", snapshot)
	repo.snapshots[snapshotID] = snapshot
	repo.projections[snapshot.NodeRoot] = nodes
	repo.edgeProjections[snapshotID] = map[string]Edge{}
	next := repo.newCommit(snapshotID, []ObjectID{base}, "", "legacy graph data")
	nextID := repo.store("commit", next)
	repo.commits[nextID], repo.branches["main"] = next, nextID
	if err := repo.persistRepositoryLocked(); err != nil {
		t.Fatalf("persistRepositoryLocked: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}
	closeTestRepository(t, reopened)
}

func TestApplyCleanBoundMergeRejectsSchemaInvalidTargetSnapshot(t *testing.T) {
	repo := newTestSeedRepository(t)
	base := repo.branches["main"]
	source := commit{Snapshot: repo.commits[base].Snapshot, Parents: []ObjectID{base}, Message: "feature change"}
	sourceID := repo.store("commit", source)
	repo.commits[sourceID] = source
	repo.branches["feature"] = sourceID

	invalidSnapshot := addSchemaLifecycleSnapshot(t, repo, schemaLifecyclePeopleSchema(t), map[string]Node{
		SeedNodeID: {ID: SeedNodeID, Title: "Alice", Labels: []string{"Person"}},
	}, nil)
	target := commit{Snapshot: invalidSnapshot, Parents: []ObjectID{base}, Message: "target change"}
	targetID := repo.store("commit", target)
	repo.commits[targetID] = target
	repo.branches["main"] = targetID
	initialCommitCount := len(repo.commits)

	_, err := repo.ApplyCleanBoundMerge("feature", "main", "owner", MergePreviewBinding{
		MergeBase: base, SourceCommit: sourceID, TargetCommit: targetID,
	})
	assertSchemaLifecycleValidationError(t, err)
	if got := repo.branches["main"]; got != targetID {
		t.Fatalf("main head = %q, want unchanged %q", got, targetID)
	}
	if got := len(repo.commits); got != initialCommitCount {
		t.Fatalf("commit count = %d, want unchanged %d", got, initialCommitCount)
	}
	if _, held := repo.mergeLeases["main"]; held {
		t.Fatal("schema-rejected clean merge acquired a lease")
	}
}

func TestFinalizeMergeTransactionRejectsSchemaInvalidStagedSnapshot(t *testing.T) {
	repo := newTestSeedRepository(t)
	base, source, target := createDivergedBranchHeads(repo)
	binding := MergePreviewBinding{MergeBase: base, SourceCommit: source, TargetCommit: target}
	if err := repo.ApplyConflictedBoundMerge("feature", "main", "owner", binding); !errors.Is(err, ErrMergeConflicted) {
		t.Fatalf("ApplyConflictedBoundMerge: %v", err)
	}

	stagedSnapshot := addSchemaLifecycleSnapshot(t, repo, schemaLifecyclePeopleSchema(t), map[string]Node{
		SeedNodeID: {ID: SeedNodeID, Title: "Alice", Labels: []string{"Person"}},
	}, nil)
	if err := repo.ResolveMergeTransaction("main", "owner", stagedSnapshot); err != nil {
		t.Fatalf("ResolveMergeTransaction: %v", err)
	}
	if err := repo.RestageMergeTransaction("main", "owner"); err != nil {
		t.Fatalf("RestageMergeTransaction: %v", err)
	}
	initialCommitCount := len(repo.commits)

	_, err := repo.FinalizeMergeTransaction("main", "owner")
	assertSchemaLifecycleValidationError(t, err)
	if got := repo.branches["main"]; got != target {
		t.Fatalf("main head = %q, want unchanged %q", got, target)
	}
	if got := len(repo.commits); got != initialCommitCount {
		t.Fatalf("commit count = %d, want unchanged %d", got, initialCommitCount)
	}
	if _, active := repo.mergeTransactions["main"]; !active {
		t.Fatal("schema-rejected finalization discarded its transaction")
	}
	if got := repo.mergeLeases["main"]; got != "owner" {
		t.Fatalf("lease owner = %q, want owner", got)
	}
}

func schemaLifecyclePeopleSchema(t *testing.T) SchemaSnapshot {
	t.Helper()
	schema, err := (SchemaSnapshot{
		Version: 2,
		NodeRules: []NodeLabelRule{{
			Label: "Person",
			Properties: []PropertyRule{{
				Key: "name", Required: true, Types: []PropertyKind{PropertyString},
			}},
		}},
	}).Normalize()
	if err != nil {
		t.Fatalf("normalize schema: %v", err)
	}
	return schema
}

func addSchemaLifecycleSnapshot(t *testing.T, repo *Repository, schema SchemaSnapshot, nodes map[string]Node, edges map[string]Edge) ObjectID {
	t.Helper()
	schemaRoot := repo.store("schema-root", schema)
	canonicalNodes, canonicalEdges := cloneNodes(nodes), cloneEdges(edges)
	snapshot, err := repo.materializeSnapshotLocked(canonicalNodes, canonicalEdges, schemaRoot)
	if err != nil {
		t.Fatalf("materializeSnapshotLocked: %v", err)
	}
	snapshotID := repo.store("graph-snapshot", snapshot)
	repo.snapshots[snapshotID] = snapshot
	repo.projections[snapshot.NodeRoot] = canonicalNodes
	repo.edgeProjections[snapshotID] = canonicalEdges
	return snapshotID
}

func assertSchemaLifecycleValidationError(t *testing.T, err error) {
	t.Helper()
	var validation *SchemaValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want *SchemaValidationError", err)
	}
	if len(validation.Violations) == 0 {
		t.Fatal("schema validation error has no violations")
	}
}
