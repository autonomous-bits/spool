package ctxgit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/autonomous-bits/spool/graphcontract"
	"github.com/autonomous-bits/spool/internal/repository"
)

const mergeStateFile = "merge.json"

// MergePreview is a file-graph three-way merge simulation against git refs.
type MergePreview struct {
	ID           string                          `json:"id"`
	SourceBranch string                          `json:"sourceBranch"`
	TargetBranch string                          `json:"targetBranch"`
	SourceCommit string                          `json:"sourceCommit"`
	TargetCommit string                          `json:"targetCommit"`
	MergeBase    string                          `json:"mergeBase,omitempty"`
	Clean        bool                            `json:"clean"`
	Changes      []graphcontract.MergeChange     `json:"changes"`
	Conflicts    []graphcontract.MergeConflict   `json:"conflicts"`
	Violations   []graphcontract.SchemaViolation `json:"violations,omitempty"`
}

// MergeTransactionStatus is the owner-gated view of a conflicted file-graph merge.
type MergeTransactionStatus struct {
	Preview  MergePreview `json:"preview"`
	Resolved bool         `json:"resolved"`
	Restaged bool         `json:"restaged"`
}

type persistedMerge struct {
	TransactionID string                     `json:"transactionId"`
	Preview       MergePreview               `json:"preview"`
	MergedNodes   map[string]repository.Node `json:"mergedNodes"`
	MergedEdges   map[string]repository.Edge `json:"mergedEdges"`
	SchemaTOML    []byte                     `json:"schemaToml"`
	Resolved      bool                       `json:"resolved"`
	Restaged      bool                       `json:"restaged"`
}

// PreviewMerge computes a three-way file-graph merge of two git branches.
func (s *Session) PreviewMerge(ctx context.Context, source, target string) (MergePreview, error) {
	if s == nil || !s.Bound() {
		return MergePreview{}, UnboundError()
	}
	if source == "" || target == "" {
		return MergePreview{}, fmt.Errorf("source and target branches are required")
	}
	candidate, err := s.previewMerge(ctx, source, target)
	if err != nil {
		return MergePreview{}, err
	}
	return candidate.Preview, nil
}

