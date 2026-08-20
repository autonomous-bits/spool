// Package contextual assembles bounded lexical evidence and graph context.
package contextual

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/autonomous-bits/spool/internal/repository"
)

var (
	// ErrInvalidSeedSelector reports a selector that is neither lexical nor a
	// metadata filter, or that attempts to combine the two modes.
	ErrInvalidSeedSelector = errors.New("context seed selector is invalid")
	// ErrInvalidBudget reports a non-positive resource bound or timeout.
	ErrInvalidBudget = errors.New("context query budget is invalid")
	// ErrResponseBudgetTooSmall reports a budget unable to encode response metadata.
	ErrResponseBudgetTooSmall = errors.New("context response budget cannot represent result")
)

// Direction controls which incident edges may be followed.
type Direction string

const (
	DirectionOut  Direction = "out"
	DirectionIn   Direction = "in"
	DirectionBoth Direction = "both"
)

// QueryBudget bounds seed retrieval, graph traversal, response size, and time.
type QueryBudget struct {
	MaxRows          int           `json:"maxRows"`
	MaxVisited       int           `json:"maxVisited"`
	MaxDepth         int           `json:"maxDepth"`
	MaxResponseBytes int           `json:"maxResponseBytes"`
	Timeout          time.Duration `json:"timeout"`
}

// DefaultQueryBudget returns conservative bounds for contextual queries.
func DefaultQueryBudget() QueryBudget {
	return QueryBudget{
		MaxRows: 1_000, MaxVisited: 10_000, MaxDepth: 32,
		MaxResponseBytes: 1 << 20, Timeout: 10 * time.Second,
	}
}

// SeedSelector selects ranked lexical evidence or deterministic metadata matches.
// Query is mutually exclusive with Labels and Predicates.
type SeedSelector struct {
	Query      string                         `json:"query,omitempty"`
	Labels     []string                       `json:"labels,omitempty"`
	Predicates []repository.MetadataPredicate `json:"predicates,omitempty"`
}

// SearchExpandRequest describes a lexical-or-filter seed query followed by
// bounded graph expansion against the same pinned snapshot.
type SearchExpandRequest struct {
	Branch string `json:"branch"`
	// Commit optionally identifies a reachable commit. The projection is
	// branch-head-only, so a historical commit is intentionally rejected by
	// seed retrieval with repository.ErrHistoricalProjectionUnsupported.
	Commit *repository.ObjectID `json:"commit,omitempty"`
	Seeds  SeedSelector         `json:"seeds"`
	// SeedLimit limits evidence retrieved before graph expansion. Zero uses
	// Budget.MaxRows for backward-compatible bounded seed retrieval.
	SeedLimit int         `json:"seedLimit,omitempty"`
	Direction Direction   `json:"direction"`
	EdgeTypes []string    `json:"edgeTypes,omitempty"`
	Budget    QueryBudget `json:"budget"`
}

// ContextRequest describes context assembly. It has the same execution
// semantics as SearchExpand, but its response prioritizes seed evidence.
type ContextRequest = SearchExpandRequest

// SnapshotMetadata identifies the immutable snapshot used for all returned data.
type SnapshotMetadata struct {
	Branch   string              `json:"branch"`
	Commit   repository.ObjectID `json:"commit"`
	Snapshot repository.ObjectID `json:"snapshot"`
}

// ProjectionMetadata identifies the branch-head projection used for seed retrieval.
type ProjectionMetadata struct {
	State    string              `json:"state"`
	NodeRoot repository.ObjectID `json:"nodeRoot"`
}

// Completion describes the bounded execution and encoded response.
type Completion struct {
	Complete      bool `json:"complete"`
	Truncated     bool `json:"truncated"`
	TimedOut      bool `json:"timedOut"`
	Visited       int  `json:"visited"`
	ReturnedRows  int  `json:"returnedRows"`
	ResponseBytes int  `json:"responseBytes"`
}

// Evidence is a lexical match or a metadata-filter seed. Score and snippet
// fields are populated only for lexical evidence.
type Evidence struct {
	Node          repository.Node   `json:"node"`
	Score         float64           `json:"score,omitempty"`
	MatchedFields []string          `json:"matchedFields,omitempty"`
	Snippets      map[string]string `json:"snippets,omitempty"`
}

