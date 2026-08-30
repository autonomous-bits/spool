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
	ErrCherryPickCommitRequired       = cherrypick.ErrCommitRequired
	ErrCherryPickTargetBranchRequired = cherrypick.ErrTargetBranchRequired
	ErrCherryPickConflicts            = cherrypick.ErrConflicts
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

type cherryPickCandidate struct {
	targetHead       ObjectID
	srcCommitID      ObjectID
	srcCommit        commit
	mergedNodes      map[string]Node
	mergedEdges      map[string]Edge
	mergedSchemaRoot ObjectID
	changes          []cherrypick.Change
	conflicts        []MergeConflict
	violations       []cherrypick.SchemaViolation
}

// CherryPick computes single-commit graph deltas against its parent, performs 3-way
// property merging against target HEAD, validates referential integrity and schema rules,
// and orchestrates atomic commit generation on targetBranch.
func (r *Repository) CherryPick(request CherryPickRequest) (CherryPickResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return CherryPickResult{}, err
	}

	candidate, err := r.previewCherryPickLocked(request)
	if err != nil {
		return CherryPickResult{}, err
	}

	result := formatCherryPickResult(request, candidate)

	if request.DryRun {
		return result, nil
	}
	if len(candidate.conflicts) > 0 {
		return result, cherrypick.ErrConflicts
	}
	if len(candidate.changes) == 0 {
		return result, nil
	}

	return r.applyCherryPickCandidateLocked(request, candidate)
}

func (r *Repository) previewCherryPickLocked(request CherryPickRequest) (cherryPickCandidate, error) {
	if request.Commit == "" {
		return cherryPickCandidate{}, cherrypick.ErrCommitRequired
	}
	if request.TargetBranch == "" {
		return cherryPickCandidate{}, cherrypick.ErrTargetBranchRequired
	}
	if _, held := r.mergeLeases[request.TargetBranch]; held {
		return cherryPickCandidate{}, ErrMergeTargetLeaseHeld
	}
	targetHead, exists := r.branches[request.TargetBranch]
	if !exists {
		return cherryPickCandidate{}, ErrBranchNotFound
	}
	if staged, exists := r.stagedMutations[request.TargetBranch]; exists && (len(staged.Operations) > 0 || staged.TargetSchema != nil) {
		return cherryPickCandidate{}, ErrUncommittedStagedChanges
	}

	srcCommitID := ObjectID(request.Commit)
	srcCommit, exists := r.commits[srcCommitID]
	if !exists {
		return cherryPickCandidate{}, ErrCommitNotFound
	}

	baseNodes, baseEdges, baseSchemaRoot, err := r.resolveCherryPickBaseLocked(srcCommit)
	if err != nil {
		return cherryPickCandidate{}, err
	}

	srcNodes, srcEdges, srcSchemaRoot, err := r.resolveSnapshotEntitiesLocked(srcCommit.Snapshot)
	if err != nil {
		return cherryPickCandidate{}, err
	}

	targetNodes, targetEdges, targetSchemaRoot, err := r.resolveSnapshotEntitiesLocked(r.commits[targetHead].Snapshot)
	if err != nil {
		return cherryPickCandidate{}, err
	}

	conflicts := make([]MergeConflict, 0)
	mergedNodes := mergeNodeMaps(baseNodes, srcNodes, targetNodes, &conflicts)
	mergedEdges := mergeEdgeMaps(baseEdges, srcEdges, targetEdges, &conflicts)
	mergedSchemaRoot := mergeCherryPickSchemaRoot(baseSchemaRoot, srcSchemaRoot, targetSchemaRoot, &conflicts)

	validateCherryPickEndpoints(mergedNodes, mergedEdges, &conflicts)
	violations, err := r.validateCherryPickSchemaLocked(mergedSchemaRoot, mergedNodes, mergedEdges, &conflicts)
	if err != nil {
		return cherryPickCandidate{}, err
	}

	sortMergeConflicts(conflicts)
	for i := range conflicts {
		if conflicts[i].Paths == nil {
			conflicts[i].Paths = mergeConflictPaths(conflicts[i])
		}
		conflicts[i].ConflictID = mergeConflictID(conflicts[i])
	}

	return cherryPickCandidate{
		targetHead:       targetHead,
		srcCommitID:      srcCommitID,
		srcCommit:        srcCommit,
		mergedNodes:      mergedNodes,
		mergedEdges:      mergedEdges,
		mergedSchemaRoot: mergedSchemaRoot,
		changes:          formatCherryPickChanges(targetNodes, mergedNodes, targetEdges, mergedEdges),
		conflicts:        conflicts,
		violations:       violations,
	}, nil
}

func (r *Repository) resolveCherryPickBaseLocked(srcCommit commit) (map[string]Node, map[string]Edge, ObjectID, error) {
	if len(srcCommit.Parents) == 0 {
		return make(map[string]Node), make(map[string]Edge), "", nil
	}
	baseCommitID := srcCommit.Parents[0]
	baseCommit, ok := r.commits[baseCommitID]
	if !ok {
		return nil, nil, "", ErrCommitNotFound
	}
	return r.resolveSnapshotEntitiesLocked(baseCommit.Snapshot)
}

