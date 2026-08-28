package repository

import (
	"errors"
	"fmt"

	"github.com/autonomous-bits/spool/internal/repository/prune"
)

// PruneRequest describes the options for a graph pruning operation.
type PruneRequest = prune.Request

// PruneResult summarizes the entities removed, cascading edges excised, and durable orphans detected.
type PruneResult = prune.Result

// Pruning errors re-exported from the prune package.
var (
	ErrProtectedBranch          = prune.ErrProtectedBranch
	ErrUncommittedStagedChanges = prune.ErrUncommittedStagedChanges
)

// PruneCommittedWithWarningError reports that a prune operation was committed, but directory sync had a warning.
type PruneCommittedWithWarningError struct {
	Result PruneResult
	err    error
}

// Error returns the underlying durability warning.
func (e *PruneCommittedWithWarningError) Error() string { return e.err.Error() }

// Unwrap returns the underlying durability warning.
func (e *PruneCommittedWithWarningError) Unwrap() error { return e.err }

// Prune performs two-phase ephemeral entity discovery, cascading edge excision,
// orphan detection, and commit generation.
func (r *Repository) Prune(request PruneRequest) (PruneResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return PruneResult{}, err
	}
	if request.Branch == "" {
		return PruneResult{}, prune.ErrBranchRequired
	}
	if _, held := r.mergeLeases[request.Branch]; held {
		return PruneResult{}, ErrMergeTargetLeaseHeld
	}
	head, exists := r.branches[request.Branch]
	if !exists {
		return PruneResult{}, prune.ErrBranchNotFound
	}
	if staged, exists := r.stagedMutations[request.Branch]; exists && (len(staged.Operations) > 0 || staged.TargetSchema != nil) {
		return PruneResult{}, ErrUncommittedStagedChanges
	}
	if request.Branch == r.defaultBranch && !request.Force {
		return PruneResult{}, ErrProtectedBranch
	}

	snapshotID := r.commits[head].Snapshot
	if err := r.ensureSnapshotProjectionLocked(snapshotID); err != nil {
		return PruneResult{}, err
	}
	snapshot := r.snapshots[snapshotID]
	existingNodes := r.projections[snapshot.NodeRoot]
	existingEdges := r.edgeProjections[snapshotID]

	// Phase 1: Ephemeral entity discovery
	prunedNodeIDs := make([]string, 0)
	isEphemeral := make(map[string]bool)
	for _, id := range sortedNodeIDs(existingNodes) {
		node := existingNodes[id]
		if hasLabel(node, UniversalModifierLabel) {
			prunedNodeIDs = append(prunedNodeIDs, id)
			isEphemeral[id] = true
		}
	}

	// Idempotent zero-match no-op
	if len(prunedNodeIDs) == 0 {
		return PruneResult{
			Branch:               request.Branch,
			Commit:               string(head),
			DryRun:               request.DryRun,
			PrunedNodesCount:     0,
			PrunedEdgesCount:     0,
			PrunedNodeIDs:        []string{},
			OrphanedDurableNodes: []string{},
		}, nil
	}

	// Phase 2: Cascading edge excision
	prunedEdgeIDs := make([]string, 0)
	for _, id := range sortedEdgeIDs(existingEdges) {
		edge := existingEdges[id]
		if isEphemeral[edge.Source] || isEphemeral[edge.Target] {
			prunedEdgeIDs = append(prunedEdgeIDs, id)
		}
	}

	// Phase 3: Orphaned durable entity detection
	durableInitialDegree := make(map[string]int)
	durableRemainingDegree := make(map[string]int)
	for _, edge := range existingEdges {
		if !isEphemeral[edge.Source] {
			durableInitialDegree[edge.Source]++
		}
		if !isEphemeral[edge.Target] {
			durableInitialDegree[edge.Target]++
		}
		if !isEphemeral[edge.Source] && !isEphemeral[edge.Target] {
			durableRemainingDegree[edge.Source]++
			durableRemainingDegree[edge.Target]++
		}
	}
	orphanedDurableNodes := make([]string, 0)
	for _, id := range sortedNodeIDs(existingNodes) {
		if isEphemeral[id] {
			continue
		}
		if durableInitialDegree[id] > 0 && durableRemainingDegree[id] == 0 {
			orphanedDurableNodes = append(orphanedDurableNodes, id)
		}
	}

	// Phase 4: Dry-run preview
	if request.DryRun {
		return PruneResult{
			Branch:               request.Branch,
			Commit:               string(head),
			DryRun:               true,
			PrunedNodesCount:     len(prunedNodeIDs),
			PrunedEdgesCount:     len(prunedEdgeIDs),
			PrunedNodeIDs:        prunedNodeIDs,
			OrphanedDurableNodes: orphanedDurableNodes,
		}, nil
	}

	// Phase 5: Materialization & commit generation
	operations := make([]MutationOperation, 0, len(prunedNodeIDs)+len(prunedEdgeIDs))
	for _, id := range prunedNodeIDs {
		operations = append(operations, MutationOperation{
			Action: "delete",
			Entity: "node",
			ID:     id,
		})
	}
	for _, id := range prunedEdgeIDs {
		operations = append(operations, MutationOperation{
			Action: "delete",
			Entity: "edge",
			ID:     id,
		})
	}

	nodes, edges := cloneNodes(existingNodes), cloneEdges(existingEdges)
	applyMutationOperations(nodes, edges, operations)
	if err := validateCandidateValues(nodes, edges); err != nil {
		return PruneResult{}, err
	}
	schema, err := r.schemaSnapshotLocked(snapshot.SchemaRoot)
	if err != nil {
		return PruneResult{}, err
	}
	if err := ValidateSchemaSnapshot(schema, nodes, edges); err != nil {
		return PruneResult{}, err
	}

	objects, snapshots, projections, edgeProjections := r.objects, r.snapshots, r.projections, r.edgeProjections
	materializedSnapshots, historicalProjectionLRU := r.materializedSnapshots, r.historicalProjectionLRU
	commits, branches, stagedMutations := r.commits, r.branches, r.stagedMutations
	r.objects, r.snapshots = cloneObjects(r.objects), cloneSnapshots(r.snapshots)
	r.projections, r.edgeProjections = cloneProjectionMap(r.projections), cloneEdgeProjectionMap(r.edgeProjections)
	r.materializedSnapshots, r.historicalProjectionLRU = cloneMaterializedSnapshots(r.materializedSnapshots), append([]ObjectID(nil), r.historicalProjectionLRU...)
	r.commits, r.branches, r.stagedMutations = cloneCommits(r.commits), cloneBranches(r.branches), cloneStagedMutations(r.stagedMutations)
	r.objectBatch = r.objectStore.beginWriteBatch()
	defer func() { r.objectBatch = nil }()

	newSnapshot, err := r.materializeSnapshotLocked(nodes, edges, snapshot.SchemaRoot)
	if err != nil {
		r.objects, r.snapshots, r.projections, r.edgeProjections = objects, snapshots, projections, edgeProjections
		r.materializedSnapshots, r.historicalProjectionLRU = materializedSnapshots, historicalProjectionLRU
		r.commits, r.branches, r.stagedMutations = commits, branches, stagedMutations
		return PruneResult{}, fmt.Errorf("materialize pruned snapshot: %w", err)
	}
	newSnapshotID, err := r.storeObject("graph-snapshot", newSnapshot)
	if err != nil {
		r.objects, r.snapshots, r.projections, r.edgeProjections = objects, snapshots, projections, edgeProjections
		r.materializedSnapshots, r.historicalProjectionLRU = materializedSnapshots, historicalProjectionLRU
		r.commits, r.branches, r.stagedMutations = commits, branches, stagedMutations
		return PruneResult{}, fmt.Errorf("store pruned snapshot: %w", err)
	}
	r.snapshots[newSnapshotID] = newSnapshot

	author := request.Author
	if author == "" {
		author = defaultCommitAuthor
	}
	message := request.Message
	if message == "" {
		message = "Prune ephemeral entities"
	}
	next := r.newCommit(newSnapshotID, []ObjectID{head}, author, message)
	nextID, err := r.storeObject("commit", next)
	if err != nil {
		r.objects, r.snapshots, r.projections, r.edgeProjections = objects, snapshots, projections, edgeProjections
		r.materializedSnapshots, r.historicalProjectionLRU = materializedSnapshots, historicalProjectionLRU
		r.commits, r.branches, r.stagedMutations = commits, branches, stagedMutations
		return PruneResult{}, fmt.Errorf("store pruned commit: %w", err)
	}

	packErr := r.objectBatch.publish()
	if packErr != nil && !packPublicationCommitted(packErr) {
		r.objects, r.snapshots, r.projections, r.edgeProjections = objects, snapshots, projections, edgeProjections
		r.materializedSnapshots, r.historicalProjectionLRU = materializedSnapshots, historicalProjectionLRU
		r.commits, r.branches, r.stagedMutations = commits, branches, stagedMutations
		return PruneResult{}, fmt.Errorf("publish pruned immutable objects: %w", packErr)
	}
	r.commits[nextID], r.branches[request.Branch] = next, nextID
	if err := r.ensureBranchHeadProjectionsLocked(); err != nil {
		r.objects, r.snapshots, r.projections, r.edgeProjections = objects, snapshots, projections, edgeProjections
		r.materializedSnapshots, r.historicalProjectionLRU = materializedSnapshots, historicalProjectionLRU
		r.commits, r.branches, r.stagedMutations = commits, branches, stagedMutations
		return PruneResult{}, fmt.Errorf("pin committed snapshot: %w", err)
	}

	result := PruneResult{
		Branch:               request.Branch,
		Commit:               string(nextID),
		DryRun:               false,
		PrunedNodesCount:     len(prunedNodeIDs),
		PrunedEdgesCount:     len(prunedEdgeIDs),
		PrunedNodeIDs:        prunedNodeIDs,
		OrphanedDurableNodes: orphanedDurableNodes,
	}

	refErr := r.writeRefLocked(request.Branch, head, nextID, "prune")
	if refErr != nil && !durableWriteCommitted(refErr) {
		r.objects, r.snapshots, r.projections, r.edgeProjections = objects, snapshots, projections, edgeProjections
		r.materializedSnapshots, r.historicalProjectionLRU = materializedSnapshots, historicalProjectionLRU
		r.commits, r.branches, r.stagedMutations = commits, branches, stagedMutations
		_ = r.ensureBranchHeadProjectionsLocked()
		return PruneResult{}, refErr
	}

	projectionErr := r.maintainActiveProjectionLocked(request.Branch)
	if packErr != nil || refErr != nil {
		return result, &PruneCommittedWithWarningError{Result: result, err: fmt.Errorf("prune committed with durability warning: %w", errors.Join(packErr, refErr, projectionErr))}
	}
	if projectionErr != nil {
		return result, fmt.Errorf("prune completed but projection maintenance failed: %w", projectionErr)
	}
	return result, nil
}
