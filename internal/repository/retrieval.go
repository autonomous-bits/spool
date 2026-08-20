package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

var (
	// ErrInvalidProjectionSearch reports an empty or malformed lexical query.
	ErrInvalidProjectionSearch = errors.New("projection search query is invalid")
	// ErrInvalidMetadataPredicate reports a predicate with incompatible operands.
	ErrInvalidMetadataPredicate = errors.New("metadata predicate is invalid")
	// ErrUnindexedMetadataProperty reports a predicate for a property not indexed
	// by the selected snapshot schema.
	ErrUnindexedMetadataProperty = errors.New("metadata property is not indexed")
	// ErrUnsupportedMetadataPredicate reports a predicate whose value type is not
	// permitted by the selected snapshot schema.
	ErrUnsupportedMetadataPredicate = errors.New("metadata predicate is unsupported")
)

// SearchNodesRequest describes a bounded lexical search of the branch-head
// projection. Commit must be a commit previously pinned from Branch and must
// still be Branch's head when the query runs.
type SearchNodesRequest struct {
	Branch            string   `json:"branch"`
	Commit            ObjectID `json:"commit"`
	Query             string   `json:"query"`
	MaxRows           int      `json:"maxRows"`
	MaxResponseBytes  int      `json:"maxResponseBytes"`
	ContinuationToken string   `json:"continuationToken,omitempty"`
}

// SearchNodeMatch is a projection-backed lexical match. MatchedFields uses the
// FTS field names title, body, labels, and tags; Snippets contains marked
// excerpts for each matched field.
type SearchNodeMatch struct {
	Node          Node              `json:"node"`
	Score         float64           `json:"score"`
	MatchedFields []string          `json:"matchedFields"`
	Snippets      map[string]string `json:"snippets"`
}

// SearchNodesResult is one deterministic page of lexical matches.
type SearchNodesResult struct {
	Branch            string            `json:"branch"`
	Commit            ObjectID          `json:"commit"`
	Snapshot          ObjectID          `json:"snapshot"`
	Matches           []SearchNodeMatch `json:"matches"`
	ContinuationToken string            `json:"continuationToken,omitempty"`
}

// MetadataPredicate is a typed predicate over one schema-indexed scalar
// property. Set TextEquals for text equality, NumberEquals for numeric equality,
// or NumberMin and/or NumberMax for an inclusive numeric range.
type MetadataPredicate struct {
	Key          string   `json:"key"`
	TextEquals   *string  `json:"textEquals,omitempty"`
	NumberEquals *float64 `json:"numberEquals,omitempty"`
	NumberMin    *float64 `json:"numberMin,omitempty"`
	NumberMax    *float64 `json:"numberMax,omitempty"`
}

// FilterNodesRequest describes a bounded metadata query of the branch-head
// projection. Labels and predicates are combined with AND.
type FilterNodesRequest struct {
	Branch            string              `json:"branch"`
	Commit            ObjectID            `json:"commit"`
	Labels            []string            `json:"labels,omitempty"`
	Predicates        []MetadataPredicate `json:"predicates,omitempty"`
	MaxRows           int                 `json:"maxRows"`
	MaxResponseBytes  int                 `json:"maxResponseBytes"`
	ContinuationToken string              `json:"continuationToken,omitempty"`
}

// FilterNodesResult is one deterministic page of metadata matches.
type FilterNodesResult struct {
	Branch            string   `json:"branch"`
	Commit            ObjectID `json:"commit"`
	Snapshot          ObjectID `json:"snapshot"`
	Nodes             []Node   `json:"nodes"`
	ContinuationToken string   `json:"continuationToken,omitempty"`
}

// SearchNodes searches node titles, string properties, labels, and tags through
// the private FTS5 projection.
func (r *Repository) SearchNodes(request SearchNodesRequest) (SearchNodesResult, error) {
	return r.SearchNodesContext(context.Background(), request)
}

