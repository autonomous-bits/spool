package ctxgit

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/autonomous-bits/spool/internal/contextual"
	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
)

type queryNeighbor struct {
	id   string
	edge repository.Edge
}

type queryDiscovered struct {
	node repository.Node
	path contextual.SupportingPath
}

// QuerySnapshot is provenance for a bound checkout query.
func (s *Session) QuerySnapshot(branch string) (resolve.SnapshotMetadata, resolve.ProjectionMetadata) {
	if branch == "" && s != nil {
		branch = s.Bind.ProtectedBranch
	}
	return contextSnapshot(s, branch)
}

func contextSnapshot(session *Session, branch string) (resolve.SnapshotMetadata, resolve.ProjectionMetadata) {
	if session == nil {
		return resolve.SnapshotMetadata{Branch: branch, Root: "context-git"},
			resolve.ProjectionMetadata{State: "unavailable", SchemaVersion: "1"}
	}
	status, err := session.ProjectionStatus()
	if err != nil {
		return resolve.SnapshotMetadata{Repository: session.Bind.SolutionID, Branch: branch, Root: "context-git"},
			resolve.ProjectionMetadata{State: "unavailable", SchemaVersion: "1"}
	}
	return resolve.SnapshotMetadata{
			Repository: session.Bind.SolutionID,
			Branch:     branch,
			Commit:     status.Commit,
			Root:       "context-git",
		}, resolve.ProjectionMetadata{
			NodeRoot:      "checkout",
			State:         status.State,
			SchemaVersion: "1",
		}
}

// ResolveResult returns a node from the bound checkout projection.
func (s *Session) ResolveResult(id string) (resolve.ResolveResult, error) {
	if s == nil || !s.Bound() {
		return resolve.ResolveResult{}, UnboundError()
	}
	node, ok := s.ResolveNode(id)
	if !ok {
		return resolve.ResolveResult{}, fmt.Errorf("%w: %s", resolve.ErrNodeNotFound, id)
	}
	snapshot, projection := s.QuerySnapshot(s.Bind.ProtectedBranch)
	return resolve.ResolveResult{Node: node, Snapshot: snapshot, Projection: projection}, nil
}

// SearchResult runs FTS against the rebuilt local projection.
func (s *Session) SearchResult(ctx context.Context, query string, limit int) (resolve.SearchResult, error) {
	if s == nil || !s.Bound() {
		return resolve.SearchResult{}, UnboundError()
	}
	matches, err := s.SearchNodes(ctx, query, limit)
	if err != nil {
		return resolve.SearchResult{}, err
	}
	if matches == nil {
		matches = []repository.SearchNodeMatch{}
	}
	snapshot, projection := s.QuerySnapshot(s.Bind.ProtectedBranch)
	return resolve.SearchResult{Snapshot: snapshot, Projection: projection, Matches: matches}, nil
}

// FilterResult returns nodes matching labels and typed property predicates.
func (s *Session) FilterResult(labels []string, predicates []repository.MetadataPredicate, limit int) (resolve.FilterResult, error) {
	if s == nil || !s.Bound() {
		return resolve.FilterResult{}, UnboundError()
	}
	nodes := s.FilterNodesPredicated(labels, predicates, limit)
	if nodes == nil {
		nodes = []repository.Node{}
	}
	snapshot, projection := s.QuerySnapshot(s.Bind.ProtectedBranch)
	return resolve.FilterResult{Snapshot: snapshot, Projection: projection, Nodes: nodes}, nil
}

// GraphResult dumps the bound checkout graph.
func (s *Session) GraphResult() (resolve.GraphResult, error) {
	if s == nil || !s.Bound() {
		return resolve.GraphResult{}, UnboundError()
	}
	nodes, edges := s.GraphDump()
	if nodes == nil {
		nodes = []repository.Node{}
	}
	if edges == nil {
		edges = []repository.Edge{}
	}
	snapshot, _ := s.QuerySnapshot(s.Bind.ProtectedBranch)
	return resolve.GraphResult{Snapshot: snapshot, Nodes: nodes, Edges: edges}, nil
}

