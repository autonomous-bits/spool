package resolve

import (
	"context"
	"errors"

	"github.com/autonomous-bits/spool/internal/contextual"
	"github.com/autonomous-bits/spool/internal/repository"
)

// MetadataPredicate is a typed equality or range predicate over a schema-indexed
// node property. It deliberately does not expose projection query syntax.
type MetadataPredicate = repository.MetadataPredicate

// SeedSelector selects either lexical evidence or typed metadata-filter evidence.
type SeedSelector = contextual.SeedSelector

// Direction controls graph edge traversal during contextual retrieval.
type Direction = contextual.Direction

const (
	DirectionOut  = contextual.DirectionOut
	DirectionIn   = contextual.DirectionIn
	DirectionBoth = contextual.DirectionBoth
)

// Evidence is a lexical or typed-filter seed match.
type Evidence = contextual.Evidence

// ContextNode is a graph node with its supporting path from a seed.
type ContextNode = contextual.ContextNode

// SupportingPath is the canonical path supporting a contextual node.
type SupportingPath = contextual.SupportingPath

// FilterRequest selects nodes by labels and typed indexed-property predicates.
type FilterRequest struct {
	Selector          SnapshotSelector    `json:"selector"`
	Labels            []string            `json:"labels,omitempty"`
	Predicates        []MetadataPredicate `json:"predicates,omitempty"`
	ContinuationToken string              `json:"continuationToken,omitempty"`
	Budget            QueryBudgetRequest  `json:"budget"`
}

// FilterResult contains filtered nodes and public snapshot provenance.
type FilterResult struct {
	Snapshot          SnapshotMetadata        `json:"snapshot"`
	Projection        ProjectionMetadata      `json:"projection"`
	Budget            QueryBudget             `json:"budget"`
	Completion        QueryCompletionMetadata `json:"completion"`
	Nodes             []repository.Node       `json:"nodes"`
	ContinuationToken string                  `json:"continuationToken,omitempty"`
}

// SearchRequest selects lexical projection matches from a branch snapshot.
type SearchRequest struct {
	Selector          SnapshotSelector   `json:"selector"`
	Query             string             `json:"query"`
	ContinuationToken string             `json:"continuationToken,omitempty"`
	Budget            QueryBudgetRequest `json:"budget"`
}

// SearchResult contains lexical matches and public snapshot provenance.
type SearchResult struct {
	Snapshot          SnapshotMetadata             `json:"snapshot"`
	Projection        ProjectionMetadata           `json:"projection"`
	Budget            QueryBudget                  `json:"budget"`
	Completion        QueryCompletionMetadata      `json:"completion"`
	Matches           []repository.SearchNodeMatch `json:"matches"`
	ContinuationToken string                       `json:"continuationToken,omitempty"`
}

// SearchExpandRequest selects lexical or typed-filter evidence, then expands
// bounded graph context from those seeds.
type SearchExpandRequest struct {
	Selector SnapshotSelector `json:"selector"`
	Seeds    SeedSelector     `json:"seeds"`
	// SeedLimit limits evidence before expansion. Zero uses the effective row budget.
	SeedLimit int                `json:"seedLimit,omitempty"`
	Direction Direction          `json:"direction"`
	EdgeTypes []string           `json:"edgeTypes,omitempty"`
	Budget    QueryBudgetRequest `json:"budget"`
}

// ContextRequest assembles evidence and related graph context with the same
// selection and bounding semantics as SearchExpandRequest.
type ContextRequest = SearchExpandRequest

// SearchExpandResult contains contextual evidence and graph traversal output
// with public snapshot and projection provenance.
type SearchExpandResult struct {
	Snapshot          SnapshotMetadata        `json:"snapshot"`
	Projection        ProjectionMetadata      `json:"projection"`
	Budget            QueryBudget             `json:"budget"`
	Completion        QueryCompletionMetadata `json:"completion"`
	Evidence          []Evidence              `json:"evidence"`
	Nodes             []ContextNode           `json:"nodes"`
	Edges             []repository.Edge       `json:"edges"`
	Paths             []SupportingPath        `json:"paths"`
	CapacityExhausted bool                    `json:"capacityExhausted,omitempty"`
}

// ContextResult is the public context-assembly result.
type ContextResult = SearchExpandResult

// GraphResult contains every node and edge in one immutable branch snapshot.
type GraphResult struct {
	Snapshot SnapshotMetadata  `json:"snapshot"`
	Nodes    []repository.Node `json:"nodes"`
	Edges    []repository.Edge `json:"edges"`
}

