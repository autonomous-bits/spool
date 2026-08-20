package repository

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSQLiteProjectionBuildsFromCanonicalBranchHead(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	closeTestRepository(t, repo)

	status, err := repo.ProjectionStatus()
	if err != nil {
		t.Fatalf("ProjectionStatus: %v", err)
	}
	head, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}
	if status.State != "ready" || status.Commit != head || status.Branch != "main" || status.SchemaVersion != projectionSchemaVersion {
		t.Fatalf("initial projection status = %#v", status)
	}

	schema := []byte(`
version = 2
[[node]]
label = "Seed"
[[node.property]]
key = "search"
required = true
indexed = true
types = ["string"]
[[node.property]]
key = "rank"
required = true
indexed = true
types = ["integer"]
[[node.property]]
key = "hidden"
required = true
types = ["string"]
`)
	_, err = repo.StageSchemaMigration(SchemaMigrationRequest{
		Branch: "main", SchemaTOML: schema,
		Operations: []MutationOperation{{
			Action: "update", Entity: "node", ID: SeedNodeID, Title: "Seed title", Labels: []string{"Seed"},
			Properties: map[string]PropertyValue{
				"search": StringPropertyValue("indexed prose"),
				"rank":   IntegerPropertyValue(7),
				"hidden": StringPropertyValue("not typed indexed"),
			},
		}},
	})
	if err != nil {
		t.Fatalf("StageSchemaMigration: %v", err)
	}
	committed, err := repo.CommitStagedMutationBatch(CommitStagedMutationRequest{Branch: "main"})
	if err != nil {
		t.Fatalf("CommitStagedMutationBatch: %v", err)
	}
	status, err = repo.ProjectionStatus()
	if err != nil {
		t.Fatalf("ProjectionStatus after commit: %v", err)
	}
	if status.Commit != committed.Commit || status.State != "ready" {
		t.Fatalf("committed projection status = %#v", status)
	}

	var textCount, numberCount, hiddenCount, ftsCount, changes int
	var createdCommit, updatedCommit ObjectID
	if err := repo.projectionDB.QueryRow(`SELECT count(*) FROM node_property_text WHERE property_key='search' AND property_value='indexed prose'`).Scan(&textCount); err != nil {
		t.Fatalf("query text index: %v", err)
	}
	if err := repo.projectionDB.QueryRow(`SELECT count(*) FROM node_property_number WHERE property_key='rank' AND property_value=7`).Scan(&numberCount); err != nil {
		t.Fatalf("query number index: %v", err)
	}
	if err := repo.projectionDB.QueryRow(`SELECT count(*) FROM node_property_text WHERE property_key='hidden'`).Scan(&hiddenCount); err != nil {
		t.Fatalf("query hidden index: %v", err)
	}
	if err := repo.projectionDB.QueryRow(`SELECT count(*) FROM node_fts WHERE node_fts MATCH 'indexed prose'`).Scan(&ftsCount); err != nil {
		t.Fatalf("query FTS: %v", err)
	}
	if err := repo.projectionDB.QueryRow(`SELECT count(*) FROM entity_changes WHERE commit_oid=?`, committed.Commit).Scan(&changes); err != nil {
		t.Fatalf("query entity changes: %v", err)
	}
	if err := repo.projectionDB.QueryRow(`SELECT created_commit, updated_commit FROM nodes WHERE node_id=?`, SeedNodeID).Scan(&createdCommit, &updatedCommit); err != nil {
		t.Fatalf("query node lifecycle: %v", err)
	}
	if textCount != 1 || numberCount != 1 || hiddenCount != 0 || ftsCount != 1 || changes != 1 {
		t.Fatalf("projection counts text=%d number=%d hidden=%d fts=%d changes=%d", textCount, numberCount, hiddenCount, ftsCount, changes)
	}
	if createdCommit == committed.Commit || updatedCommit != committed.Commit {
		t.Fatalf("node lifecycle created=%s updated=%s commit=%s", createdCommit, updatedCommit, committed.Commit)
	}
}

func TestSQLiteProjectionRecoversFromCorruptDatabase(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "graph.db"), []byte("not a SQLite database"), 0o600); err != nil {
		t.Fatalf("corrupt graph.db: %v", err)
	}
	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository recovery: %v", err)
	}
	closeTestRepository(t, reopened)
	status, err := reopened.ProjectionStatus()
	if err != nil {
		t.Fatalf("ProjectionStatus: %v", err)
	}
	if status.State != "ready" {
		t.Fatalf("projection state = %q, want ready", status.State)
	}
}

func TestSQLiteProjectionRejectsHistoricalCommit(t *testing.T) {
	repo := NewSeedRepository()
	other := ObjectID("different")
	if _, err := repo.EnsureBranchHeadProjection("main", &other); !errors.Is(err, ErrHistoricalProjectionUnsupported) {
		t.Fatalf("EnsureBranchHeadProjection error = %v, want ErrHistoricalProjectionUnsupported", err)
	}
}