// SupportingPath is the canonical shortest path supporting a returned node.
type SupportingPath struct {
	NodeID   string   `json:"nodeId"`
	NodeIDs  []string `json:"nodeIds"`
	EdgeIDs  []string `json:"edgeIds"`
	Distance int      `json:"distance"`
}

// ContextNode contains a node and its canonical supporting path. Seed nodes
// have a zero-length path and appear in seed evidence order.
type ContextNode struct {
	Node repository.Node `json:"node"`
	Path SupportingPath  `json:"path"`
}

// SearchExpandResult is the bounded graph expansion response.
type SearchExpandResult struct {
	Snapshot          SnapshotMetadata   `json:"snapshot"`
	Projection        ProjectionMetadata `json:"projection"`
	Budget            QueryBudget        `json:"budget"`
	Completion        Completion         `json:"completion"`
	Evidence          []Evidence         `json:"evidence"`
	Nodes             []ContextNode      `json:"nodes"`
	Edges             []repository.Edge  `json:"edges"`
	Paths             []SupportingPath   `json:"paths"`
	CapacityExhausted bool               `json:"capacityExhausted,omitempty"`
}

// ContextResult is the assembled evidence context response.
type ContextResult = SearchExpandResult

// Service implements contextual graph query use cases.
type Service struct {
	repo *repository.Repository
}

// NewService constructs contextual use cases over repo.
func NewService(repo *repository.Repository) *Service {
	return &Service{repo: repo}
}

// SearchExpand retrieves lexical or metadata seeds and expands their bounded graph context.
func (s *Service) SearchExpand(ctx context.Context, request SearchExpandRequest) (SearchExpandResult, error) {
	return s.execute(ctx, request)
}

// Context retrieves ranked seed evidence first, then bounded related graph context.
func (s *Service) Context(ctx context.Context, request ContextRequest) (ContextResult, error) {
	return s.execute(ctx, request)
}

type discovered struct {
	node repository.Node
	path SupportingPath
}

type neighbor struct {
	id   string
	edge repository.Edge
}

func (s *Service) execute(ctx context.Context, request SearchExpandRequest) (SearchExpandResult, error) {
	if s == nil || s.repo == nil {
		return SearchExpandResult{}, errors.New("context repository is required")
	}
	if err := validateRequest(request); err != nil {
		return SearchExpandResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, request.Budget.Timeout)
	defer cancel()

	commit, err := s.resolveCommit(ctx, request)
	if err != nil {
		return SearchExpandResult{}, err
	}
	evidence, projectionSnapshot, moreSeeds, err := s.seedEvidence(ctx, commit, request)
	if err != nil {
		return SearchExpandResult{}, err
	}
	record, err := s.repo.PinnedSnapshotRecordContext(ctx, commit)
	if err != nil {
		return SearchExpandResult{}, err
	}
	if projectionSnapshot != record.Snapshot {
		return SearchExpandResult{}, fmt.Errorf("projection snapshot %q differs from pinned snapshot %q", projectionSnapshot, record.Snapshot)
	}
	edges, err := s.repo.PinnedEdgesContext(ctx, commit)
	if err != nil {
		return SearchExpandResult{}, err
	}
	all, contextEdges, exhausted, timedOut, err := s.expand(ctx, commit, evidence, edges, request)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return SearchExpandResult{}, err
	}

	result := SearchExpandResult{
		Snapshot:   SnapshotMetadata{Branch: request.Branch, Commit: commit, Snapshot: record.Snapshot},
		Projection: ProjectionMetadata{State: "ready", NodeRoot: record.NodeRoot},
		Budget:     request.Budget, Evidence: evidence,
		CapacityExhausted: exhausted,
		Completion: Completion{
			Complete:  !moreSeeds && !exhausted && !timedOut,
			Truncated: moreSeeds || exhausted || timedOut,
			TimedOut:  timedOut, Visited: len(all),
		},
	}
	result, boundedErr := boundedResult(result, all, contextEdges)
	if boundedErr != nil {
		return SearchExpandResult{}, boundedErr
	}
	if timedOut {
		return result, context.DeadlineExceeded
	}
	return result, nil
}

func (s *Service) resolveCommit(ctx context.Context, request SearchExpandRequest) (repository.ObjectID, error) {
	if request.Commit == nil {
		return s.repo.PinBranchContext(ctx, request.Branch)
	}
	return s.repo.ResolveExplicitCommitContext(ctx, request.Branch, *request.Commit, false)
}

