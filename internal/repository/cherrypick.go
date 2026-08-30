package repository

import (
	"errors"
	"fmt"

	"github.com/autonomous-bits/spool/internal/repository/cherrypick"
)

// CherryPickRequest describes the options for a cherry-pick transplantation operation.
type CherryPickRequest = cherrypick.Request

// CherryPickResult summarizes the transplanted changes, resulting commit, conflicts, and schema violations.
type CherryPickResult = cherrypick.Result

// CherryPickChange describes an entity changed by a cherry-pick.
type CherryPickChange = cherrypick.Change

// CherryPickConflict describes a deterministic three-way merge disagreement during cherry-picking.
type CherryPickConflict = cherrypick.Conflict

// CherryPickSchemaViolation describes a schema rule or constraint failure during cherry-picking.
type CherryPickSchemaViolation = cherrypick.SchemaViolation

// CherryPick sentinel errors re-exported from the cherrypick package.
var (
	ErrCherryPickCommitRequired                = cherrypick.ErrCommitRequired
	ErrCherryPickTargetBranchRequired          = cherrypick.ErrTargetBranchRequired
	ErrCherryPickCommitNotFound                = cherrypick.ErrCommitNotFound
	ErrCherryPickConflicts                     = cherrypick.ErrConflicts
	ErrCherryPickReferentialIntegrityViolation = cherrypick.ErrReferentialIntegrityViolation
)

// CherryPickCommittedWithWarningError reports that a cherry-pick operation was committed,
// but directory synchronization or projection update had a durability warning.
type CherryPickCommittedWithWarningError struct {
	Result CherryPickResult
	err    error
}

// Error returns the underlying durability warning.
func (e *CherryPickCommittedWithWarningError) Error() string { return e.err.Error() }

// Unwrap returns the underlying durability warning.
func (e *CherryPickCommittedWithWarningError) Unwrap() error { return e.err }