// SearchNodesContext searches the branch-head projection with a parameterized
// FTS5 match expression and honors cancellation while querying and materializing
// the page.
func (r *Repository) SearchNodesContext(ctx context.Context, request SearchNodesRequest) (SearchNodesResult, error) {
	if err := ctx.Err(); err != nil {
		return SearchNodesResult{}, err
	}
	if strings.TrimSpace(request.Query) == "" {
		return SearchNodesResult{}, ErrInvalidProjectionSearch
	}
	if err := validateProjectionQueryBudget(request.MaxRows, request.MaxResponseBytes); err != nil {
		return SearchNodesResult{}, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return SearchNodesResult{}, err
	}
	if err := r.ensureOpenLocked(); err != nil {
		return SearchNodesResult{}, err
	}
	snapshot, err := r.requireBranchHeadProjectionLocked(ctx, request.Branch, request.Commit)
	if err != nil {
		return SearchNodesResult{}, err
	}
	fingerprint := queryFingerprint(struct {
		Branch           string
		Commit           ObjectID
		Query            string
		MaxRows          int
		MaxResponseBytes int
	}{request.Branch, request.Commit, request.Query, request.MaxRows, request.MaxResponseBytes})
	offset, err := decodeContinuation(request.ContinuationToken, fingerprint)
	if err != nil {
		return SearchNodesResult{}, err
	}

	rows, err := r.projectionDB.QueryContext(ctx, `
SELECT f.node_id, n.properties_json, f.title, bm25(node_fts),
       highlight(node_fts, 1, '<mark>', '</mark>'), snippet(node_fts, 1, '<mark>', '</mark>', '…', 12),
       highlight(node_fts, 2, '<mark>', '</mark>'), snippet(node_fts, 2, '<mark>', '</mark>', '…', 12),
       highlight(node_fts, 3, '<mark>', '</mark>'), snippet(node_fts, 3, '<mark>', '</mark>', '…', 12),
       highlight(node_fts, 4, '<mark>', '</mark>'), snippet(node_fts, 4, '<mark>', '</mark>', '…', 12)
FROM node_fts AS f
JOIN nodes AS n ON n.node_id = f.node_id
WHERE node_fts MATCH ?
ORDER BY bm25(node_fts), f.node_id
LIMIT ? OFFSET ?`, request.Query, request.MaxRows+1, offset)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return SearchNodesResult{}, ctxErr
		}
		return SearchNodesResult{}, fmt.Errorf("%w: %v", ErrInvalidProjectionSearch, err)
	}
	defer rows.Close()

	type searchRow struct {
		id, properties, title string
		score                 float64
		highlights, snippets  [4]string
	}
	matches := make([]searchRow, 0, projectionPageCapacity(request.MaxRows))
	for rows.Next() {
		var row searchRow
		if err := rows.Scan(&row.id, &row.properties, &row.title, &row.score,
			&row.highlights[0], &row.snippets[0], &row.highlights[1], &row.snippets[1],
			&row.highlights[2], &row.snippets[2], &row.highlights[3], &row.snippets[3]); err != nil {
			return SearchNodesResult{}, fmt.Errorf("read projection search result: %w", err)
		}
		matches = append(matches, row)
	}
	if err := rows.Err(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return SearchNodesResult{}, ctxErr
		}
		return SearchNodesResult{}, fmt.Errorf("search projection: %w", err)
	}
	if offset > 0 && len(matches) == 0 {
		return SearchNodesResult{}, ErrInvalidContinuation
	}
	hasMore := len(matches) > request.MaxRows
	if hasMore {
		matches = matches[:request.MaxRows]
	}

	result := SearchNodesResult{
		Branch: request.Branch, Commit: request.Commit, Snapshot: snapshot,
		Matches: make([]SearchNodeMatch, 0, projectionPageCapacity(request.MaxRows)),
	}
	if !resultFits(result, request.MaxResponseBytes) {
		return SearchNodesResult{}, ErrResponseBudgetTooSmall
	}
	fields := [...]string{"title", "body", "labels", "tags"}
	for index, row := range matches {
		if err := ctx.Err(); err != nil {
			return SearchNodesResult{}, err
		}
		node, err := r.projectionNodeLocked(ctx, row.id, row.title, row.properties)
		if err != nil {
			return SearchNodesResult{}, err
		}
		match := SearchNodeMatch{Node: node, Score: row.score, MatchedFields: make([]string, 0), Snippets: make(map[string]string)}
		for fieldIndex, field := range fields {
			if strings.Contains(row.highlights[fieldIndex], "<mark>") {
				match.MatchedFields = append(match.MatchedFields, field)
				match.Snippets[field] = row.snippets[fieldIndex]
			}
		}
		moreAfter := index+1 < len(matches) || hasMore
		candidate := result
		candidate.Matches = append(append([]SearchNodeMatch(nil), result.Matches...), match)
		candidate.ContinuationToken = ""
		if moreAfter {
			candidate.ContinuationToken = encodeContinuation(fingerprint, offset+len(candidate.Matches))
		}
		if !resultFits(candidate, request.MaxResponseBytes) {
			break
		}
		result = candidate
	}
	if len(result.Matches) < len(matches) || hasMore {
		if len(result.Matches) == 0 {
			return SearchNodesResult{}, ErrResponseBudgetTooSmall
		}
		result.ContinuationToken = encodeContinuation(fingerprint, offset+len(result.Matches))
		if !resultFits(result, request.MaxResponseBytes) {
			return SearchNodesResult{}, ErrResponseBudgetTooSmall
		}
	}
	return result, nil
}