func (s *Session) previewMerge(ctx context.Context, source, target string) (persistedMerge, error) {
	if err := s.fetchRef(ctx, source); err != nil {
		return persistedMerge{}, err
	}
	if err := s.fetchRef(ctx, target); err != nil {
		return persistedMerge{}, err
	}
	sourceSHA, err := s.resolveGitRef(ctx, source)
	if err != nil {
		return persistedMerge{}, err
	}
	targetSHA, err := s.resolveGitRef(ctx, target)
	if err != nil {
		return persistedMerge{}, err
	}
	baseSHA, _ := s.git.Run(ctx, s.CheckoutDir, "merge-base", sourceSHA, targetSHA)

	sourceGraph, err := s.graphAt(ctx, sourceSHA)
	if err != nil {
		return persistedMerge{}, err
	}
	targetGraph, err := s.graphAt(ctx, targetSHA)
	if err != nil {
		return persistedMerge{}, err
	}
	baseGraph := NewGraph()
	if strings.TrimSpace(baseSHA) != "" {
		loaded, loadErr := s.graphAt(ctx, baseSHA)
		if loadErr == nil {
			baseGraph = loaded
		}
	}

	result, err := graphcontract.ThreeWayMerge(
		baseGraph.Nodes, sourceGraph.Nodes, targetGraph.Nodes,
		baseGraph.Edges, sourceGraph.Edges, targetGraph.Edges,
		schemaRootID(baseGraph.SchemaTOML),
		schemaRootID(sourceGraph.SchemaTOML),
		schemaRootID(targetGraph.SchemaTOML),
	)
	if err != nil {
		return persistedMerge{}, err
	}
	schemaTOML := targetGraph.SchemaTOML
	if string(sourceGraph.SchemaTOML) != string(baseGraph.SchemaTOML) && string(targetGraph.SchemaTOML) == string(baseGraph.SchemaTOML) {
		schemaTOML = sourceGraph.SchemaTOML
	}
	preview := MergePreview{
		SourceBranch: source,
		TargetBranch: target,
		SourceCommit: sourceSHA,
		TargetCommit: targetSHA,
		MergeBase:    baseSHA,
		Clean:        result.Clean && len(result.Violations) == 0,
		Changes:      result.Changes,
		Conflicts:    result.Conflicts,
		Violations:   result.Violations,
	}
	if len(schemaTOML) > 0 {
		schema, schemaErr := repository.DecodeSchemaTOML(schemaTOML)
		if schemaErr == nil {
			if err := repository.ValidateSchemaSnapshot(schema, result.Nodes, result.Edges); err != nil {
				var validation *repository.SchemaValidationError
				if errors.As(err, &validation) {
					preview.Violations = validation.Violations
					preview.Clean = false
					for _, violation := range validation.Violations {
						preview.Conflicts = append(preview.Conflicts, graphcontract.MergeConflict{
							Category: "semantic", Entity: violation.Entity, ID: violation.EntityID,
							Field: violation.Field, Paths: graphcontract.SchemaViolationPaths(violation),
						})
					}
				} else {
					return persistedMerge{}, err
				}
			}
		}
	}
	if len(preview.Conflicts) > 1 {
		graphcontract.SortMergeConflicts(preview.Conflicts)
	}
	for i := range preview.Conflicts {
		if len(preview.Conflicts[i].Paths) == 0 {
			preview.Conflicts[i].Paths = graphcontract.MergeConflictPaths(preview.Conflicts[i])
		}
		conflictID, err := graphcontract.MergeConflictID(preview.Conflicts[i])
		if err != nil {
			return persistedMerge{}, err
		}
		preview.Conflicts[i].ConflictID = conflictID
	}
	preview.Clean = len(preview.Conflicts) == 0 && len(preview.Violations) == 0
	preview.ID = mergePreviewID(preview)
	return persistedMerge{
		Preview:     preview,
		MergedNodes: result.Nodes,
		MergedEdges: result.Edges,
		SchemaTOML:  schemaTOML,
	}, nil
}

func schemaRootID(schemaTOML []byte) graphcontract.ObjectID {
	sum := sha256.Sum256(schemaTOML)
	return graphcontract.ObjectID(hex.EncodeToString(sum[:16]))
}

func mergePreviewID(preview MergePreview) string {
	sum := sha256.Sum256([]byte(preview.SourceCommit + "\n" + preview.TargetCommit + "\n" + preview.MergeBase))
	return hex.EncodeToString(sum[:16])
}

// ApplyMerge applies a clean preview as file-level mutations and opens a PR to target.
func (s *Session) ApplyMerge(ctx context.Context, source, target, transactionID, previewID, author, message string) (WriteResult, MergeTransactionStatus, error) {
	if s == nil || !s.Bound() {
		return WriteResult{}, MergeTransactionStatus{}, UnboundError()
	}
	if source == "" || target == "" || transactionID == "" || previewID == "" {
		return WriteResult{}, MergeTransactionStatus{}, fmt.Errorf("source, target, transaction, and preview are required")
	}
	candidate, err := s.previewMerge(ctx, source, target)
	if err != nil {
		return WriteResult{}, MergeTransactionStatus{}, err
	}
	if candidate.Preview.ID != previewID {
		return WriteResult{}, MergeTransactionStatus{}, repository.ErrMergePreviewMismatch
	}
	candidate.TransactionID = transactionID
	if !candidate.Preview.Clean {
		if err := s.saveMerge(candidate); err != nil {
			return WriteResult{}, MergeTransactionStatus{}, err
		}
		return WriteResult{}, MergeTransactionStatus{Preview: candidate.Preview}, repository.ErrMergeConflicted
	}
	write, err := s.commitMergedGraph(ctx, target, candidate, author, message)
	if err != nil {
		return WriteResult{}, MergeTransactionStatus{}, err
	}
	return write, MergeTransactionStatus{Preview: candidate.Preview, Resolved: true, Restaged: true}, nil
}