// SPLGraph returns the complete graph for a branch snapshot.
func (t *ResolveTool) SPLGraph(ctx context.Context, selector SnapshotSelector) (GraphResult, error) {
	commit, err := t.resolver.resolveSnapshotCommit(ctx, selector)
	if err != nil {
		return GraphResult{}, err
	}
	snapshot, err := t.resolver.snapshotMetadataForCommitContext(ctx, selector.Branch, commit)
	if err != nil {
		return GraphResult{}, err
	}
	nodes, err := t.repository.PinnedNodesContext(ctx, commit)
	if err != nil {
		return GraphResult{}, err
	}
	edges, err := t.repository.PinnedEdgesContext(ctx, commit)
	if err != nil {
		return GraphResult{}, err
	}
	return GraphResult{Snapshot: snapshot, Nodes: nodes, Edges: edges}, nil
}

// SPLFilter returns a bounded page of nodes selected only through the
// branch-head projection's typed filter API.
func (t *ResolveTool) SPLFilter(ctx context.Context, request FilterRequest) (FilterResult, error) {
	budget := NormalizeQueryBudget(request.Budget, t.queryBudget)
	queryCtx, execution, cancel := BeginQuery(ctx, budget)
	defer cancel()
	if err := queryCtx.Err(); err != nil {
		return FilterResult{}, err
	}
	commit, err := t.resolver.resolveSnapshotCommit(queryCtx, request.Selector)
	if err != nil {
		return FilterResult{}, err
	}
	snapshot, err := t.resolver.snapshotMetadataForCommitContext(queryCtx, request.Selector.Branch, commit)
	if err != nil {
		return FilterResult{}, err
	}
	result := FilterResult{
		Snapshot: snapshot, Projection: t.resolver.projectionMetadataForCommitContext(queryCtx, request.Selector.Branch, commit),
		Budget: budget, Completion: queryCompletionTemplate(budget), Nodes: make([]repository.Node, 0),
	}
	payloadBudget, err := publicPayloadBudget(result, filterPayload{}, budget.MaxResponseBytes)
	if err != nil {
		return FilterResult{}, err
	}
	payload, err := t.repository.FilterNodesContext(queryCtx, repository.FilterNodesRequest{
		Branch: request.Selector.Branch, Commit: commit, Labels: request.Labels, Predicates: request.Predicates,
		MaxRows: budget.MaxRows, MaxResponseBytes: payloadBudget, ContinuationToken: request.ContinuationToken,
	})
	timedOut := isTimedOutPrefix(err, len(payload.Nodes) > 0)
	if err != nil && !timedOut {
		return FilterResult{}, err
	}
	result.Nodes, result.ContinuationToken = payload.Nodes, payload.ContinuationToken
	if err := finalizeToolQuery(&result, &result.Completion, execution, budget.MaxResponseBytes,
		payload.ContinuationToken != "" || timedOut, timedOut, len(payload.Nodes)); err != nil {
		return FilterResult{}, err
	}
	return result, nil
}

// SPLSearch returns a bounded page of lexical matches from the branch-head
// projection. Historical commits fail with ErrHistoricalProjectionUnsupported.
func (t *ResolveTool) SPLSearch(ctx context.Context, request SearchRequest) (SearchResult, error) {
	budget := NormalizeQueryBudget(request.Budget, t.queryBudget)
	queryCtx, execution, cancel := BeginQuery(ctx, budget)
	defer cancel()
	if err := queryCtx.Err(); err != nil {
		return SearchResult{}, err
	}
	commit, err := t.resolver.resolveSnapshotCommit(queryCtx, request.Selector)
	if err != nil {
		return SearchResult{}, err
	}
	snapshot, err := t.resolver.snapshotMetadataForCommitContext(queryCtx, request.Selector.Branch, commit)
	if err != nil {
		return SearchResult{}, err
	}
	result := SearchResult{
		Snapshot: snapshot, Projection: t.resolver.projectionMetadataForCommitContext(queryCtx, request.Selector.Branch, commit),
		Budget: budget, Completion: queryCompletionTemplate(budget), Matches: make([]repository.SearchNodeMatch, 0),
	}
	payloadBudget, err := publicPayloadBudget(result, searchPayload{}, budget.MaxResponseBytes)
	if err != nil {
		return SearchResult{}, err
	}
	payload, err := t.repository.SearchNodesContext(queryCtx, repository.SearchNodesRequest{
		Branch: request.Selector.Branch, Commit: commit, Query: request.Query,
		MaxRows: budget.MaxRows, MaxResponseBytes: payloadBudget, ContinuationToken: request.ContinuationToken,
	})
	timedOut := isTimedOutPrefix(err, len(payload.Matches) > 0)
	if err != nil && !timedOut {
		return SearchResult{}, err
	}
	result.Matches, result.ContinuationToken = payload.Matches, payload.ContinuationToken
	if err := finalizeToolQuery(&result, &result.Completion, execution, budget.MaxResponseBytes,
		payload.ContinuationToken != "" || timedOut, timedOut, len(payload.Matches)); err != nil {
		return SearchResult{}, err
	}
	return result, nil
}