// CherryPick computes single-commit graph deltas against its parent, performs 3-way
// property merging against target HEAD, validates referential integrity and schema rules,
// and orchestrates atomic commit generation on targetBranch.
func (r *Repository) CherryPick(request CherryPickRequest) (CherryPickResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return CherryPickResult{}, err
	}
	if request.Commit == "" {
		return CherryPickResult{}, cherrypick.ErrCommitRequired
	}
	if request.TargetBranch == "" {
		return CherryPickResult{}, cherrypick.ErrTargetBranchRequired
	}
	if _, held := r.mergeLeases[request.TargetBranch]; held {
		return CherryPickResult{}, ErrMergeTargetLeaseHeld
	}
	targetHead, exists := r.branches[request.TargetBranch]
	if !exists {
		return CherryPickResult{}, ErrBranchNotFound
	}
	if staged, exists := r.stagedMutations[request.TargetBranch]; exists && (len(staged.Operations) > 0 || staged.TargetSchema != nil) {
		return CherryPickResult{}, ErrUncommittedStagedChanges
	}

	srcCommitID := ObjectID(request.Commit)
	srcCommit, exists := r.commits[srcCommitID]
	if !exists {
		return CherryPickResult{}, ErrCommitNotFound
	}

	var baseSnapshotID ObjectID
	var baseSnapshot graphSnapshot
	var baseNodes map[string]Node
	var baseEdges map[string]Edge
	var baseSchemaRoot ObjectID

	if len(srcCommit.Parents) > 0 {
		baseCommitID := srcCommit.Parents[0]
		baseCommit, ok := r.commits[baseCommitID]
		if !ok {
			return CherryPickResult{}, ErrCommitNotFound
		}
		baseSnapshotID = baseCommit.Snapshot
		if err := r.ensureSnapshotProjectionLocked(baseSnapshotID); err != nil {
			return CherryPickResult{}, err
		}
		baseSnapshot = r.snapshots[baseSnapshotID]
		baseNodes = r.projections[baseSnapshot.NodeRoot]
		baseEdges = r.edgeProjections[baseSnapshotID]
		baseSchemaRoot = baseSnapshot.SchemaRoot
	} else {
		baseNodes = make(map[string]Node)
		baseEdges = make(map[string]Edge)
	}

	srcSnapshotID := srcCommit.Snapshot
	if err := r.ensureSnapshotProjectionLocked(srcSnapshotID); err != nil {
		return CherryPickResult{}, err
	}
	srcSnapshot := r.snapshots[srcSnapshotID]
	srcNodes := r.projections[srcSnapshot.NodeRoot]
	srcEdges := r.edgeProjections[srcSnapshotID]

	targetSnapshotID := r.commits[targetHead].Snapshot
	if err := r.ensureSnapshotProjectionLocked(targetSnapshotID); err != nil {
		return CherryPickResult{}, err
	}
	targetSnapshot := r.snapshots[targetSnapshotID]
	targetNodes := r.projections[targetSnapshot.NodeRoot]
	targetEdges := r.edgeProjections[targetSnapshotID]

	conflicts := make([]MergeConflict, 0)
	mergedNodes := mergeNodeMaps(baseNodes, srcNodes, targetNodes, &conflicts)
	mergedEdges := mergeEdgeMaps(baseEdges, srcEdges, targetEdges, &conflicts)

	var mergedSchemaRoot ObjectID
	if baseSnapshotID != "" {
		mergedSchemaRoot = mergeSchemaRoot(baseSchemaRoot, srcSnapshot.SchemaRoot, targetSnapshot.SchemaRoot, &conflicts)
	} else {
		if srcSnapshot.SchemaRoot == targetSnapshot.SchemaRoot || srcSnapshot.SchemaRoot == "" {
			mergedSchemaRoot = targetSnapshot.SchemaRoot
		} else if targetSnapshot.SchemaRoot == "" {
			mergedSchemaRoot = srcSnapshot.SchemaRoot
		} else {
			mergedSchemaRoot = targetSnapshot.SchemaRoot
			conflicts = append(conflicts, MergeConflict{Category: "schema", Entity: "schema", Field: "root"})
		}
	}

	rawChanges := mergeChanges(targetNodes, mergedNodes, targetEdges, mergedEdges)
	changes := make([]cherrypick.Change, len(rawChanges))
	for i, c := range rawChanges {
		changes[i] = cherrypick.Change{Entity: c.Entity, ID: c.ID, Change: c.Change}
	}

	// Referential integrity pre-flight verification:
	// Verify that all candidate edges have valid source and target nodes present.
	for _, edgeID := range sortedEdgeIDs(mergedEdges) {
		edge := mergedEdges[edgeID]
		sourceExists := hasNode(mergedNodes, edge.Source)
		targetExists := hasNode(mergedNodes, edge.Target)
		if !sourceExists || !targetExists {
			var field string
			if !sourceExists && !targetExists {
				field = "source,target"
			} else if !sourceExists {
				field = "source"
			} else {
				field = "target"
			}
			conflicts = append(conflicts, MergeConflict{
				Category: "structural",
				Entity:   "edge",
				ID:       edgeID,
				Field:    field,
				Paths:    []string{"edge/" + edgeID + "/" + field},
			})
		}
	}

	// Schema conformance validation
	violations := []cherrypick.SchemaViolation(nil)
	if len(conflicts) == 0 {
		schema, err := r.schemaSnapshotLocked(mergedSchemaRoot)
		if err != nil {
			return CherryPickResult{}, err
		}
		if err := ValidateSchemaSnapshot(schema, mergedNodes, mergedEdges); err != nil {
			var validation *SchemaValidationError
			if errors.As(err, &validation) {
				for _, v := range validation.Violations {
					violations = append(violations, cherrypick.SchemaViolation{
						Code:     string(v.Code),
						Entity:   v.Entity,
						EntityID: v.EntityID,
						Rule:     v.Rule,
						Field:    v.Field,
						Expected: v.Expected,
						Actual:   v.Actual,
					})
					conflicts = append(conflicts, MergeConflict{
						Category: "semantic",
						Entity:   v.Entity,
						ID:       v.EntityID,
						Field:    v.Field,
						Paths:    schemaViolationPaths(v),
					})
				}
			} else {
				return CherryPickResult{}, err
			}
		}
	}

	sortMergeConflicts(conflicts)
	cherrypickConflicts := make([]cherrypick.Conflict, len(conflicts))
	for index := range conflicts {
		if conflicts[index].Paths == nil {
			conflicts[index].Paths = mergeConflictPaths(conflicts[index])
		}
		conflicts[index].ConflictID = mergeConflictID(conflicts[index])
		cherrypickConflicts[index] = cherrypick.Conflict{
			ConflictID: conflicts[index].ConflictID,
			Category:   conflicts[index].Category,
			Entity:     conflicts[index].Entity,
			ID:         conflicts[index].ID,
			Field:      conflicts[index].Field,
			Paths:      conflicts[index].Paths,
		}
	}

	// Dry-run preview simulation
	if request.DryRun {
		return CherryPickResult{
			TargetBranch: request.TargetBranch,
			SourceCommit: request.Commit,
			Commit:       string(targetHead),
			DryRun:       true,
			Changes:      changes,
			Conflicts:    cherrypickConflicts,
			Violations:   violations,
		}, nil
	}

	// Non-dry-run conflicts gating: target branch must remain completely unmodified
	if len(conflicts) > 0 {
		return CherryPickResult{
			TargetBranch: request.TargetBranch,
			SourceCommit: request.Commit,
			Commit:       string(targetHead),
			DryRun:       false,
			Changes:      changes,
			Conflicts:    cherrypickConflicts,
			Violations:   violations,
		}, cherrypick.ErrConflicts
	}

	// Idempotent / zero-change no-op
	if len(changes) == 0 {
		return CherryPickResult{
			TargetBranch: request.TargetBranch,
			SourceCommit: request.Commit,
			Commit:       string(targetHead),
			DryRun:       false,
			Changes:      []cherrypick.Change{},
			Conflicts:    []cherrypick.Conflict{},
			Violations:   []cherrypick.SchemaViolation{},
		}, nil
	}

	// Atomic snapshot materialization & commit generation
	objects, snapshots, projections, edgeProjections := r.objects, r.snapshots, r.projections, r.edgeProjections
	materializedSnapshots, historicalProjectionLRU := r.materializedSnapshots, r.historicalProjectionLRU
	commits, branches, stagedMutations := r.commits, r.branches, r.stagedMutations
	r.objects, r.snapshots = cloneObjects(r.objects), cloneSnapshots(r.snapshots)
	r.projections, r.edgeProjections = cloneProjectionMap(r.projections), cloneEdgeProjectionMap(r.edgeProjections)
	r.materializedSnapshots, r.historicalProjectionLRU = cloneMaterializedSnapshots(r.materializedSnapshots), append([]ObjectID(nil), r.historicalProjectionLRU...)
	r.commits, r.branches, r.stagedMutations = cloneCommits(r.commits), cloneBranches(r.branches), cloneStagedMutations(r.stagedMutations)
	r.objectBatch = r.objectStore.beginWriteBatch()
	defer func() { r.objectBatch = nil }()

	rollback := func() {
		r.objects, r.snapshots, r.projections, r.edgeProjections = objects, snapshots, projections, edgeProjections
		r.materializedSnapshots, r.historicalProjectionLRU = materializedSnapshots, historicalProjectionLRU
		r.commits, r.branches, r.stagedMutations = commits, branches, stagedMutations
	}

	newSnapshot, err := r.materializeSnapshotLocked(mergedNodes, mergedEdges, mergedSchemaRoot)
	if err != nil {
		rollback()
		return CherryPickResult{}, fmt.Errorf("materialize cherry-picked snapshot: %w", err)
	}
	newSnapshotID, err := r.storeObject("graph-snapshot", newSnapshot)
	if err != nil {
		rollback()
		return CherryPickResult{}, fmt.Errorf("store cherry-picked snapshot: %w", err)
	}
	r.snapshots[newSnapshotID] = newSnapshot

	author := request.Author
	if author == "" {
		author = srcCommit.Author
		if author == "" {
			author = defaultCommitAuthor
		}
	}
	message := request.Message
	if message == "" {
		if srcCommit.Message != "" {
			message = fmt.Sprintf("%s\n\n(cherry picked from commit %s)", srcCommit.Message, srcCommitID)
		} else {
			message = fmt.Sprintf("Cherry-pick commit %s\n\n(cherry picked from commit %s)", srcCommitID, srcCommitID)
		}
	}
	nextCommit := r.newCommit(newSnapshotID, []ObjectID{targetHead}, author, message)
	nextID, err := r.storeObject("commit", nextCommit)
	if err != nil {
		rollback()
		return CherryPickResult{}, fmt.Errorf("store cherry-picked commit: %w", err)
	}

	packErr := r.objectBatch.publish()
	if packErr != nil && !packPublicationCommitted(packErr) {
		rollback()
		return CherryPickResult{}, fmt.Errorf("publish cherry-picked immutable objects: %w", packErr)
	}
	r.commits[nextID], r.branches[request.TargetBranch] = nextCommit, nextID
	if err := r.ensureBranchHeadProjectionsLocked(); err != nil {
		rollback()
		return CherryPickResult{}, fmt.Errorf("pin cherry-picked snapshot: %w", err)
	}

	result := CherryPickResult{
		TargetBranch: request.TargetBranch,
		SourceCommit: request.Commit,
		Commit:       string(nextID),
		DryRun:       false,
		Changes:      changes,
		Conflicts:    []cherrypick.Conflict{},
		Violations:   []cherrypick.SchemaViolation{},
	}

	refErr := r.writeRefLocked(request.TargetBranch, targetHead, nextID, "cherry-pick")
	if refErr != nil && !durableWriteCommitted(refErr) {
		rollback()
		_ = r.ensureBranchHeadProjectionsLocked()
		return CherryPickResult{}, refErr
	}

	projectionErr := r.maintainActiveProjectionLocked(request.TargetBranch)
	if packErr != nil || refErr != nil {
		return result, &CherryPickCommittedWithWarningError{
			Result: result,
			err:    fmt.Errorf("cherry-pick committed with durability warning: %w", errors.Join(packErr, refErr, projectionErr)),
		}
	}
	if projectionErr != nil {
		return result, fmt.Errorf("cherry-pick completed but projection maintenance failed: %w", projectionErr)
	}
	return result, nil
}
