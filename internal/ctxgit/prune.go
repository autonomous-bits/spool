package ctxgit

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/autonomous-bits/spool/internal/repository"
)

// PruneRequest describes bound graph cleanup of Ephemeral nodes.
type PruneRequest struct {
	DryRun  bool   `json:"dryRun,omitempty"`
	Author  string `json:"author,omitempty"`
	Message string `json:"message,omitempty"`
}

// PruneResult summarizes ephemeral nodes and cascading edges removed from the
// bound checkout. Non-dry-run results include the short-lived branch and PR.
type PruneResult struct {
	Branch               string      `json:"branch"`
	Commit               string      `json:"commit,omitempty"`
	DryRun               bool        `json:"dryRun,omitempty"`
	PrunedNodesCount     int         `json:"prunedNodesCount"`
	PrunedEdgesCount     int         `json:"prunedEdgesCount"`
	PrunedNodeIDs        []string    `json:"prunedNodeIds"`
	OrphanedDurableNodes []string    `json:"orphanedDurableNodes"`
	PullRequest          PullRequest `json:"pullRequest,omitempty"`
}

// Prune removes Ephemeral-labeled nodes and their incident edges from the bound
// checkout. Writes go through a short-lived branch and pull request. It never
// touches CAS packs, gc, or leftover .spl object stores.
func (s *Session) Prune(ctx context.Context, request PruneRequest) (PruneResult, error) {
	if s == nil || s.CodeRoot == "" {
		return PruneResult{}, UnboundError()
	}
	if _, _, _, err := FindBind(s.CodeRoot); err != nil {
		return PruneResult{}, err
	}
	if !s.Bound() {
		return PruneResult{}, UnboundError()
	}
	if err := ctx.Err(); err != nil {
		return PruneResult{}, err
	}
	if err := s.syncProtected(ctx); err != nil {
		return PruneResult{}, err
	}
	graph, err := LoadGraph(s.CheckoutDir)
	if err != nil {
		return PruneResult{}, err
	}
	s.graph = graph
	if err := s.rebuildProjection(ctx, s.head); err != nil {
		return PruneResult{}, fmt.Errorf("rebuild local projection from checkout: %w", err)
	}

	prunedNodeIDs, prunedEdgeIDs, orphans := pruneSet(graph)
	result := PruneResult{
		Branch:               s.Bind.ProtectedBranch,
		Commit:               s.head,
		DryRun:               request.DryRun,
		PrunedNodesCount:     len(prunedNodeIDs),
		PrunedEdgesCount:     len(prunedEdgeIDs),
		PrunedNodeIDs:        prunedNodeIDs,
		OrphanedDurableNodes: orphans,
	}
	if len(prunedNodeIDs) == 0 || request.DryRun {
		return result, nil
	}

	ops := make([]repository.MutationOperation, 0, len(prunedNodeIDs)+len(prunedEdgeIDs))
	for _, id := range prunedNodeIDs {
		ops = append(ops, repository.MutationOperation{Action: "delete", Entity: "node", ID: id})
	}
	for _, id := range prunedEdgeIDs {
		ops = append(ops, repository.MutationOperation{Action: "delete", Entity: "edge", ID: id})
	}
	normalized, err := normalizeOperations(ops)
	if err != nil {
		return PruneResult{}, err
	}
	if err := validateBatch(graph, normalized); err != nil {
		return PruneResult{}, err
	}
	after, _, err := applyOperations(graph, normalized)
	if err != nil {
		return PruneResult{}, err
	}
	message := strings.TrimSpace(request.Message)
	if message == "" {
		message = "Prune ephemeral entities"
	}
	write, err := s.CommitGraph(ctx, after, request.Author, message, s.Bind.ProtectedBranch)
	if err != nil {
		return PruneResult{}, err
	}
	result.Branch = write.Branch
	result.Commit = write.Commit
	result.DryRun = false
	result.PullRequest = write.PR
	return result, nil
}

func pruneSet(graph *Graph) (prunedNodes, prunedEdges, orphans []string) {
	isEphemeral := map[string]bool{}
	nodeIDs := make([]string, 0, len(graph.Nodes))
	for id := range graph.Nodes {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)
	for _, id := range nodeIDs {
		if hasEphemeralLabel(graph.Nodes[id]) {
			prunedNodes = append(prunedNodes, id)
			isEphemeral[id] = true
		}
	}
	edgeIDs := make([]string, 0, len(graph.Edges))
	for id := range graph.Edges {
		edgeIDs = append(edgeIDs, id)
	}
	sort.Strings(edgeIDs)
	for _, id := range edgeIDs {
		edge := graph.Edges[id]
		if isEphemeral[edge.Source] || isEphemeral[edge.Target] {
			prunedEdges = append(prunedEdges, id)
		}
	}

	durableInitial := map[string]int{}
	durableRemaining := map[string]int{}
	for _, edge := range graph.Edges {
		if !isEphemeral[edge.Source] {
			durableInitial[edge.Source]++
		}
		if !isEphemeral[edge.Target] {
			durableInitial[edge.Target]++
		}
		if !isEphemeral[edge.Source] && !isEphemeral[edge.Target] {
			durableRemaining[edge.Source]++
			durableRemaining[edge.Target]++
		}
	}
	for _, id := range nodeIDs {
		if isEphemeral[id] {
			continue
		}
		if durableInitial[id] > 0 && durableRemaining[id] == 0 {
			orphans = append(orphans, id)
		}
	}
	if prunedNodes == nil {
		prunedNodes = []string{}
	}
	if prunedEdges == nil {
		prunedEdges = []string{}
	}
	if orphans == nil {
		orphans = []string{}
	}
	return prunedNodes, prunedEdges, orphans
}

func hasEphemeralLabel(node repository.Node) bool {
	label := repository.UniversalModifierLabel
	for _, have := range node.Labels {
		if have == label {
			return true
		}
	}
	return false
}