// SPLSearchExpand returns lexical or filter evidence plus bounded graph context.
func (t *ResolveTool) SPLSearchExpand(ctx context.Context, request SearchExpandRequest) (SearchExpandResult, error) {
	return t.edgContextual(ctx, request, false)
}

// SPLContext returns evidence-focused bounded graph context.
func (t *ResolveTool) SPLContext(ctx context.Context, request ContextRequest) (ContextResult, error) {
	return t.edgContextual(ctx, request, true)
}

func (t *ResolveTool) edgContextual(ctx context.Context, request SearchExpandRequest, evidenceFirst bool) (SearchExpandResult, error) {
	budget := NormalizeQueryBudget(request.Budget, t.queryBudget)
	queryCtx, execution, cancel := BeginQuery(ctx, budget)
	defer cancel()
	if err := queryCtx.Err(); err != nil {
		return SearchExpandResult{}, err
	}
	commit, err := t.resolver.resolveSnapshotCommit(queryCtx, request.Selector)
	if err != nil {
		return SearchExpandResult{}, err
	}
	snapshot, err := t.resolver.snapshotMetadataForCommitContext(queryCtx, request.Selector.Branch, commit)
	if err != nil {
		return SearchExpandResult{}, err
	}
	result := SearchExpandResult{
		Snapshot: snapshot, Projection: t.resolver.projectionMetadataForCommitContext(queryCtx, request.Selector.Branch, commit),
		Budget: budget, Completion: queryCompletionTemplate(budget),
		Evidence: make([]contextual.Evidence, 0), Nodes: make([]contextual.ContextNode, 0),
		Edges: make([]repository.Edge, 0), Paths: make([]contextual.SupportingPath, 0),
	}
	payloadBudget, err := publicPayloadBudget(result, contextualPayload{}, budget.MaxResponseBytes)
	if err != nil {
		return SearchExpandResult{}, err
	}
	seedLimit := request.SeedLimit
	if seedLimit > budget.MaxRows {
		seedLimit = budget.MaxRows
	}
	inner := contextual.SearchExpandRequest{
		Branch: request.Selector.Branch, Commit: &commit, Seeds: request.Seeds, SeedLimit: seedLimit,
		Direction: request.Direction, EdgeTypes: request.EdgeTypes,
		Budget: contextual.QueryBudget{
			MaxRows: budget.MaxRows, MaxVisited: budget.MaxVisited, MaxDepth: budget.MaxDepth,
			MaxResponseBytes: payloadBudget, Timeout: budget.Timeout,
		},
	}
	var payload contextual.SearchExpandResult
	if evidenceFirst {
		payload, err = contextual.NewService(t.repository).Context(queryCtx, inner)
	} else {
		payload, err = contextual.NewService(t.repository).SearchExpand(queryCtx, inner)
	}
	timedOut := errors.Is(err, context.DeadlineExceeded) && (len(payload.Evidence) > 0 || len(payload.Nodes) > 0)
	if err != nil && !timedOut {
		return SearchExpandResult{}, err
	}
	result.Evidence, result.Nodes, result.Edges, result.Paths = payload.Evidence, payload.Nodes, payload.Edges, payload.Paths
	result.CapacityExhausted = payload.CapacityExhausted
	truncated := payload.Completion.Truncated || timedOut
	if err := finalizeToolQuery(&result, &result.Completion, execution, budget.MaxResponseBytes,
		truncated, timedOut, payload.Completion.Visited); err != nil {
		return SearchExpandResult{}, err
	}
	return result, nil
}

type filterPayload struct {
	Nodes             []repository.Node `json:"nodes"`
	ContinuationToken string            `json:"continuationToken,omitempty"`
}

type searchPayload struct {
	Matches           []repository.SearchNodeMatch `json:"matches"`
	ContinuationToken string                       `json:"continuationToken,omitempty"`
}

type contextualPayload struct {
	Evidence          []contextual.Evidence       `json:"evidence"`
	Nodes             []contextual.ContextNode    `json:"nodes"`
	Edges             []repository.Edge           `json:"edges"`
	Paths             []contextual.SupportingPath `json:"paths"`
	CapacityExhausted bool                        `json:"capacityExhausted,omitempty"`
}
