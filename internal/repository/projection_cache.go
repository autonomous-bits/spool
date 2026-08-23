package repository

import (
	"fmt"

	"github.com/fxamacker/cbor/v2"
)

const historicalProjectionCacheCapacity = 8

// ensureSnapshotProjectionLocked materializes one durable graph snapshot on
// demand. Callers must hold r.mu for writing because materialization updates
// the bounded historical cache.
func (r *Repository) ensureSnapshotProjectionLocked(snapshotID ObjectID) error {
	if r.materializedSnapshots == nil {
		r.materializedSnapshots = make(map[ObjectID]struct{})
	}
	snapshot, ok := r.snapshots[snapshotID]
	if !ok {
		return fmt.Errorf("snapshot %q: %w", snapshotID, ErrCommitNotFound)
	}
	if _, materialized := r.materializedSnapshots[snapshotID]; materialized {
		r.touchHistoricalProjectionLocked(snapshotID)
		return nil
	}
	_, nodes := r.projections[snapshot.NodeRoot]
	_, edges := r.edgeProjections[snapshotID]
	if nodes && edges {
		r.materializedSnapshots[snapshotID] = struct{}{}
	} else if nodes && r.legacyNodeOnlyProjectionLocked(snapshot.NodeRoot) {
		r.materializedSnapshots[snapshotID] = struct{}{}
	} else {
		if err := r.reconstructSnapshotProjectionsLocked(snapshotID, snapshot); err != nil {
			return err
		}
		schema, err := r.schemaSnapshotLocked(snapshot.SchemaRoot)
		if err != nil {
			r.evictSnapshotProjectionLocked(snapshotID)
			return err
		}
		if err := ValidateSchemaSnapshot(schema, r.projections[snapshot.NodeRoot], r.edgeProjections[snapshotID]); err != nil {
			r.evictSnapshotProjectionLocked(snapshotID)
			return fmt.Errorf("validate snapshot schema: %w", err)
		}
	}
	r.touchHistoricalProjectionLocked(snapshotID)
	return nil
}

func (r *Repository) legacyNodeOnlyProjectionLocked(nodeRoot ObjectID) bool {
	data, err := r.objectStore.get(nodeRoot, "prolly-node-root")
	if err != nil {
		return false
	}
	var entries []ObjectID
	return cbor.Unmarshal(data, &entries) == nil
}

func (r *Repository) ensureBranchHeadProjectionsLocked() error {
	for _, head := range r.branches {
		commit, ok := r.commits[head]
		if !ok {
			return fmt.Errorf("branch head %q: %w", head, ErrCommitNotFound)
		}
		if err := r.ensureSnapshotProjectionLocked(commit.Snapshot); err != nil {
			return err
		}
	}
	r.reconcileSnapshotProjectionCacheLocked()
	return nil
}

func (r *Repository) touchHistoricalProjectionLocked(snapshotID ObjectID) {
	if r.snapshotIsBranchHeadLocked(snapshotID) {
		r.removeHistoricalProjectionLocked(snapshotID)
		return
	}
	r.removeHistoricalProjectionLocked(snapshotID)
	r.historicalProjectionLRU = append(r.historicalProjectionLRU, snapshotID)
	for len(r.historicalProjectionLRU) > historicalProjectionCacheCapacity {
		r.evictSnapshotProjectionLocked(r.historicalProjectionLRU[0])
		r.historicalProjectionLRU = r.historicalProjectionLRU[1:]
	}
}

func (r *Repository) reconcileSnapshotProjectionCacheLocked() {
	for snapshotID := range r.materializedSnapshots {
		if _, exists := r.snapshots[snapshotID]; !exists {
			delete(r.materializedSnapshots, snapshotID)
			r.removeHistoricalProjectionLocked(snapshotID)
		}
	}
	for snapshotID := range r.edgeProjections {
		if r.snapshotIsBranchHeadLocked(snapshotID) {
			r.removeHistoricalProjectionLocked(snapshotID)
			continue
		}
		if !r.inHistoricalProjectionCacheLocked(snapshotID) {
			r.evictSnapshotProjectionLocked(snapshotID)
		}
	}
}

func (r *Repository) snapshotIsBranchHeadLocked(snapshotID ObjectID) bool {
	for _, head := range r.branches {
		if commit, ok := r.commits[head]; ok && commit.Snapshot == snapshotID {
			return true
		}
	}
	return false
}

func (r *Repository) inHistoricalProjectionCacheLocked(snapshotID ObjectID) bool {
	for _, cached := range r.historicalProjectionLRU {
		if cached == snapshotID {
			return true
		}
	}
	return false
}

func (r *Repository) removeHistoricalProjectionLocked(snapshotID ObjectID) {
	for index, cached := range r.historicalProjectionLRU {
		if cached == snapshotID {
			r.historicalProjectionLRU = append(r.historicalProjectionLRU[:index], r.historicalProjectionLRU[index+1:]...)
			return
		}
	}
}

func (r *Repository) evictSnapshotProjectionLocked(snapshotID ObjectID) {
	snapshot, ok := r.snapshots[snapshotID]
	if !ok {
		return
	}
	delete(r.edgeProjections, snapshotID)
	delete(r.materializedSnapshots, snapshotID)
	for retainedID := range r.edgeProjections {
		if retained, exists := r.snapshots[retainedID]; exists && retained.NodeRoot == snapshot.NodeRoot {
			return
		}
	}
	delete(r.projections, snapshot.NodeRoot)
}
