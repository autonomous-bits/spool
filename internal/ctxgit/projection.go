package ctxgit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/autonomous-bits/spool/internal/repository"
	_ "modernc.org/sqlite"
)

const projectionFileName = "projection.db"

// ProjectionStatus describes the disposable local query cache.
type ProjectionStatus struct {
	State         string `json:"state"`
	SolutionID    string `json:"solutionId"`
	Commit        string `json:"commit"`
	NodeCount     int    `json:"nodeCount"`
	EdgeCount     int    `json:"edgeCount"`
	Path          string `json:"path"`
	SchemaVersion int    `json:"schemaVersion"`
}

func projectionPath(cacheDir string) string {
	return filepath.Join(cacheDir, projectionFileName)
}

func (s *Session) rebuildProjection(ctx context.Context, commit string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(s.CacheDir, 0o755); err != nil {
		return err
	}
	path := projectionPath(s.CacheDir)
	_ = os.Remove(path)
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open projection: %w", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(ctx, `PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE meta (
    solution_id TEXT PRIMARY KEY,
    remote TEXT NOT NULL,
    protected_branch TEXT NOT NULL,
    git_commit TEXT NOT NULL,
    schema_version INTEGER NOT NULL,
    state TEXT NOT NULL,
    rebuilt_at_us INTEGER NOT NULL
);
CREATE TABLE nodes (
    node_id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    labels_json TEXT NOT NULL,
    properties_json TEXT NOT NULL
);
CREATE TABLE node_labels (
    label TEXT NOT NULL,
    node_id TEXT NOT NULL,
    PRIMARY KEY (label, node_id)
);
CREATE TABLE edges (
    edge_id TEXT PRIMARY KEY,
    edge_type TEXT NOT NULL,
    from_node TEXT NOT NULL,
    to_node TEXT NOT NULL,
    properties_json TEXT NOT NULL
);
CREATE VIRTUAL TABLE node_fts USING fts5(
    node_id UNINDEXED, title, body, labels,
    tokenize = 'unicode61 remove_diacritics 2'
);`); err != nil {
		return fmt.Errorf("create projection schema: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	ids := make([]string, 0, len(s.graph.Nodes))
	for id := range s.graph.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		node := s.graph.Nodes[id]
		labelsJSON, err := json.Marshal(node.Labels)
		if err != nil {
			return err
		}
		propsJSON, err := json.Marshal(node.Properties)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO nodes(node_id, title, labels_json, properties_json) VALUES (?, ?, ?, ?)`,
			node.ID, node.Title, string(labelsJSON), string(propsJSON)); err != nil {
			return err
		}
		for _, label := range node.Labels {
			if _, err := tx.ExecContext(ctx, `INSERT INTO node_labels(label, node_id) VALUES (?, ?)`, label, node.ID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO node_fts(node_id, title, body, labels) VALUES (?, ?, ?, ?)`,
			node.ID, node.Title, ftsBody(node), strings.Join(node.Labels, " ")); err != nil {
			return err
		}
	}
	edgeIDs := make([]string, 0, len(s.graph.Edges))
	for id := range s.graph.Edges {
		edgeIDs = append(edgeIDs, id)
	}
	sort.Strings(edgeIDs)
	for _, id := range edgeIDs {
		edge := s.graph.Edges[id]
		propsJSON, err := json.Marshal(edge.Properties)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO edges(edge_id, edge_type, from_node, to_node, properties_json) VALUES (?, ?, ?, ?, ?)`,
			edge.ID, edge.Type, edge.Source, edge.Target, string(propsJSON)); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO meta(solution_id, remote, protected_branch, git_commit, schema_version, state, rebuilt_at_us) VALUES (?, ?, ?, ?, 1, 'ready', ?)`,
		s.Bind.SolutionID, s.Bind.Remote, s.Bind.ProtectedBranch, commit, time.Now().UTC().UnixMicro()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.projectionPath = path
	return nil
}

func ftsBody(node repository.Node) string {
	var parts []string
	for key, value := range node.Properties {
		if value.Kind == repository.PropertyString && value.String != "" {
			parts = append(parts, key+":"+value.String)
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

// ProjectionStatus reports the rebuilt local cache. It is never source of truth.
func (s *Session) ProjectionStatus() (ProjectionStatus, error) {
	if s == nil || !s.Bound() {
		return ProjectionStatus{}, UnboundError()
	}
	status := ProjectionStatus{
		State:         "ready",
		SolutionID:    s.Bind.SolutionID,
		Commit:        s.head,
		NodeCount:     len(s.graph.Nodes),
		EdgeCount:     len(s.graph.Edges),
		Path:          s.projectionPath,
		SchemaVersion: 1,
	}
	if s.projectionPath == "" {
		status.State = "missing"
	}
	return status, nil
}

// ResolveNode returns a node from the current checkout-backed graph.
func (s *Session) ResolveNode(id string) (repository.Node, bool) {
	if s == nil || s.graph == nil {
		return repository.Node{}, false
	}
	id = NamespaceID(s.Bind.RepositoryID, id)
	node, ok := s.graph.Nodes[id]
	if ok {
		return node, true
	}
	node, ok = s.graph.Nodes[strings.TrimSpace(id)]
	return node, ok
}

// SearchNodes runs FTS against the local projection, falling back to title scan.
func (s *Session) SearchNodes(ctx context.Context, query string, limit int) ([]repository.SearchNodeMatch, error) {
	if s == nil || !s.Bound() {
		return nil, UnboundError()
	}
	if strings.TrimSpace(query) == "" {
		return nil, repository.ErrInvalidProjectionSearch
	}
	if limit <= 0 {
		limit = 20
	}
	if s.projectionPath != "" {
		db, err := sql.Open("sqlite", s.projectionPath)
		if err == nil {
			defer func() { _ = db.Close() }()
			rows, err := db.QueryContext(ctx, `SELECT node_id FROM node_fts WHERE node_fts MATCH ? LIMIT ?`, query, limit)
			if err == nil {
				defer func() { _ = rows.Close() }()
				var matches []repository.SearchNodeMatch
				for rows.Next() {
					var id string
					if err := rows.Scan(&id); err != nil {
						return nil, err
					}
					if node, ok := s.graph.Nodes[id]; ok {
						matches = append(matches, repository.SearchNodeMatch{
							Node:          node,
							Score:         1,
							MatchedFields: []string{"title"},
							Snippets:      map[string]string{"title": node.Title},
						})
					}
				}
				if err := rows.Err(); err != nil {
					return nil, err
				}
				if len(matches) > 0 {
					return matches, nil
				}
			}
		}
	}
	q := strings.ToLower(query)
	var matches []repository.SearchNodeMatch
	ids := make([]string, 0, len(s.graph.Nodes))
	for id := range s.graph.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		node := s.graph.Nodes[id]
		if strings.Contains(strings.ToLower(node.Title), q) || strings.Contains(strings.ToLower(id), q) {
			matches = append(matches, repository.SearchNodeMatch{
				Node: node, Score: 1, MatchedFields: []string{"title"}, Snippets: map[string]string{"title": node.Title},
			})
			if len(matches) >= limit {
				break
			}
		}
	}
	return matches, nil
}

// FilterNodes returns nodes matching every requested label.
func (s *Session) FilterNodes(labels []string, limit int) []repository.Node {
	if s == nil || s.graph == nil {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	ids := make([]string, 0, len(s.graph.Nodes))
	for id := range s.graph.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out []repository.Node
	for _, id := range ids {
		node := s.graph.Nodes[id]
		if !hasAllLabels(node.Labels, labels) {
			continue
		}
		out = append(out, node)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// GraphDump returns the current checkout-backed nodes and edges.
func (s *Session) GraphDump() (nodes []repository.Node, edges []repository.Edge) {
	if s == nil || s.graph == nil {
		return nil, nil
	}
	ids := make([]string, 0, len(s.graph.Nodes))
	for id := range s.graph.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		nodes = append(nodes, s.graph.Nodes[id])
	}
	edgeIDs := make([]string, 0, len(s.graph.Edges))
	for id := range s.graph.Edges {
		edgeIDs = append(edgeIDs, id)
	}
	sort.Strings(edgeIDs)
	for _, id := range edgeIDs {
		edges = append(edges, s.graph.Edges[id])
	}
	return nodes, edges
}

func hasAllLabels(have, want []string) bool {
	if len(want) == 0 {
		return true
	}
	set := map[string]struct{}{}
	for _, label := range have {
		set[label] = struct{}{}
	}
	for _, label := range want {
		if _, ok := set[label]; !ok {
			return false
		}
	}
	return true
}

// AssertProjectionUntracked reports whether a path would be committed from checkout.
func (s *Session) ProjectionInsideCheckout() bool {
	if s == nil || s.projectionPath == "" || s.CheckoutDir == "" {
		return false
	}
	rel, err := filepath.Rel(s.CheckoutDir, s.projectionPath)
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, "..")
}