func validateRequest(request SearchExpandRequest) error {
	if request.Branch == "" || request.Direction != DirectionOut && request.Direction != DirectionIn && request.Direction != DirectionBoth {
		return ErrInvalidSeedSelector
	}
	if request.Seeds.Query == "" && len(request.Seeds.Labels) == 0 && len(request.Seeds.Predicates) == 0 {
		return ErrInvalidSeedSelector
	}
	if request.Seeds.Query != "" && (len(request.Seeds.Labels) != 0 || len(request.Seeds.Predicates) != 0) {
		return ErrInvalidSeedSelector
	}
	if request.SeedLimit < 0 || request.Budget.MaxRows <= 0 || request.Budget.MaxVisited <= 0 || request.Budget.MaxDepth < 0 ||
		request.Budget.MaxResponseBytes <= 0 || request.Budget.Timeout <= 0 {
		return ErrInvalidBudget
	}
	seen := make(map[string]struct{}, len(request.EdgeTypes))
	for _, edgeType := range request.EdgeTypes {
		if edgeType == "" {
			return ErrInvalidSeedSelector
		}
		if _, exists := seen[edgeType]; exists {
			return ErrInvalidSeedSelector
		}
		seen[edgeType] = struct{}{}
	}
	return nil
}

func (s *Service) seedEvidence(ctx context.Context, commit repository.ObjectID, request SearchExpandRequest) ([]Evidence, repository.ObjectID, bool, error) {
	seedLimit := request.SeedLimit
	if seedLimit == 0 || seedLimit > request.Budget.MaxRows {
		seedLimit = request.Budget.MaxRows
	}
	if request.Seeds.Query != "" {
		result, err := s.repo.SearchNodesContext(ctx, repository.SearchNodesRequest{
			Branch: request.Branch, Commit: commit, Query: request.Seeds.Query,
			MaxRows: seedLimit, MaxResponseBytes: request.Budget.MaxResponseBytes,
		})
		if err != nil {
			return nil, "", false, err
		}
		evidence := make([]Evidence, len(result.Matches))
		for i, match := range result.Matches {
			evidence[i] = Evidence{Node: match.Node, Score: match.Score, MatchedFields: match.MatchedFields, Snippets: match.Snippets}
		}
		return evidence, result.Snapshot, result.ContinuationToken != "", nil
	}
	result, err := s.repo.FilterNodesContext(ctx, repository.FilterNodesRequest{
		Branch: request.Branch, Commit: commit, Labels: request.Seeds.Labels, Predicates: request.Seeds.Predicates,
		MaxRows: seedLimit, MaxResponseBytes: request.Budget.MaxResponseBytes,
	})
	if err != nil {
		return nil, "", false, err
	}
	evidence := make([]Evidence, len(result.Nodes))
	for i, node := range result.Nodes {
		evidence[i] = Evidence{Node: node}
	}
	return evidence, result.Snapshot, result.ContinuationToken != "", nil
}