// FilterNodesPredicated returns nodes matching every requested label and predicate.
func (s *Session) FilterNodesPredicated(labels []string, predicates []repository.MetadataPredicate, limit int) []repository.Node {
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
		if !matchesPredicates(node, predicates) {
			continue
		}
		out = append(out, node)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func matchesPredicates(node repository.Node, predicates []repository.MetadataPredicate) bool {
	for _, pred := range predicates {
		value, ok := node.Properties[pred.Key]
		if !ok {
			return false
		}
		if pred.TextEquals != nil {
			if value.Kind != repository.PropertyString || value.String != *pred.TextEquals {
				return false
			}
			continue
		}
		number, ok := propertyNumber(value)
		if !ok {
			return false
		}
		if pred.NumberEquals != nil && number != *pred.NumberEquals {
			return false
		}
		if pred.NumberMin != nil && number < *pred.NumberMin {
			return false
		}
		if pred.NumberMax != nil && number > *pred.NumberMax {
			return false
		}
	}
	return true
}

func propertyNumber(value repository.PropertyValue) (float64, bool) {
	switch value.Kind {
	case repository.PropertyInteger:
		return float64(value.Integer), true
	case repository.PropertyFloat:
		return value.Float, true
	default:
		return 0, false
	}
}

// QueryContextRequest is a bound checkout contextual query.
type QueryContextRequest struct {
	Query      string
	Labels     []string
	Predicates []repository.MetadataPredicate
	Direction  contextual.Direction
	EdgeTypes  []string
	SeedLimit  int
	MaxRows    int
	MaxVisited int
	MaxDepth   int
}

// QueryContext expands seed evidence over the bound checkout graph.
func (s *Session) QueryContext(ctx context.Context, req QueryContextRequest) (resolve.ContextResult, error) {
	return s.expandContext(ctx, req)
}

func (s *Session) expandContext(ctx context.Context, req QueryContextRequest) (resolve.ContextResult, error) {
	if s == nil || !s.Bound() {
		return resolve.ContextResult{}, UnboundError()
	}
	if err := ctx.Err(); err != nil {
		return resolve.ContextResult{}, err
	}
	if req.Direction == "" {
		req.Direction = contextual.DirectionOut
	}
	if req.Direction != contextual.DirectionOut && req.Direction != contextual.DirectionIn && req.Direction != contextual.DirectionBoth {
		return resolve.ContextResult{}, fmt.Errorf("invalid direction %q: must be 'out', 'in', or 'both'", req.Direction)
	}
	hasQuery := strings.TrimSpace(req.Query) != ""
	hasFilters := len(req.Labels) > 0 || len(req.Predicates) > 0
	if !hasQuery && !hasFilters {
		return resolve.ContextResult{}, contextual.ErrInvalidSeedSelector
	}
	if hasQuery && hasFilters {
		return resolve.ContextResult{}, contextual.ErrInvalidSeedSelector
	}
	if req.MaxRows <= 0 {
		req.MaxRows = 1000
	}
	if req.MaxVisited <= 0 {
		req.MaxVisited = 10_000
	}
	if req.MaxDepth < 0 {
		req.MaxDepth = 32
	}
	if req.MaxDepth == 0 {
		req.MaxDepth = 32
	}
	seedLimit := req.SeedLimit
	if seedLimit <= 0 || seedLimit > req.MaxRows {
		seedLimit = req.MaxRows
	}

	var evidence []contextual.Evidence
	if hasQuery {
		matches, err := s.SearchNodes(ctx, req.Query, seedLimit)
		if err != nil {
			return resolve.ContextResult{}, err
		}
		for _, match := range matches {
			evidence = append(evidence, contextual.Evidence{
				Node: match.Node, Score: match.Score, MatchedFields: match.MatchedFields, Snippets: match.Snippets,
			})
		}
	} else {
		for _, node := range s.FilterNodesPredicated(req.Labels, req.Predicates, seedLimit) {
			evidence = append(evidence, contextual.Evidence{Node: node})
		}
	}

	all, contextEdges, exhausted, timedOut, err := s.expandSeeds(ctx, evidence, req)
	if err != nil && !timedOut {
		return resolve.ContextResult{}, err
	}
	snapshot, projection := s.QuerySnapshot(s.Bind.ProtectedBranch)
	result := resolve.ContextResult{
		Snapshot:   snapshot,
		Projection: projection,
		Budget: resolve.QueryBudget{
			MaxRows: req.MaxRows, MaxVisited: req.MaxVisited, MaxDepth: req.MaxDepth,
			MaxResponseBytes: 1 << 20, Timeout: 10 * time.Second,
		},
		Evidence:          evidence,
		CapacityExhausted: exhausted,
		Completion: resolve.QueryCompletionMetadata{
			Complete:  !exhausted && !timedOut,
			Truncated: exhausted || timedOut,
			TimedOut:  timedOut,
			Visited:   len(all),
		},
	}
	if evidence == nil {
		result.Evidence = []contextual.Evidence{}
	}
	maxBytes := 1 << 20
	for _, entry := range all {
		if len(result.Nodes) == req.MaxRows {
			result.Completion.Truncated = true
			result.Completion.Complete = false
			break
		}
		candidate := result
		candidate.Nodes = append(append([]contextual.ContextNode(nil), result.Nodes...), contextual.ContextNode{Node: entry.node, Path: entry.path})
		candidate.Paths = append(append([]contextual.SupportingPath(nil), result.Paths...), entry.path)
		nodeIDs := make(map[string]struct{}, len(candidate.Nodes))
		for _, node := range candidate.Nodes {
			nodeIDs[node.Node.ID] = struct{}{}
		}
		candidate.Edges = selectedContextEdges(contextEdges, nodeIDs)
		candidate.Completion.Visited = len(candidate.Nodes)
		data, marshalErr := json.Marshal(candidate)
		if marshalErr != nil || len(data) > maxBytes {
			result.Completion.Truncated = true
			result.Completion.Complete = false
			break
		}
		candidate.Completion.ResponseBytes = len(data)
		result = candidate
	}
	if result.Nodes == nil {
		result.Nodes = []contextual.ContextNode{}
	}
	if result.Edges == nil {
		result.Edges = []repository.Edge{}
	}
	if result.Paths == nil {
		result.Paths = []contextual.SupportingPath{}
	}
	data, _ := json.Marshal(result)
	result.Completion.ResponseBytes = len(data)
	return result, nil
}

func (s *Session) expandSeeds(ctx context.Context, evidence []contextual.Evidence, req QueryContextRequest) ([]queryDiscovered, map[string]repository.Edge, bool, bool, error) {
	adjacency := map[string][]queryNeighbor{}
	contextEdges := map[string]repository.Edge{}
	allowed := map[string]struct{}{}
	for _, edgeType := range req.EdgeTypes {
		if edgeType != "" {
			allowed[edgeType] = struct{}{}
		}
	}
	for _, edge := range s.graph.Edges {
		if err := ctx.Err(); err != nil {
			return nil, nil, false, true, err
		}
		if len(allowed) > 0 {
			if _, ok := allowed[edge.Type]; !ok {
				continue
			}
		}
		contextEdges[edge.ID] = edge
		if req.Direction == contextual.DirectionOut || req.Direction == contextual.DirectionBoth {
			adjacency[edge.Source] = append(adjacency[edge.Source], queryNeighbor{id: edge.Target, edge: edge})
		}
		if req.Direction == contextual.DirectionIn || req.Direction == contextual.DirectionBoth {
			adjacency[edge.Target] = append(adjacency[edge.Target], queryNeighbor{id: edge.Source, edge: edge})
		}
	}
	for id := range adjacency {
		sort.Slice(adjacency[id], func(i, j int) bool {
			if adjacency[id][i].id != adjacency[id][j].id {
				return adjacency[id][i].id < adjacency[id][j].id
			}
			return adjacency[id][i].edge.ID < adjacency[id][j].edge.ID
		})
	}

	all := make([]queryDiscovered, 0, minInt(len(evidence), req.MaxVisited))
	queue := make([]queryDiscovered, 0, minInt(len(evidence), req.MaxVisited))
	seen := map[string]struct{}{}
	exhausted := false
	for _, item := range evidence {
		if _, exists := seen[item.Node.ID]; exists {
			continue
		}
		if len(seen) >= req.MaxVisited {
			exhausted = true
			break
		}
		entry := queryDiscovered{node: item.Node, path: contextual.SupportingPath{NodeID: item.Node.ID, NodeIDs: []string{item.Node.ID}}}
		seen[item.Node.ID] = struct{}{}
		queue = append(queue, entry)
		all = append(all, entry)
	}
	for next := 0; next < len(queue); next++ {
		if err := ctx.Err(); err != nil {
			return all, contextEdges, exhausted, true, err
		}
		current := queue[next]
		if current.path.Distance == req.MaxDepth {
			continue
		}
		for _, candidate := range adjacency[current.node.ID] {
			if _, exists := seen[candidate.id]; exists {
				continue
			}
			if len(seen) >= req.MaxVisited {
				exhausted = true
				continue
			}
			node, ok := s.graph.Nodes[candidate.id]
			if !ok {
				continue
			}
			path := contextual.SupportingPath{
				NodeID: candidate.id, Distance: current.path.Distance + 1,
				NodeIDs: append(append([]string(nil), current.path.NodeIDs...), candidate.id),
				EdgeIDs: append(append([]string(nil), current.path.EdgeIDs...), candidate.edge.ID),
			}
			entry := queryDiscovered{node: node, path: path}
			seen[candidate.id] = struct{}{}
			queue = append(queue, entry)
			all = append(all, entry)
		}
	}
	return all, contextEdges, exhausted, false, nil
}

func selectedContextEdges(edges map[string]repository.Edge, nodeIDs map[string]struct{}) []repository.Edge {
	result := make([]repository.Edge, 0, len(edges))
	for _, edge := range edges {
		if _, source := nodeIDs[edge.Source]; !source {
			continue
		}
		if _, target := nodeIDs[edge.Target]; !target {
			continue
		}
		result = append(result, edge)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