func (r *Repository) resolveSnapshotEntitiesLocked(snapshotID ObjectID) (map[string]Node, map[string]Edge, ObjectID, error) {
	if err := r.ensureSnapshotProjectionLocked(snapshotID); err != nil {
		return nil, nil, "", err
	}
	snapshot := r.snapshots[snapshotID]
	return r.projections[snapshot.NodeRoot], r.edgeProjections[snapshotID], snapshot.SchemaRoot, nil
}

func mergeCherryPickSchemaRoot(baseRoot, srcRoot, targetRoot ObjectID, conflicts *[]MergeConflict) ObjectID {
	if baseRoot != "" {
		return mergeSchemaRoot(baseRoot, srcRoot, targetRoot, conflicts)
	}
	if srcRoot == targetRoot || srcRoot == "" {
		return targetRoot
	}
	if targetRoot == "" {
		return srcRoot
	}
	*conflicts = append(*conflicts, MergeConflict{Category: "schema", Entity: "schema", Field: "root"})
	return targetRoot
}

func validateCherryPickEndpoints(nodes map[string]Node, edges map[string]Edge, conflicts *[]MergeConflict) {
	for _, edgeID := range sortedEdgeIDs(edges) {
		edge := edges[edgeID]
		sourceExists := hasNode(nodes, edge.Source)
		targetExists := hasNode(nodes, edge.Target)
		if !sourceExists || !targetExists {
			var field string
			if !sourceExists && !targetExists {
				field = "source,target"
			} else if !sourceExists {
				field = "source"
			} else {
				field = "target"
			}
			*conflicts = append(*conflicts, MergeConflict{
				Category: "structural",
				Entity:   "edge",
				ID:       edgeID,
				Field:    field,
				Paths:    []string{"edge/" + edgeID + "/" + field},
			})
		}
	}
}

func (r *Repository) validateCherryPickSchemaLocked(schemaRoot ObjectID, nodes map[string]Node, edges map[string]Edge, conflicts *[]MergeConflict) ([]cherrypick.SchemaViolation, error) {
	if len(*conflicts) > 0 {
		return nil, nil
	}
	schema, err := r.schemaSnapshotLocked(schemaRoot)
	if err != nil {
		return nil, err
	}
	violations := []cherrypick.SchemaViolation(nil)
	if err := ValidateSchemaSnapshot(schema, nodes, edges); err != nil {
		var validation *SchemaValidationError
		if !errors.As(err, &validation) {
			return nil, err
		}
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
			*conflicts = append(*conflicts, MergeConflict{
				Category: "semantic",
				Entity:   v.Entity,
				ID:       v.EntityID,
				Field:    v.Field,
				Paths:    schemaViolationPaths(v),
			})
		}
	}
	return violations, nil
}

func formatCherryPickChanges(targetNodes, mergedNodes map[string]Node, targetEdges, mergedEdges map[string]Edge) []cherrypick.Change {
	rawChanges := mergeChanges(targetNodes, mergedNodes, targetEdges, mergedEdges)
	changes := make([]cherrypick.Change, len(rawChanges))
	for i, c := range rawChanges {
		changes[i] = cherrypick.Change{Entity: c.Entity, ID: c.ID, Change: c.Change}
	}
	return changes
}

func formatCherryPickResult(request CherryPickRequest, candidate cherryPickCandidate) CherryPickResult {
	conflictList := make([]cherrypick.Conflict, len(candidate.conflicts))
	for i, c := range candidate.conflicts {
		conflictList[i] = cherrypick.Conflict{
			ConflictID: c.ConflictID,
			Category:   c.Category,
			Entity:     c.Entity,
			ID:         c.ID,
			Field:      c.Field,
			Paths:      c.Paths,
		}
	}
	return CherryPickResult{
		TargetBranch: request.TargetBranch,
		SourceCommit: request.Commit,
		Commit:       string(candidate.targetHead),
		DryRun:       request.DryRun,
		Changes:      candidate.changes,
		Conflicts:    conflictList,
		Violations:   candidate.violations,
	}
}

func (r *Repository) applyCherryPickCandidateLocked(request CherryPickRequest, candidate cherryPickCandidate) (CherryPickResult, error) {
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

	newSnapshot, err := r.materializeSnapshotLocked(candidate.mergedNodes, candidate.mergedEdges, candidate.mergedSchemaRoot)
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
		author = candidate.srcCommit.Author
		if author == "" {
			author = defaultCommitAuthor
		}
	}
	message := request.Message
	if message == "" {
		if candidate.srcCommit.Message != "" {
			message = fmt.Sprintf("%s\n\n(cherry picked from commit %s)", candidate.srcCommit.Message, candidate.srcCommitID)
		} else {
			message = fmt.Sprintf("Cherry-pick commit %s\n\n(cherry picked from commit %s)", candidate.srcCommitID, candidate.srcCommitID)
		}
	}
	nextCommit := r.newCommit(newSnapshotID, []ObjectID{candidate.targetHead}, author, message)
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
		Changes:      candidate.changes,
		Conflicts:    []cherrypick.Conflict{},
		Violations:   []cherrypick.SchemaViolation{},
	}

	refErr := r.writeRefLocked(request.TargetBranch, candidate.targetHead, nextID, "cherry-pick")
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