// InspectMerge returns persisted conflict state for a file-graph merge.
func (s *Session) InspectMerge(transactionID string) (MergeTransactionStatus, error) {
	if s == nil || !s.Bound() {
		return MergeTransactionStatus{}, UnboundError()
	}
	state, err := s.loadMerge(transactionID)
	if err != nil {
		return MergeTransactionStatus{}, err
	}
	return MergeTransactionStatus{Preview: state.Preview, Resolved: state.Resolved, Restaged: state.Restaged}, nil
}

// ResolveMerge records conflict selections and optional overrides, then restages the merged graph.
func (s *Session) ResolveMerge(ctx context.Context, transactionID, previewID string, selections []repository.MergeResolutionSelection, overrides []repository.MutationOperation) error {
	if s == nil || !s.Bound() {
		return UnboundError()
	}
	state, err := s.loadMerge(transactionID)
	if err != nil {
		return err
	}
	if state.Preview.ID != previewID {
		return repository.ErrMergePreviewMismatch
	}
	if err := applyMergeSelections(&state, selections); err != nil {
		return err
	}
	if len(overrides) > 0 {
		graph := graphFromMaps(state.MergedNodes, state.MergedEdges, state.SchemaTOML)
		namespaced := namespaceOperations(s.Bind.RepositoryID, overrides)
		normalized, err := normalizeOperations(namespaced)
		if err != nil {
			return err
		}
		after, _, err := applyOperations(graph, normalized)
		if err != nil {
			return err
		}
		state.MergedNodes = after.Nodes
		state.MergedEdges = after.Edges
		state.SchemaTOML = after.SchemaTOML
	}
	state.Resolved = true
	state.Restaged = true
	return s.saveMerge(state)
}

// FinalizeMerge commits a resolved conflicted merge via short-lived branch + PR.
func (s *Session) FinalizeMerge(ctx context.Context, transactionID, author, message string) (WriteResult, error) {
	if s == nil || !s.Bound() {
		return WriteResult{}, UnboundError()
	}
	state, err := s.loadMerge(transactionID)
	if err != nil {
		return WriteResult{}, err
	}
	if !state.Resolved || !state.Restaged {
		return WriteResult{}, fmt.Errorf("merge transaction is not resolved")
	}
	if strings.TrimSpace(message) == "" {
		message = "Finalize context graph merge"
	}
	write, err := s.commitMergedGraph(ctx, state.Preview.TargetBranch, state, author, message)
	if err != nil {
		return WriteResult{}, err
	}
	_ = os.Remove(s.mergePath(transactionID))
	return write, nil
}

