package repository

import (
	"errors"
	"reflect"
	"testing"
)

const peopleSchemaTOML = `
version = 2

[[node]]
label = "Person"
[[node.property]]
key = "name"
required = true
types = ["string"]

[[edge]]
type = "KNOWS"
source_labels = ["Person"]
target_labels = ["Person"]
[edge.cardinality]
source_max = 3
target_max = 3
`

func peopleMigrationOperations() []MutationOperation {
	return []MutationOperation{
		{
			Action: "update", Entity: "node", ID: SeedNodeID, Title: "Alice",
			Labels: []string{"Person"}, Properties: map[string]PropertyValue{"name": StringPropertyValue("Alice")},
		},
		{
			Action: "add", Entity: "node", ID: "person-2", Title: "Bob",
			Labels: []string{"Person"}, Properties: map[string]PropertyValue{"name": StringPropertyValue("Bob")},
		},
		{Action: "add", Entity: "edge", ID: "knows-1", Source: SeedNodeID, Target: "person-2", Type: "KNOWS"},
	}
}

func TestStageSchemaMigrationAtomicallyCommitsTargetSchemaAndGraph(t *testing.T) {
	repo := newTestSeedRepository(t)
	oldCommit := repo.branches["main"]
	oldRoot := repo.snapshots[repo.commits[oldCommit].Snapshot].SchemaRoot
	target, err := DecodeSchemaTOML([]byte(peopleSchemaTOML))
	if err != nil {
		t.Fatalf("DecodeSchemaTOML: %v", err)
	}

	staged, err := repo.StageSchemaMigration(SchemaMigrationRequest{
		Branch: "main", SchemaTOML: []byte(peopleSchemaTOML), Operations: peopleMigrationOperations(),
	})
	if err != nil {
		t.Fatalf("StageSchemaMigration: %v", err)
	}
	if staged.BaseCommit != oldCommit || !reflect.DeepEqual(repo.stagedMutations["main"].TargetSchema, &target) {
		t.Fatalf("staged migration = %#v", repo.stagedMutations["main"])
	}
	committed, err := repo.CommitStagedMutationBatch(CommitStagedMutationRequest{Branch: "main", Message: "migrate people schema"})
	if err != nil {
		t.Fatalf("CommitStagedMutationBatch: %v", err)
	}
	snapshot := repo.snapshots[repo.commits[committed.Commit].Snapshot]
	if snapshot.SchemaRoot == oldRoot || snapshot.SchemaRoot != repo.objectID("schema-root", target) {
		t.Fatalf("target SchemaRoot = %q, want canonical target distinct from %q", snapshot.SchemaRoot, oldRoot)
	}
	if node := repo.projections[snapshot.NodeRoot]["person-2"]; !node.Equal(Node{
		ID: "person-2", Title: "Bob", Labels: []string{"Person"}, Properties: map[string]PropertyValue{"name": StringPropertyValue("Bob")},
	}) {
		t.Fatalf("migrated node = %#v", node)
	}
	if edge := repo.edgeProjections[repo.commits[committed.Commit].Snapshot]["knows-1"]; !edge.Equal(Edge{
		ID: "knows-1", Source: SeedNodeID, Target: "person-2", Type: "KNOWS",
	}) {
		t.Fatalf("migrated edge = %#v", edge)
	}
	if got := repo.snapshots[repo.commits[oldCommit].Snapshot].SchemaRoot; got != oldRoot {
		t.Fatalf("old history SchemaRoot = %q, want %q", got, oldRoot)
	}
}

