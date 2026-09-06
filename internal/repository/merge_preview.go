package repository

import (
	"errors"
	"fmt"

	"github.com/autonomous-bits/spool/graphcontract"
)

var (
	// ErrMergePreviewNotClean reports an attempt to apply a preview with conflicts.
	ErrMergePreviewNotClean = errors.New("merge preview is not clean")
	// ErrMergePreviewMismatch reports an apply request that does not name the current preview.
	ErrMergePreviewMismatch = errors.New("merge preview identifier does not match")
)

type (
	// MergeConflict describes a deterministic three-way merge disagreement.
	MergeConflict = graphcontract.MergeConflict
	// MergeChange describes an entity changed from the target snapshot by a preview.
	MergeChange = graphcontract.MergeChange
)

// MergePreview is an immutable, deterministic prediction of merging SourceBranch into TargetBranch.
type MergePreview struct {
	ID           ObjectID            `json:"id"`
	Binding      MergePreviewBinding `json:"binding"`
	SourceBranch string              `json:"sourceBranch"`
	TargetBranch string              `json:"targetBranch"`
	Clean        bool                `json:"clean"`
	Changes      []MergeChange       `json:"changes"`
	Conflicts    []MergeConflict     `json:"conflicts"`
	Violations   []SchemaViolation   `json:"violations,omitempty"`
}

type mergeCandidate struct {
	nodes      map[string]Node
	edges      map[string]Edge
	schemaRoot ObjectID
	preview    MergePreview
}

// PreviewMerge computes a three-way graph merge without changing repository state.
func (r *Repository) PreviewMerge(sourceBranch, targetBranch string) (MergePreview, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return MergePreview{}, err
	}
	candidate, err := r.previewMergeLocked(sourceBranch, targetBranch)
	if err != nil {
		return MergePreview{}, err
	}
	return candidate.preview, nil
}

func (r *Repository) previewMergeLocked(sourceBranch, targetBranch string) (mergeCandidate, error) {
	source, ok := r.branches[sourceBranch]
	if !ok {
		return mergeCandidate{}, ErrBranchNotFound
	}
	target, ok := r.branches[targetBranch]
	if !ok {
		return mergeCandidate{}, ErrBranchNotFound
	}
	base, ok := r.mergeBaseLocked(source, target)
	if !ok {
		return mergeCandidate{}, ErrCommitNotFound
	}
	baseSnapshot := r.snapshots[r.commits[base].Snapshot]
	sourceSnapshot := r.snapshots[r.commits[source].Snapshot]
	targetSnapshot := r.snapshots[r.commits[target].Snapshot]
	for _, snapshotID := range []ObjectID{r.commits[base].Snapshot, r.commits[source].Snapshot, r.commits[target].Snapshot} {
		if err := r.ensureSnapshotProjectionLocked(snapshotID); err != nil {
			return mergeCandidate{}, err
		}
	}
	result, err := graphcontract.ThreeWayMerge(
		r.projections[baseSnapshot.NodeRoot],
		r.projections[sourceSnapshot.NodeRoot],
		r.projections[targetSnapshot.NodeRoot],
		r.edgeProjections[r.commits[base].Snapshot],
		r.edgeProjections[r.commits[source].Snapshot],
		r.edgeProjections[r.commits[target].Snapshot],
		baseSnapshot.SchemaRoot,
		sourceSnapshot.SchemaRoot,
		targetSnapshot.SchemaRoot,
	)
	if err != nil {
		return mergeCandidate{}, fmt.Errorf("simulate three-way merge: %w", err)
	}

	conflicts := append([]MergeConflict(nil), result.Conflicts...)
	violations := []SchemaViolation(nil)
	if len(conflicts) == 0 {
		schema, err := r.schemaSnapshotLocked(result.SchemaRoot)
		if err != nil {
			return mergeCandidate{}, err
		}
		if err := ValidateSchemaSnapshot(schema, result.Nodes, result.Edges); err != nil {
			var validation *SchemaValidationError
			if errors.As(err, &validation) {
				violations = validation.Violations
			} else {
				return mergeCandidate{}, err
			}
			for _, violation := range violations {
				conflicts = append(conflicts, MergeConflict{
					Category: "semantic", Entity: violation.Entity, ID: violation.EntityID,
					Field: violation.Field, Paths: graphcontract.SchemaViolationPaths(violation),
				})
			}
		}
	}
	if len(conflicts) > 1 {
		graphcontract.SortMergeConflicts(conflicts)
	}
	for index := range conflicts {
		if conflicts[index].Paths == nil {
			conflicts[index].Paths = graphcontract.MergeConflictPaths(conflicts[index])
		}
		conflictID, err := graphcontract.MergeConflictID(conflicts[index])
		if err != nil {
			return mergeCandidate{}, fmt.Errorf("calculate merge conflict ID: %w", err)
		}
		conflicts[index].ConflictID = conflictID
	}
	preview := MergePreview{
		Binding:      MergePreviewBinding{MergeBase: base, SourceCommit: source, TargetCommit: target},
		SourceBranch: sourceBranch,
		TargetBranch: targetBranch,
		Clean:        len(conflicts) == 0,
		Changes:      result.Changes,
		Conflicts:    conflicts,
		Violations:   violations,
	}
	previewID, err := mergePreviewID(preview)
	if err != nil {
		return mergeCandidate{}, fmt.Errorf("calculate merge preview ID: %w", err)
	}
	preview.ID = previewID
	return mergeCandidate{nodes: result.Nodes, edges: result.Edges, schemaRoot: result.SchemaRoot, preview: preview}, nil
}

func mergePreviewID(preview MergePreview) (ObjectID, error) {
	return persistedObjectID("merge-preview", struct {
		Binding      MergePreviewBinding
		SourceBranch string
		TargetBranch string
		Clean        bool
		Changes      []MergeChange
		Conflicts    []MergeConflict
		Violations   []SchemaViolation
	}{preview.Binding, preview.SourceBranch, preview.TargetBranch, preview.Clean, preview.Changes, preview.Conflicts, preview.Violations})
}
