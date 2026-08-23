package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const projectionSchemaVersion = 1

var (
	// ErrHistoricalProjectionUnsupported reports a request that cannot be served
	// by the branch-head-only projection cache.
	ErrHistoricalProjectionUnsupported = errors.New("historical snapshot projections are unsupported")
	// ErrProjectionUnavailable reports a projection that could not be made ready.
	ErrProjectionUnavailable = errors.New("projection is unavailable")
)

// ProjectionStatus describes the private SQLite projection used by future read surfaces.
type ProjectionStatus struct {
	SchemaVersion int      `json:"schemaVersion"`
	State         string   `json:"state"`
	Branch        string   `json:"branch"`
	Commit        ObjectID `json:"commit"`
	NodeRoot      ObjectID `json:"nodeRoot"`
}

func (r *Repository) projectionPath() string {
	return filepath.Join(r.mergeStateDir, "graph.db")
}

func (r *Repository) projectionRepositoryID() string {
	sum := sha256.Sum256([]byte(r.mergeStateDir))
	return hex.EncodeToString(sum[:])
}

// RepositoryID returns the stable identifier for this repository's projection
// namespace without exposing its storage location or SQLite tables.
func (r *Repository) RepositoryID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.projectionRepositoryID()
}

func (r *Repository) openProjectionLocked() error {
	if r.mergeStateDir == "" || r.projectionDB != nil {
		return nil
	}
	db, err := sql.Open("sqlite", r.projectionPath())
	if err != nil {
		return fmt.Errorf("open SQLite projection: %w", err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA synchronous=FULL;`); err != nil {
		_ = db.Close()
		return fmt.Errorf("configure SQLite projection: %w", err)
	}
	if err := createProjectionSchema(db); err != nil {
		_ = db.Close()
		return err
	}
	r.projectionDB = db
	return nil
}

func createProjectionSchema(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS index_meta (
    repository_id TEXT PRIMARY KEY,
    schema_version INTEGER NOT NULL,
    projected_branch TEXT NOT NULL,
    projected_commit TEXT NOT NULL,
    node_root TEXT NOT NULL,
    projection_state TEXT NOT NULL CHECK (projection_state IN ('building', 'ready', 'failed')),
    projected_at_us INTEGER NOT NULL,
    failure_reason TEXT NOT NULL DEFAULT ''
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS nodes (
    node_id TEXT PRIMARY KEY,
    snapshot_oid TEXT NOT NULL,
    properties_json TEXT NOT NULL,
    created_commit TEXT NOT NULL,
    updated_commit TEXT NOT NULL
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS node_labels (
    label TEXT NOT NULL,
    node_id TEXT NOT NULL,
    PRIMARY KEY (label, node_id),
    FOREIGN KEY (node_id) REFERENCES nodes(node_id) ON DELETE CASCADE
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS node_labels_by_node ON node_labels(node_id, label);
CREATE TABLE IF NOT EXISTS edges (
    edge_id TEXT PRIMARY KEY,
    snapshot_oid TEXT NOT NULL,
    edge_type TEXT NOT NULL,
    from_node TEXT NOT NULL,
    to_node TEXT NOT NULL,
    properties_json TEXT NOT NULL,
    created_commit TEXT NOT NULL,
    updated_commit TEXT NOT NULL
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS edges_out_covering ON edges(from_node, edge_type, to_node, edge_id);
CREATE INDEX IF NOT EXISTS edges_in_covering ON edges(to_node, edge_type, from_node, edge_id);
CREATE TABLE IF NOT EXISTS node_property_text (
    property_key TEXT NOT NULL,
    property_value TEXT NOT NULL,
    node_id TEXT NOT NULL,
    PRIMARY KEY (property_key, property_value, node_id)
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS node_property_number (
    property_key TEXT NOT NULL,
    property_value REAL NOT NULL,
    node_id TEXT NOT NULL,
    PRIMARY KEY (property_key, property_value, node_id)
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS commits (
    commit_oid TEXT PRIMARY KEY,
    graph_root_oid TEXT NOT NULL,
    schema_root_oid TEXT NOT NULL,
    author TEXT NOT NULL,
    committed_at_us INTEGER NOT NULL,
    message TEXT NOT NULL
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS commit_parents (
    commit_oid TEXT NOT NULL,
    parent_index INTEGER NOT NULL,
    parent_oid TEXT NOT NULL,
    PRIMARY KEY (commit_oid, parent_index)
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS entity_changes (
    entity_kind TEXT NOT NULL CHECK (entity_kind IN ('node', 'edge')),
    entity_id TEXT NOT NULL,
    commit_oid TEXT NOT NULL,
    change_kind TEXT NOT NULL CHECK (change_kind IN ('add', 'modify', 'delete')),
    before_oid TEXT,
    after_oid TEXT,
    PRIMARY KEY (entity_kind, entity_id, commit_oid)
) WITHOUT ROWID;
CREATE VIRTUAL TABLE IF NOT EXISTS node_fts USING fts5(
    node_id UNINDEXED, title, body, labels, tags,
    tokenize = 'unicode61 remove_diacritics 2'
);`)
	if err != nil {
		return fmt.Errorf("create SQLite projection schema: %w", err)
	}
	return nil
}

// ProjectionStatus returns the cached projection metadata without exposing physical tables.
func (r *Repository) ProjectionStatus() (ProjectionStatus, error) {
	return r.ProjectionStatusContext(context.Background())
}

// ProjectionStatusContext returns cached projection metadata while honoring
// cancellation around the repository read.
func (r *Repository) ProjectionStatusContext(ctx context.Context) (ProjectionStatus, error) {
	if err := ctx.Err(); err != nil {
		return ProjectionStatus{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return ProjectionStatus{}, err
	}
	if err := r.ensureOpenLocked(); err != nil {
		return ProjectionStatus{}, err
	}
	return r.projectionStatusLocked()
}

func (r *Repository) projectionStatusLocked() (ProjectionStatus, error) {
	if r.projectionDB == nil {
		return ProjectionStatus{}, ErrProjectionUnavailable
	}
	var status ProjectionStatus
	err := r.projectionDB.QueryRow(`
SELECT schema_version, projection_state, projected_branch, projected_commit, node_root
FROM index_meta WHERE repository_id = ?`, r.projectionRepositoryID()).
		Scan(&status.SchemaVersion, &status.State, &status.Branch, &status.Commit, &status.NodeRoot)
	if errors.Is(err, sql.ErrNoRows) {
		return ProjectionStatus{}, ErrProjectionUnavailable
	}
	if err != nil {
		return ProjectionStatus{}, fmt.Errorf("read projection metadata: %w", err)
	}
	return status, nil
}

// EnsureBranchHeadProjection rebuilds the projection for branch's pinned head.
// Non-head commits deliberately remain unsupported until historical projections land.
func (r *Repository) EnsureBranchHeadProjection(branch string, commit *ObjectID) (ProjectionStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return ProjectionStatus{}, err
	}
	head, ok := r.branches[branch]
	if !ok {
		return ProjectionStatus{}, ErrBranchNotFound
	}
	if commit != nil && *commit != head {
		return ProjectionStatus{}, ErrHistoricalProjectionUnsupported
	}
	if err := r.ensureProjectionForBranchLocked(branch, head); err != nil {
		return ProjectionStatus{}, err
	}
	return r.projectionStatusLocked()
}

func (r *Repository) ensureProjectionForActiveBranchLocked() error {
	head, ok := r.branches[r.activeBranch]
	if !ok {
		return ErrBranchNotFound
	}
	return r.ensureProjectionForBranchLocked(r.activeBranch, head)
}

func (r *Repository) ensureProjectionForBranchLocked(branch string, commitID ObjectID) error {
	if r.mergeStateDir == "" {
		return nil
	}
	if err := r.openProjectionLocked(); err != nil {
		if removeErr := removeProjectionFiles(r.mergeStateDir); removeErr != nil {
			return errors.Join(err, fmt.Errorf("remove unusable projection: %w", removeErr))
		}
		if err := r.openProjectionLocked(); err != nil {
			return err
		}
	}
	snapshotID := r.commits[commitID].Snapshot
	if err := r.ensureSnapshotProjectionLocked(snapshotID); err != nil {
		return err
	}
	snapshot := r.snapshots[snapshotID]
	status, err := r.projectionStatusLocked()
	if err != nil && !errors.Is(err, ErrProjectionUnavailable) {
		if closeErr := r.closeProjectionLocked(); closeErr != nil {
			return errors.Join(err, fmt.Errorf("close incompatible projection: %w", closeErr))
		}
		if removeErr := removeProjectionFiles(r.mergeStateDir); removeErr != nil {
			return errors.Join(err, fmt.Errorf("remove incompatible projection: %w", removeErr))
		}
		if openErr := r.openProjectionLocked(); openErr != nil {
			return openErr
		}
		status, err = r.projectionStatusLocked()
	}
	if err == nil && status.SchemaVersion == projectionSchemaVersion && status.State == "ready" &&
		status.Branch == branch && status.Commit == commitID && status.NodeRoot == snapshot.NodeRoot {
		return nil
	}
	return r.rebuildProjectionLocked(branch, commitID, snapshot)
}

func (r *Repository) maintainActiveProjectionLocked(branch string) error {
	if branch != r.activeBranch {
		return nil
	}
	return r.ensureProjectionForActiveBranchLocked()
}

func (r *Repository) rebuildProjectionLocked(branch string, commitID ObjectID, snapshot graphSnapshot) (err error) {
	if r.projectionDB == nil {
		return ErrProjectionUnavailable
	}
	if _, err = r.projectionDB.Exec(`
INSERT INTO index_meta(repository_id, schema_version, projected_branch, projected_commit, node_root, projection_state, projected_at_us, failure_reason)
VALUES (?, ?, ?, ?, ?, 'building', ?, '')
ON CONFLICT(repository_id) DO UPDATE SET schema_version=excluded.schema_version, projected_branch=excluded.projected_branch,
projected_commit=excluded.projected_commit, node_root=excluded.node_root, projection_state='building',
projected_at_us=excluded.projected_at_us, failure_reason=''`,
		r.projectionRepositoryID(), projectionSchemaVersion, branch, commitID, snapshot.NodeRoot, time.Now().UTC().UnixMicro()); err != nil {
		return fmt.Errorf("mark projection building: %w", err)
	}
	defer func() {
		if err == nil {
			return
		}
		_, _ = r.projectionDB.Exec(`UPDATE index_meta SET projection_state='failed', failure_reason=?, projected_at_us=? WHERE repository_id=?`,
			err.Error(), time.Now().UTC().UnixMicro(), r.projectionRepositoryID())
		err = fmt.Errorf("%w: %v", ErrProjectionUnavailable, err)
	}()

	tx, err := r.projectionDB.Begin()
	if err != nil {
		return fmt.Errorf("begin projection rebuild: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	for _, table := range []string{"node_labels", "nodes", "edges", "node_property_text", "node_property_number", "commits", "commit_parents", "entity_changes", "node_fts"} {
		if _, err = tx.Exec("DELETE FROM " + table); err != nil {
			return fmt.Errorf("clear projection %s: %w", table, err)
		}
	}
	nodeLifecycle, edgeLifecycle, err := r.entityLifecycleLocked(commitID)
	if err != nil {
		return err
	}
	if err = r.projectCommitHistoryLocked(tx, commitID); err != nil {
		return err
	}
	if err = r.projectSnapshotLocked(tx, commitID, snapshot, nodeLifecycle, edgeLifecycle); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE index_meta SET projection_state='ready', projected_at_us=?, failure_reason='' WHERE repository_id=?`,
		time.Now().UTC().UnixMicro(), r.projectionRepositoryID()); err != nil {
		return fmt.Errorf("mark projection ready: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit projection rebuild: %w", err)
	}
	return nil
}

func (r *Repository) projectCommitHistoryLocked(tx *sql.Tx, head ObjectID) error {
	seen := make(map[ObjectID]bool)
	var visit func(ObjectID) error
	visit = func(id ObjectID) error {
		if seen[id] {
			return nil
		}
		seen[id] = true
		value := r.commits[id]
		for _, parent := range value.Parents {
			if err := visit(parent); err != nil {
				return err
			}
		}
		snapshot := r.snapshots[value.Snapshot]
		if err := r.ensureSnapshotProjectionLocked(value.Snapshot); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO commits(commit_oid, graph_root_oid, schema_root_oid, author, committed_at_us, message) VALUES (?, ?, ?, ?, ?, ?)`,
			id, value.Snapshot, snapshot.SchemaRoot, value.Author, value.Time.UTC().UnixMicro(), value.Message); err != nil {
			return fmt.Errorf("insert commit %s: %w", id, err)
		}
		for index, parent := range value.Parents {
			if _, err := tx.Exec(`INSERT INTO commit_parents(commit_oid, parent_index, parent_oid) VALUES (?, ?, ?)`, id, index, parent); err != nil {
				return fmt.Errorf("insert commit parent %s: %w", id, err)
			}
		}
		if len(value.Parents) > 0 {
			if err := r.projectEntityChangesLocked(tx, id, value.Parents[0]); err != nil {
				return err
			}
		}
		return nil
	}
	return visit(head)
}