// FilterNodes returns nodes matching typed metadata predicates through the
// private projection.
func (r *Repository) FilterNodes(request FilterNodesRequest) (FilterNodesResult, error) {
	return r.FilterNodesContext(context.Background(), request)
}

// FilterNodesContext filters a branch-head projection using only schema-indexed
// scalar properties and honors cancellation throughout the query.
func (r *Repository) FilterNodesContext(ctx context.Context, request FilterNodesRequest) (FilterNodesResult, error) {
	if err := ctx.Err(); err != nil {
		return FilterNodesResult{}, err
	}
	if err := validateProjectionQueryBudget(request.MaxRows, request.MaxResponseBytes); err != nil {
		return FilterNodesResult{}, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return FilterNodesResult{}, err
	}
	if err := r.ensureOpenLocked(); err != nil {
		return FilterNodesResult{}, err
	}
	snapshot, err := r.requireBranchHeadProjectionLocked(ctx, request.Branch, request.Commit)
	if err != nil {
		return FilterNodesResult{}, err
	}
	schema, err := r.schemaSnapshotLocked(r.snapshots[snapshot].SchemaRoot)
	if err != nil {
		return FilterNodesResult{}, err
	}
	labels, predicates, err := normalizeMetadataFilter(request.Labels, request.Predicates, schema)
	if err != nil {
		return FilterNodesResult{}, err
	}
	fingerprint := queryFingerprint(struct {
		Branch           string
		Commit           ObjectID
		Labels           []string
		Predicates       []MetadataPredicate
		MaxRows          int
		MaxResponseBytes int
	}{request.Branch, request.Commit, labels, predicates, request.MaxRows, request.MaxResponseBytes})
	offset, err := decodeContinuation(request.ContinuationToken, fingerprint)
	if err != nil {
		return FilterNodesResult{}, err
	}

	query, args := metadataQuery(labels, predicates)
	args = append(args, request.MaxRows+1, offset)
	rows, err := r.projectionDB.QueryContext(ctx, query, args...)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return FilterNodesResult{}, ctxErr
		}
		return FilterNodesResult{}, fmt.Errorf("filter projection: %w", err)
	}
	defer rows.Close()
	type filterRow struct{ id, properties, title string }
	matches := make([]filterRow, 0, projectionPageCapacity(request.MaxRows))
	for rows.Next() {
		var row filterRow
		if err := rows.Scan(&row.id, &row.properties, &row.title); err != nil {
			return FilterNodesResult{}, fmt.Errorf("read projection filter result: %w", err)
		}
		matches = append(matches, row)
	}
	if err := rows.Err(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return FilterNodesResult{}, ctxErr
		}
		return FilterNodesResult{}, fmt.Errorf("filter projection: %w", err)
	}
	if offset > 0 && len(matches) == 0 {
		return FilterNodesResult{}, ErrInvalidContinuation
	}
	hasMore := len(matches) > request.MaxRows
	if hasMore {
		matches = matches[:request.MaxRows]
	}

	result := FilterNodesResult{
		Branch: request.Branch, Commit: request.Commit, Snapshot: snapshot,
		Nodes: make([]Node, 0, projectionPageCapacity(request.MaxRows)),
	}
	if !resultFits(result, request.MaxResponseBytes) {
		return FilterNodesResult{}, ErrResponseBudgetTooSmall
	}
	for index, row := range matches {
		if err := ctx.Err(); err != nil {
			return FilterNodesResult{}, err
		}
		node, err := r.projectionNodeLocked(ctx, row.id, row.title, row.properties)
		if err != nil {
			return FilterNodesResult{}, err
		}
		moreAfter := index+1 < len(matches) || hasMore
		candidate := result
		candidate.Nodes = append(append([]Node(nil), result.Nodes...), node)
		candidate.ContinuationToken = ""
		if moreAfter {
			candidate.ContinuationToken = encodeContinuation(fingerprint, offset+len(candidate.Nodes))
		}
		if !resultFits(candidate, request.MaxResponseBytes) {
			break
		}
		result = candidate
	}
	if len(result.Nodes) < len(matches) || hasMore {
		if len(result.Nodes) == 0 {
			return FilterNodesResult{}, ErrResponseBudgetTooSmall
		}
		result.ContinuationToken = encodeContinuation(fingerprint, offset+len(result.Nodes))
		if !resultFits(result, request.MaxResponseBytes) {
			return FilterNodesResult{}, ErrResponseBudgetTooSmall
		}
	}
	return result, nil
}