func TestStageSchemaMigrationRejectsInvalidCandidateWithoutReplacingStaging(t *testing.T) {
	repo := newTestSeedRepository(t)
	if _, err := repo.StageMutationBatch(StageMutationRequest{Branch: "main", Operations: validMutationBatch()}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	before := cloneStagedMutations(repo.stagedMutations)["main"]
	invalid := []MutationOperation{
		{
			Action: "add", Entity: "node", ID: "person-2", Title: "Bob",
			Labels: []string{"Person"}, Properties: map[string]PropertyValue{},
		},
	}
	if _, err := repo.StageSchemaMigration(SchemaMigrationRequest{
		Branch: "main", SchemaTOML: []byte(peopleSchemaTOML), Operations: invalid,
	}); !errors.Is(err, ErrSchemaValidation) {
		t.Fatalf("StageSchemaMigration error = %v, want ErrSchemaValidation", err)
	}
	if got := repo.stagedMutations["main"]; !reflect.DeepEqual(got, before) {
		t.Fatalf("staged set = %#v, want preserved %#v", got, before)
	}
}

func TestSchemaMigrationRollbackAndDurability(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("NewSeedRepositoryWithMergeState: %v", err)
	}
	if _, err := repo.StageSchemaMigration(SchemaMigrationRequest{
		Branch: "main", SchemaTOML: []byte(peopleSchemaTOML), Operations: peopleMigrationOperations(),
	}); err != nil {
		t.Fatalf("StageSchemaMigration: %v", err)
	}
	staged := cloneStagedMutations(repo.stagedMutations)["main"]
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository staged migration: %v", err)
	}
	if got := reopened.stagedMutations["main"]; !reflect.DeepEqual(got, staged) {
		t.Fatalf("reopened staged migration = %#v, want %#v", got, staged)
	}
	beforeHead, beforeObjects, beforeSnapshots := reopened.branches["main"], len(reopened.objects), len(reopened.snapshots)
	reopened.persistRepositoryFn = func() error { return errors.New("injected persistence failure") }
	if _, err := reopened.CommitStagedMutations("main"); err == nil {
		t.Fatal("CommitStagedMutations succeeded despite persistence failure")
	}
	if reopened.branches["main"] != beforeHead || len(reopened.objects) != beforeObjects || len(reopened.snapshots) != beforeSnapshots {
		t.Fatal("failed migration commit changed durable state")
	}
	if got := reopened.stagedMutations["main"]; !reflect.DeepEqual(got, staged) {
		t.Fatalf("failed migration commit lost staging: %#v", got)
	}
	reopened.persistRepositoryFn = nil
	committed, err := reopened.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("CommitStagedMutations after rollback: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("Close after commit: %v", err)
	}
	final, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository committed migration: %v", err)
	}
	closeTestRepository(t, final)
	snapshot := final.snapshots[final.commits[committed.Commit].Snapshot]
	if _, err := final.schemaSnapshotLocked(snapshot.SchemaRoot); err != nil {
		t.Fatalf("reopened target schema: %v", err)
	}
}

func TestOrdinaryStagingAndCommitRevalidateCurrentSchema(t *testing.T) {
	repo := newTestSeedRepository(t)
	if _, err := repo.StageSchemaMigration(SchemaMigrationRequest{
		Branch: "main", SchemaTOML: []byte(peopleSchemaTOML), Operations: peopleMigrationOperations(),
	}); err != nil {
		t.Fatalf("StageSchemaMigration: %v", err)
	}
	if _, err := repo.CommitStagedMutations("main"); err != nil {
		t.Fatalf("CommitStagedMutations migration: %v", err)
	}
	if _, err := repo.StageMutationBatch(StageMutationRequest{
		Branch:     "main",
		Operations: []MutationOperation{{Action: "add", Entity: "node", ID: "invalid-person", Title: "Invalid", Labels: []string{"Person"}}},
	}); !errors.Is(err, ErrSchemaValidation) {
		t.Fatalf("StageMutationBatch error = %v, want ErrSchemaValidation", err)
	}
	if _, err := repo.StageMutationBatch(StageMutationRequest{
		Branch: "main",
		Operations: []MutationOperation{{
			Action: "add", Entity: "node", ID: "person-3", Title: "Carol",
			Labels: []string{"Person"}, Properties: map[string]PropertyValue{"name": StringPropertyValue("Carol")},
		}},
	}); err != nil {
		t.Fatalf("StageMutationBatch valid: %v", err)
	}
	head := repo.branches["main"]
	snapshot := repo.snapshots[repo.commits[head].Snapshot]
	seed := repo.projections[snapshot.NodeRoot][SeedNodeID]
	seed.Properties = nil
	repo.projections[snapshot.NodeRoot][SeedNodeID] = seed
	if _, err := repo.CommitStagedMutations("main"); !errors.Is(err, ErrSchemaValidation) {
		t.Fatalf("CommitStagedMutations error = %v, want ErrSchemaValidation", err)
	}
	if repo.branches["main"] != head {
		t.Fatal("failed schema revalidation advanced branch")
	}
}