func (r *Repository) projectEntityChangesLocked(tx *sql.Tx, commitID, parentID ObjectID) error {
	current := r.snapshots[r.commits[commitID].Snapshot]
	parent := r.snapshots[r.commits[parentID].Snapshot]
	if err := r.ensureSnapshotProjectionLocked(r.commits[parentID].Snapshot); err != nil {
		return err
	}
	if err := r.ensureSnapshotProjectionLocked(r.commits[commitID].Snapshot); err != nil {
		return err
	}
	if err := projectEntityChangeSet(tx, "node", commitID, r.projections[parent.NodeRoot], r.projections[current.NodeRoot],
		func(node Node) ObjectID { return r.objectID("node", canonicalNodeCollections(node)) },
		func(left, right Node) bool { return left.Equal(right) }); err != nil {
		return err
	}
	return projectEntityChangeSet(tx, "edge", commitID, r.edgeProjections[r.commits[parentID].Snapshot], r.edgeProjections[r.commits[commitID].Snapshot],
		func(edge Edge) ObjectID { return r.objectID("edge", canonicalEdgeProperties(edge)) },
		func(left, right Edge) bool { return left.Equal(right) })
}

func projectEntityChangeSet[T any](tx *sql.Tx, kind string, commitID ObjectID, before, after map[string]T, objectID func(T) ObjectID, equal func(T, T) bool) error {
	ids := make([]string, 0, len(before)+len(after))
	seen := make(map[string]bool, len(before)+len(after))
	for id := range before {
		seen[id] = true
		ids = append(ids, id)
	}
	for id := range after {
		if !seen[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		beforeValue, hadBefore := before[id]
		afterValue, hadAfter := after[id]
		if hadBefore && hadAfter && equal(beforeValue, afterValue) {
			continue
		}
		change := "modify"
		var beforeID, afterID ObjectID
		if hadBefore {
			beforeID = objectID(beforeValue)
		}
		if hadAfter {
			afterID = objectID(afterValue)
		}
		if !hadBefore {
			change = "add"
		} else if !hadAfter {
			change = "delete"
		}
		if _, err := tx.Exec(`INSERT INTO entity_changes(entity_kind, entity_id, commit_oid, change_kind, before_oid, after_oid) VALUES (?, ?, ?, ?, ?, ?)`,
			kind, id, commitID, change, nullableObjectID(beforeID), nullableObjectID(afterID)); err != nil {
			return fmt.Errorf("insert %s change %s: %w", kind, id, err)
		}
	}
	return nil
}

func nullableObjectID(id ObjectID) any {
	if id == "" {
		return nil
	}
	return string(id)
}

type entityLifecycle struct {
	CreatedCommit ObjectID
	UpdatedCommit ObjectID
}

func (r *Repository) entityLifecycleLocked(head ObjectID) (map[string]entityLifecycle, map[string]entityLifecycle, error) {
	nodes, edges := make(map[string]entityLifecycle), make(map[string]entityLifecycle)
	seen := make(map[ObjectID]bool)
	var visit func(ObjectID) error
	visit = func(id ObjectID) error {
		if seen[id] {
			return nil
		}
		seen[id] = true
		value, ok := r.commits[id]
		if !ok {
			return fmt.Errorf("find entity lifecycle commit %s: %w", id, ErrCommitNotFound)
		}
		for _, parent := range value.Parents {
			if err := visit(parent); err != nil {
				return err
			}
		}
		current := r.snapshots[value.Snapshot]
		if err := r.ensureSnapshotProjectionLocked(value.Snapshot); err != nil {
			return err
		}
		if len(value.Parents) == 0 {
			for nodeID := range r.projections[current.NodeRoot] {
				nodes[nodeID] = entityLifecycle{CreatedCommit: id, UpdatedCommit: id}
			}
			for edgeID := range r.edgeProjections[value.Snapshot] {
				edges[edgeID] = entityLifecycle{CreatedCommit: id, UpdatedCommit: id}
			}
			return nil
		}
		parent := r.snapshots[r.commits[value.Parents[0]].Snapshot]
		if err := r.ensureSnapshotProjectionLocked(r.commits[value.Parents[0]].Snapshot); err != nil {
			return err
		}
		updateEntityLifecycle(nodes, r.projections[parent.NodeRoot], r.projections[current.NodeRoot], func(left, right Node) bool { return left.Equal(right) }, id)
		updateEntityLifecycle(edges, r.edgeProjections[r.commits[value.Parents[0]].Snapshot], r.edgeProjections[value.Snapshot], func(left, right Edge) bool { return left.Equal(right) }, id)
		return nil
	}
	if err := visit(head); err != nil {
		return nil, nil, err
	}
	return nodes, edges, nil
}

func updateEntityLifecycle[T any](lifecycle map[string]entityLifecycle, before, after map[string]T, equal func(T, T) bool, commitID ObjectID) {
	for id, beforeValue := range before {
		afterValue, exists := after[id]
		if !exists {
			delete(lifecycle, id)
			continue
		}
		if !equal(beforeValue, afterValue) {
			entry := lifecycle[id]
			if entry.CreatedCommit == "" {
				entry.CreatedCommit = commitID
			}
			entry.UpdatedCommit = commitID
			lifecycle[id] = entry
		}
	}
	for id := range after {
		if _, existed := before[id]; existed {
			continue
		}
		lifecycle[id] = entityLifecycle{CreatedCommit: commitID, UpdatedCommit: commitID}
	}
}

func (r *Repository) projectSnapshotLocked(tx *sql.Tx, commitID ObjectID, snapshot graphSnapshot, nodeLifecycle, edgeLifecycle map[string]entityLifecycle) error {
	if err := r.ensureSnapshotProjectionLocked(r.commits[commitID].Snapshot); err != nil {
		return err
	}
	nodes := r.projections[snapshot.NodeRoot]
	edges := r.edgeProjections[r.commits[commitID].Snapshot]
	schema, err := r.schemaSnapshotLocked(snapshot.SchemaRoot)
	if err != nil {
		return err
	}
	indexed := indexedNodeProperties(schema)
	for _, id := range sortedNodeIDs(nodes) {
		node := nodes[id]
		lifecycle := nodeLifecycle[id]
		if lifecycle.CreatedCommit == "" || lifecycle.UpdatedCommit == "" {
			return fmt.Errorf("node %s has no lifecycle metadata", id)
		}
		properties, err := json.Marshal(node.Properties)
		if err != nil {
			return fmt.Errorf("encode node %s properties: %w", id, err)
		}
		if _, err := tx.Exec(`INSERT INTO nodes(node_id, snapshot_oid, properties_json, created_commit, updated_commit) VALUES (?, ?, ?, ?, ?)`,
			node.ID, snapshot.NodeRoot, string(properties), lifecycle.CreatedCommit, lifecycle.UpdatedCommit); err != nil {
			return fmt.Errorf("insert node %s: %w", id, err)
		}
		for _, label := range node.Labels {
			if _, err := tx.Exec(`INSERT INTO node_labels(label, node_id) VALUES (?, ?)`, label, node.ID); err != nil {
				return fmt.Errorf("insert node label %s: %w", node.ID, err)
			}
		}
		bodyParts := make([]string, 0)
		keys := make([]string, 0, len(node.Properties))
		for key := range node.Properties {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := node.Properties[key]
			if value.Kind == PropertyString {
				bodyParts = append(bodyParts, key+": "+value.String)
			}
			if !indexed[key] {
				continue
			}
			switch value.Kind {
			case PropertyString:
				_, err = tx.Exec(`INSERT INTO node_property_text(property_key, property_value, node_id) VALUES (?, ?, ?)`, key, value.String, node.ID)
			case PropertyInteger:
				_, err = tx.Exec(`INSERT INTO node_property_number(property_key, property_value, node_id) VALUES (?, ?, ?)`, key, value.Integer, node.ID)
			case PropertyFloat:
				_, err = tx.Exec(`INSERT INTO node_property_number(property_key, property_value, node_id) VALUES (?, ?, ?)`, key, value.Float, node.ID)
			}
			if err != nil {
				return fmt.Errorf("insert indexed property %s.%s: %w", node.ID, key, err)
			}
		}
		if _, err := tx.Exec(`INSERT INTO node_fts(node_id, title, body, labels, tags) VALUES (?, ?, ?, ?, '')`,
			node.ID, node.Title, strings.Join(bodyParts, "\n"), strings.Join(node.Labels, " ")); err != nil {
			return fmt.Errorf("insert node FTS %s: %w", node.ID, err)
		}
	}
	for _, id := range sortedEdgeIDs(edges) {
		edge := edges[id]
		lifecycle := edgeLifecycle[id]
		if lifecycle.CreatedCommit == "" || lifecycle.UpdatedCommit == "" {
			return fmt.Errorf("edge %s has no lifecycle metadata", id)
		}
		properties, err := json.Marshal(edge.Properties)
		if err != nil {
			return fmt.Errorf("encode edge %s properties: %w", id, err)
		}
		if _, err := tx.Exec(`INSERT INTO edges(edge_id, snapshot_oid, edge_type, from_node, to_node, properties_json, created_commit, updated_commit) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			edge.ID, snapshot.EdgeRoot, edge.Type, edge.Source, edge.Target, string(properties), lifecycle.CreatedCommit, lifecycle.UpdatedCommit); err != nil {
			return fmt.Errorf("insert edge %s: %w", id, err)
		}
	}
	return nil
}

func indexedNodeProperties(schema SchemaSnapshot) map[string]bool {
	indexed := make(map[string]bool)
	for _, rule := range schema.NodeRules {
		for _, property := range rule.Properties {
			if property.Indexed {
				indexed[property.Key] = true
			}
		}
	}
	return indexed
}

func (r *Repository) closeProjectionLocked() error {
	if r.projectionDB == nil {
		return nil
	}
	err := r.projectionDB.Close()
	r.projectionDB = nil
	return err
}

func removeProjectionFiles(stateDir string) error {
	for _, name := range []string{"graph.db", "graph.db-shm", "graph.db-wal"} {
		err := os.Remove(filepath.Join(stateDir, name))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