func validateProjectionQueryBudget(maxRows, maxResponseBytes int) error {
	if maxRows <= 0 || maxRows == int(^uint(0)>>1) || maxResponseBytes <= 0 {
		return ErrInvalidListBudget
	}
	return nil
}

func projectionPageCapacity(maxRows int) int {
	if maxRows < 128 {
		return maxRows + 1
	}
	return 128
}

func (r *Repository) requireBranchHeadProjectionLocked(ctx context.Context, branch string, commitID ObjectID) (ObjectID, error) {
	head, ok := r.branches[branch]
	if !ok {
		return "", ErrBranchNotFound
	}
	commit, ok := r.commits[commitID]
	if !ok {
		return "", ErrCommitNotFound
	}
	if commitID != head {
		return "", ErrHistoricalProjectionUnsupported
	}
	snapshot, ok := r.snapshots[commit.Snapshot]
	if !ok {
		return "", fmt.Errorf("snapshot %q: %w", commit.Snapshot, ErrCommitNotFound)
	}
	if r.projectionDB == nil {
		return "", ErrProjectionUnavailable
	}
	var status ProjectionStatus
	err := r.projectionDB.QueryRowContext(ctx, `
SELECT schema_version, projection_state, projected_branch, projected_commit, node_root
FROM index_meta WHERE repository_id = ?`, r.projectionRepositoryID()).
		Scan(&status.SchemaVersion, &status.State, &status.Branch, &status.Commit, &status.NodeRoot)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrProjectionUnavailable
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		return "", fmt.Errorf("read projection metadata: %w", err)
	}
	if status.SchemaVersion != projectionSchemaVersion || status.State != "ready" ||
		status.Branch != branch || status.Commit != commitID || status.NodeRoot != snapshot.NodeRoot {
		return "", ErrProjectionUnavailable
	}
	return commit.Snapshot, nil
}

func (r *Repository) projectionNodeLocked(ctx context.Context, id, title, encodedProperties string) (Node, error) {
	properties := make(map[string]PropertyValue)
	if err := json.Unmarshal([]byte(encodedProperties), &properties); err != nil {
		return Node{}, fmt.Errorf("decode projected node %s properties: %w", id, err)
	}
	rows, err := r.projectionDB.QueryContext(ctx, `SELECT label FROM node_labels WHERE node_id = ? ORDER BY label`, id)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Node{}, ctxErr
		}
		return Node{}, fmt.Errorf("read projected node %s labels: %w", id, err)
	}
	defer rows.Close()
	labels := make([]string, 0)
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return Node{}, fmt.Errorf("read projected node %s label: %w", id, err)
		}
		labels = append(labels, label)
	}
	if err := rows.Err(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Node{}, ctxErr
		}
		return Node{}, fmt.Errorf("read projected node %s labels: %w", id, err)
	}
	node, err := (Node{ID: id, Title: title, Labels: labels, Properties: properties}).Normalize()
	if err != nil {
		return Node{}, fmt.Errorf("normalize projected node %s: %w", id, err)
	}
	return node, nil
}

func normalizeMetadataFilter(labels []string, predicates []MetadataPredicate, schema SchemaSnapshot) ([]string, []MetadataPredicate, error) {
	normalizedLabels := append([]string(nil), labels...)
	sort.Strings(normalizedLabels)
	for i, label := range normalizedLabels {
		if label == "" || (i > 0 && normalizedLabels[i-1] == label) {
			return nil, nil, ErrInvalidMetadataPredicate
		}
	}
	normalizedPredicates := append([]MetadataPredicate(nil), predicates...)
	sort.Slice(normalizedPredicates, func(i, j int) bool { return normalizedPredicates[i].Key < normalizedPredicates[j].Key })
	for i := range normalizedPredicates {
		predicate := &normalizedPredicates[i]
		if predicate.Key == "" || (i > 0 && normalizedPredicates[i-1].Key == predicate.Key) {
			return nil, nil, ErrInvalidMetadataPredicate
		}
		if err := validateMetadataPredicate(*predicate, schema); err != nil {
			return nil, nil, err
		}
	}
	return normalizedLabels, normalizedPredicates, nil
}