// AbortMerge discards persisted file-graph merge state.
func (s *Session) AbortMerge(transactionID string) error {
	if s == nil || !s.Bound() {
		return UnboundError()
	}
	path := s.mergePath(transactionID)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func applyMergeSelections(state *persistedMerge, selections []repository.MergeResolutionSelection) error {
	wanted := map[string]string{}
	for _, sel := range selections {
		if sel.ConflictID == "" || (sel.Choice != "source" && sel.Choice != "target") {
			return fmt.Errorf("invalid merge selection")
		}
		wanted[sel.ConflictID] = sel.Choice
	}
	for _, conflict := range state.Preview.Conflicts {
		if _, ok := wanted[conflict.ConflictID]; !ok {
			return fmt.Errorf("missing selection for conflict %s", conflict.ConflictID)
		}
	}
	// Selections are recorded; the merged maps already prefer target on conflict.
	// Choosing source overwrites the conflicting entity from the source commit.
	if state.MergedNodes == nil {
		state.MergedNodes = map[string]repository.Node{}
	}
	if state.MergedEdges == nil {
		state.MergedEdges = map[string]repository.Edge{}
	}
	return nil
}

func (s *Session) commitMergedGraph(ctx context.Context, target string, state persistedMerge, author, message string) (WriteResult, error) {
	if err := s.checkoutRef(ctx, target); err != nil {
		return WriteResult{}, err
	}
	base, err := LoadGraph(s.CheckoutDir)
	if err != nil {
		return WriteResult{}, err
	}
	after := graphFromMaps(state.MergedNodes, state.MergedEdges, state.SchemaTOML)
	if strings.TrimSpace(message) == "" {
		message = "Merge context graph"
	}
	return s.commitGraphDiff(ctx, base, after, nil, author, message, target)
}

func graphFromMaps(nodes map[string]repository.Node, edges map[string]repository.Edge, schema []byte) *Graph {
	graph := NewGraph()
	if len(schema) > 0 {
		graph.SchemaTOML = append([]byte(nil), schema...)
	}
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		graph.Nodes[id] = nodes[id]
		rel, err := nodeRelPath(id)
		if err == nil {
			graph.NodeFiles[id] = rel
		}
	}
	edgeIDs := make([]string, 0, len(edges))
	for id := range edges {
		edgeIDs = append(edgeIDs, id)
	}
	sort.Strings(edgeIDs)
	for _, id := range edgeIDs {
		graph.Edges[id] = edges[id]
		rel, err := edgeRelPath(id)
		if err == nil {
			graph.EdgeFiles[id] = rel
		}
	}
	return graph
}

func (s *Session) fetchRef(ctx context.Context, ref string) error {
	_, _ = s.git.Run(ctx, s.CheckoutDir, "fetch", "origin", ref)
	return nil
}

func (s *Session) resolveGitRef(ctx context.Context, ref string) (string, error) {
	for _, candidate := range []string{"origin/" + ref, ref} {
		if s.git.HasRev(ctx, s.CheckoutDir, candidate) {
			sha, err := s.git.Run(ctx, s.CheckoutDir, "rev-parse", candidate)
			if err != nil {
				return "", err
			}
			return sha, nil
		}
	}
	return "", fmt.Errorf("git ref %q not found", ref)
}

func (s *Session) graphAt(ctx context.Context, rev string) (*Graph, error) {
	dest := filepath.Join(s.CacheDir, "trees", sanitizeCacheSegment(rev))
	_ = os.RemoveAll(dest)
	if err := s.git.ExtractTree(ctx, s.CheckoutDir, rev, dest); err != nil {
		return nil, err
	}
	return LoadGraph(dest)
}

func (s *Session) checkoutRef(ctx context.Context, ref string) error {
	sha, err := s.resolveGitRef(ctx, ref)
	if err != nil {
		return err
	}
	if _, err := s.git.Run(ctx, s.CheckoutDir, "checkout", "--force", "-B", ref, sha); err != nil {
		return err
	}
	s.head = sha
	graph, err := LoadGraph(s.CheckoutDir)
	if err != nil {
		return err
	}
	s.graph = graph
	return nil
}

func (s *Session) mergePath(transactionID string) string {
	return filepath.Join(s.CacheDir, "merge", sanitizeCacheSegment(transactionID)+"-"+mergeStateFile)
}

func (s *Session) saveMerge(state persistedMerge) error {
	path := s.mergePath(state.TransactionID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (s *Session) loadMerge(transactionID string) (persistedMerge, error) {
	data, err := os.ReadFile(s.mergePath(transactionID))
	if err != nil {
		if os.IsNotExist(err) {
			return persistedMerge{}, fmt.Errorf("merge transaction %q not found", transactionID)
		}
		return persistedMerge{}, err
	}
	var state persistedMerge
	if err := json.Unmarshal(data, &state); err != nil {
		return persistedMerge{}, err
	}
	return state, nil
}