func (s *Service) expand(ctx context.Context, commit repository.ObjectID, evidence []Evidence, edges []repository.Edge, request SearchExpandRequest) ([]discovered, map[string]repository.Edge, bool, bool, error) {
	adjacency := make(map[string][]neighbor)
	contextEdges := make(map[string]repository.Edge)
	allowed := make(map[string]struct{}, len(request.EdgeTypes))
	for _, edgeType := range request.EdgeTypes {
		allowed[edgeType] = struct{}{}
	}
	for _, edge := range edges {
		if err := ctx.Err(); err != nil {
			return nil, nil, false, errors.Is(err, context.DeadlineExceeded), err
		}
		if len(allowed) > 0 {
			if _, ok := allowed[edge.Type]; !ok {
				continue
			}
		}
		contextEdges[edge.ID] = edge
		if request.Direction == DirectionOut || request.Direction == DirectionBoth {
			adjacency[edge.Source] = append(adjacency[edge.Source], neighbor{id: edge.Target, edge: edge})
		}
		if request.Direction == DirectionIn || request.Direction == DirectionBoth {
			adjacency[edge.Target] = append(adjacency[edge.Target], neighbor{id: edge.Source, edge: edge})
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

	all := make([]discovered, 0, min(len(evidence), request.Budget.MaxVisited))
	queue := make([]discovered, 0, min(len(evidence), request.Budget.MaxVisited))
	seen := make(map[string]struct{}, request.Budget.MaxVisited)
	exhausted := false
	for _, item := range evidence {
		if _, exists := seen[item.Node.ID]; exists {
			continue
		}
		if len(seen) >= request.Budget.MaxVisited {
			exhausted = true
			break
		}
		entry := discovered{node: item.Node, path: SupportingPath{NodeID: item.Node.ID, NodeIDs: []string{item.Node.ID}}}
		seen[item.Node.ID] = struct{}{}
		queue = append(queue, entry)
		all = append(all, entry)
	}
	for next := 0; next < len(queue); next++ {
		if err := ctx.Err(); err != nil {
			return all, contextEdges, exhausted, errors.Is(err, context.DeadlineExceeded), err
		}
		current := queue[next]
		if current.path.Distance == request.Budget.MaxDepth {
			continue
		}
		for _, candidate := range adjacency[current.node.ID] {
			if err := ctx.Err(); err != nil {
				return all, contextEdges, exhausted, errors.Is(err, context.DeadlineExceeded), err
			}
			if _, exists := seen[candidate.id]; exists {
				continue
			}
			if len(seen) >= request.Budget.MaxVisited {
				exhausted = true
				continue
			}
			resolved, err := s.repo.ResolvePinnedContext(ctx, commit, candidate.id)
			if err != nil {
				return all, contextEdges, exhausted, errors.Is(err, context.DeadlineExceeded), err
			}
			path := SupportingPath{
				NodeID: candidate.id, Distance: current.path.Distance + 1,
				NodeIDs: append(append([]string(nil), current.path.NodeIDs...), candidate.id),
				EdgeIDs: append(append([]string(nil), current.path.EdgeIDs...), candidate.edge.ID),
			}
			entry := discovered{node: resolved.Node, path: path}
			seen[candidate.id] = struct{}{}
			queue = append(queue, entry)
			all = append(all, entry)
		}
	}
	return all, contextEdges, exhausted, false, nil
}

func boundedResult(result SearchExpandResult, all []discovered, contextEdges map[string]repository.Edge) (SearchExpandResult, error) {
	if !finalizeResponseBytes(&result, result.Budget.MaxResponseBytes) {
		return SearchExpandResult{}, ErrResponseBudgetTooSmall
	}
	for _, entry := range all {
		if len(result.Nodes) == result.Budget.MaxRows {
			result.Completion.Truncated = true
			result.Completion.Complete = false
			break
		}
		candidate := result
		candidate.Nodes = append(append([]ContextNode(nil), result.Nodes...), ContextNode{Node: entry.node, Path: entry.path})
		candidate.Paths = append(append([]SupportingPath(nil), result.Paths...), entry.path)
		nodeIDs := make(map[string]struct{}, len(candidate.Nodes))
		for _, node := range candidate.Nodes {
			nodeIDs[node.Node.ID] = struct{}{}
		}
		candidate.Edges = selectedEdges(contextEdges, nodeIDs)
		candidate.Completion.ReturnedRows = len(candidate.Nodes)
		if !finalizeResponseBytes(&candidate, candidate.Budget.MaxResponseBytes) {
			result.Completion.Truncated = true
			result.Completion.Complete = false
			break
		}
		result = candidate
	}
	if len(result.Nodes) < len(all) {
		result.Completion.Truncated = true
		result.Completion.Complete = false
		if len(result.Nodes) == 0 {
			return SearchExpandResult{}, ErrResponseBudgetTooSmall
		}
	}
	if !finalizeResponseBytes(&result, result.Budget.MaxResponseBytes) {
		return SearchExpandResult{}, ErrResponseBudgetTooSmall
	}
	return result, nil
}

func selectedEdges(edges map[string]repository.Edge, nodeIDs map[string]struct{}) []repository.Edge {
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

func finalizeResponseBytes(result *SearchExpandResult, limit int) bool {
	for attempts := 0; attempts < 16; attempts++ {
		data, err := json.Marshal(*result)
		if err != nil || len(data) > limit {
			return false
		}
		if result.Completion.ResponseBytes == len(data) {
			return true
		}
		result.Completion.ResponseBytes = len(data)
	}
	return false
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