func validateMetadataPredicate(predicate MetadataPredicate, schema SchemaSnapshot) error {
	hasText := predicate.TextEquals != nil
	hasNumberEquality := predicate.NumberEquals != nil
	hasNumberRange := predicate.NumberMin != nil || predicate.NumberMax != nil
	if (hasText && (hasNumberEquality || hasNumberRange)) || (hasNumberEquality && hasNumberRange) || (!hasText && !hasNumberEquality && !hasNumberRange) {
		return ErrInvalidMetadataPredicate
	}
	for _, value := range []*float64{predicate.NumberEquals, predicate.NumberMin, predicate.NumberMax} {
		if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
			return ErrInvalidMetadataPredicate
		}
	}
	if predicate.NumberMin != nil && predicate.NumberMax != nil && *predicate.NumberMin > *predicate.NumberMax {
		return ErrInvalidMetadataPredicate
	}
	indexed, types := indexedPropertyTypes(schema, predicate.Key)
	if !indexed {
		return fmt.Errorf("%w: %s", ErrUnindexedMetadataProperty, predicate.Key)
	}
	if hasText && !types[PropertyString] {
		return fmt.Errorf("%w: %s does not accept text", ErrUnsupportedMetadataPredicate, predicate.Key)
	}
	if !hasText && !types[PropertyInteger] && !types[PropertyFloat] {
		return fmt.Errorf("%w: %s does not accept numbers", ErrUnsupportedMetadataPredicate, predicate.Key)
	}
	return nil
}

func indexedPropertyTypes(schema SchemaSnapshot, key string) (bool, map[PropertyKind]bool) {
	types := make(map[PropertyKind]bool)
	indexed := false
	for _, rule := range schema.NodeRules {
		for _, property := range rule.Properties {
			if property.Key != key || !property.Indexed {
				continue
			}
			indexed = true
			for _, kind := range property.Types {
				types[kind] = true
			}
		}
	}
	return indexed, types
}

func metadataQuery(labels []string, predicates []MetadataPredicate) (string, []any) {
	var query strings.Builder
	query.WriteString(`
SELECT n.node_id, n.properties_json, f.title
FROM nodes AS n
JOIN node_fts AS f ON f.node_id = n.node_id
WHERE 1 = 1`)
	args := make([]any, 0, len(labels)+len(predicates)*3)
	for _, label := range labels {
		query.WriteString(`
  AND EXISTS (SELECT 1 FROM node_labels AS l WHERE l.node_id = n.node_id AND l.label = ?)`)
		args = append(args, label)
	}
	for _, predicate := range predicates {
		switch {
		case predicate.TextEquals != nil:
			query.WriteString(`
  AND EXISTS (SELECT 1 FROM node_property_text AS p WHERE p.node_id = n.node_id AND p.property_key = ? AND p.property_value = ?)`)
			args = append(args, predicate.Key, *predicate.TextEquals)
		case predicate.NumberEquals != nil:
			query.WriteString(`
  AND EXISTS (SELECT 1 FROM node_property_number AS p WHERE p.node_id = n.node_id AND p.property_key = ? AND p.property_value = ?)`)
			args = append(args, predicate.Key, *predicate.NumberEquals)
		default:
			query.WriteString(`
  AND EXISTS (SELECT 1 FROM node_property_number AS p WHERE p.node_id = n.node_id AND p.property_key = ?`)
			args = append(args, predicate.Key)
			if predicate.NumberMin != nil {
				query.WriteString(` AND p.property_value >= ?`)
				args = append(args, *predicate.NumberMin)
			}
			if predicate.NumberMax != nil {
				query.WriteString(` AND p.property_value <= ?`)
				args = append(args, *predicate.NumberMax)
			}
			query.WriteString(`)`)
		}
	}
	query.WriteString(`
ORDER BY n.node_id
LIMIT ? OFFSET ?`)
	return query.String(), args
}
