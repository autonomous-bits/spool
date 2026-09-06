package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/autonomous-bits/spool/graphcontract"
	"github.com/fxamacker/cbor/v2"
)

// Pull errors.
var (
	// ErrPullBaseMismatch reports that the pack's declared base commit does
	// not match the branch's locally recomputed wire head, so the pack
	// cannot be installed as a fast-forward extension of local history.
	ErrPullBaseMismatch = errors.New("repository: pull pack base does not match local branch head")
	// ErrPullBootstrapUnsupported reports that the pack declares an empty
	// base (a from-scratch history with no common ancestor). Installing a
	// branch with no local ancestry in common with Rack is not yet
	// supported; that is a separate, not-yet-implemented capability.
	ErrPullBootstrapUnsupported = errors.New("repository: pull does not yet support bootstrapping a branch with no local history")
	// ErrPullInvalidPack reports that a pull pack failed to decode or
	// failed an internal consistency check (canonical round-trip, object
	// content hash, or final head mismatch).
	ErrPullInvalidPack = errors.New("repository: invalid pull pack")
	// ErrPullChainDiscontinuous reports that a pack's declared base does not
	// chain from the previous pack's target within a single pull.
	ErrPullChainDiscontinuous = errors.New("repository: pull packs are not contiguous")
)

var pullStrictCBOR, _ = cbor.DecOptions{
	DupMapKey:         cbor.DupMapKeyEnforcedAPF,
	IndefLength:       cbor.IndefLengthForbidden,
	TagsMd:            cbor.TagsForbidden,
	ExtraReturnErrors: cbor.ExtraDecErrorUnknownField,
}.DecMode()

// InstallPullResult reports the outcome of installing a pull pack.
type InstallPullResult struct {
	// Branch is the branch the pull advanced.
	Branch string
	// CommitsInstalled is the number of new local commits created.
	CommitsInstalled int
	// HeadCommit is the branch's new local head object ID.
	HeadCommit ObjectID
}

// InstallPullPack decodes and installs the given pull packs (as returned by
// remote.Pull, oldest-to-newest) onto branch's local history, advancing the
// branch ref only after every commit and object across all packs has been
// decoded, validated, and materialized successfully.
//
// Each pack must be a canonical CBOR pushPackFrame (the same wire shape
// BuildPushPack produces), byte-compatible with Rack's sync.PackFrameV2.
// InstallPullPack requires the first pack's declared base commit to equal
// branch's current local head's recomputed wire commit ID: pulling a branch
// with no common ancestor with Rack (a from-scratch bootstrap/clone) is not
// yet supported (ErrPullBootstrapUnsupported).
//
// InstallPullPack preserves each pulled commit's original Author, Message,
// and Time exactly as Rack reports them, so a subsequent push recomputes the
// same wire commit IDs Rack already has.
func (r *Repository) InstallPullPack(ctx context.Context, branch string, packs [][]byte, expectedHead string) (InstallPullResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return InstallPullResult{}, err
	}
	head, ok := r.branches[branch]
	if !ok {
		return InstallPullResult{}, ErrBranchNotFound
	}

	chain, err := r.recomputePushChainLocked(ctx, head)
	if err != nil {
		return InstallPullResult{}, err
	}
	localHeadWireID := ""
	if len(chain) > 0 {
		localHeadWireID = chain[len(chain)-1].wireID
	}

	frames := make([]pushPackFrame, 0, len(packs))
	for i, raw := range packs {
		var frame pushPackFrame
		if err := pullStrictCBOR.Unmarshal(raw, &frame); err != nil {
			return InstallPullResult{}, fmt.Errorf("%w: decode pack %d: %v", ErrPullInvalidPack, i, err)
		}
		reencoded, err := pushCanonicalCBOR.Marshal(frame)
		if err != nil || pushContentID(reencoded) != pushContentID(raw) {
			return InstallPullResult{}, fmt.Errorf("%w: pack %d is not canonically encoded", ErrPullInvalidPack, i)
		}
		frames = append(frames, frame)
	}

	if len(frames) == 0 {
		return InstallPullResult{Branch: branch, HeadCommit: head}, nil
	}
	if frames[0].Base.ID == "" {
		return InstallPullResult{}, ErrPullBootstrapUnsupported
	}
	if frames[0].Base.ID != localHeadWireID {
		return InstallPullResult{}, fmt.Errorf("%w: local head is %q, pack base is %q", ErrPullBaseMismatch, localHeadWireID, frames[0].Base.ID)
	}
	for i := 1; i < len(frames); i++ {
		if frames[i].Base.ID != frames[i-1].Target.ID {
			return InstallPullResult{}, fmt.Errorf("%w: pack %d base %q does not match pack %d target %q", ErrPullChainDiscontinuous, i, frames[i].Base.ID, i-1, frames[i-1].Target.ID)
		}
	}

	// Collect every object referenced across all packs, keyed by its
	// content-addressed wire ID, and verify each one's declared hash.
	objectsByID := make(map[string][]byte)
	for i, frame := range frames {
		for _, object := range frame.Objects {
			if got := pushContentID(object.Data); got != object.ID {
				return InstallPullResult{}, fmt.Errorf("%w: pack %d object %q has hash %q", ErrPullInvalidPack, i, object.ID, got)
			}
			objectsByID[object.ID] = object.Data
		}
	}

	objects, snapshots, projections, edgeProjections := r.objects, r.snapshots, r.projections, r.edgeProjections
	materializedSnapshots, historicalProjectionLRU := r.materializedSnapshots, r.historicalProjectionLRU
	commits, branches := r.commits, r.branches
	restore := func() {
		r.objects, r.snapshots, r.projections, r.edgeProjections = objects, snapshots, projections, edgeProjections
		r.materializedSnapshots, r.historicalProjectionLRU = materializedSnapshots, historicalProjectionLRU
		r.commits, r.branches = commits, branches
	}
	r.objects, r.snapshots = cloneObjects(r.objects), cloneSnapshots(r.snapshots)
	r.projections, r.edgeProjections = cloneProjectionMap(r.projections), cloneEdgeProjectionMap(r.edgeProjections)
	r.materializedSnapshots, r.historicalProjectionLRU = cloneMaterializedSnapshots(r.materializedSnapshots), append([]ObjectID(nil), r.historicalProjectionLRU...)
	r.commits, r.branches = cloneCommits(r.commits), cloneBranches(r.branches)
	r.objectBatch = r.objectStore.beginWriteBatch()
	defer func() { r.objectBatch = nil }()

	wireToLocal := map[string]ObjectID{localHeadWireID: head}
	installed := 0
	currentLocalHead := head
	var lastWireID string
	for i, frame := range frames {
		if err := ctx.Err(); err != nil {
			restore()
			return InstallPullResult{}, err
		}
		for j, cframe := range frame.Commits {
			parents := make([]ObjectID, 0, len(cframe.Parents))
			for _, parent := range cframe.Parents {
				localParent, ok := wireToLocal[parent.ID]
				if !ok {
					restore()
					return InstallPullResult{}, fmt.Errorf("%w: pack %d commit %d references unknown parent %q", ErrPullInvalidPack, i, j, parent.ID)
				}
				parents = append(parents, localParent)
			}

			objectData, ok := objectsByID[cframe.SnapshotRoot]
			if !ok {
				restore()
				return InstallPullResult{}, fmt.Errorf("%w: pack %d commit %d references unknown object %q", ErrPullInvalidPack, i, j, cframe.SnapshotRoot)
			}
			var envelope pushSnapshotEnvelope
			if err := pullStrictCBOR.Unmarshal(objectData, &envelope); err != nil {
				restore()
				return InstallPullResult{}, fmt.Errorf("%w: pack %d commit %d snapshot envelope: %v", ErrPullInvalidPack, i, j, err)
			}

			nodes := make(map[string]Node, len(envelope.Nodes))
			for id, node := range envelope.Nodes {
				nodes[id] = node
			}
			edges := make(map[string]Edge, len(envelope.Edges))
			for id, edge := range envelope.Edges {
				edges[id] = edge
			}
			schemaRoot, err := r.storeObject("schema-root", envelope.Schema)
			if err != nil {
				restore()
				return InstallPullResult{}, fmt.Errorf("repository: store pulled schema: %w", err)
			}
			snapshot, err := r.materializeSnapshotLocked(nodes, edges, schemaRoot)
			if err != nil {
				restore()
				return InstallPullResult{}, fmt.Errorf("repository: materialize pulled snapshot: %w", err)
			}
			snapshotID, err := r.storeObject("graph-snapshot", snapshot)
			if err != nil {
				restore()
				return InstallPullResult{}, fmt.Errorf("repository: store pulled snapshot: %w", err)
			}
			r.snapshots[snapshotID] = snapshot

			next := commit{
				Snapshot: snapshotID,
				Parents:  parents,
				Author:   cframe.Author,
				Message:  cframe.Message,
				Time:     cframe.Time,
			}
			nextID, err := r.storeObject("commit", next)
			if err != nil {
				restore()
				return InstallPullResult{}, fmt.Errorf("repository: store pulled commit: %w", err)
			}
			r.commits[nextID] = next

			normalized, err := graphcontract.Commit{
				Snapshot: graphcontract.ObjectID(cframe.SnapshotRoot),
				Parents:  identitiesToObjectIDs(cframe.Parents),
				Author:   cframe.Author,
				Message:  cframe.Message,
				Time:     cframe.Time,
			}.Normalize()
			if err != nil {
				restore()
				return InstallPullResult{}, fmt.Errorf("%w: normalize pack %d commit %d: %v", ErrPullInvalidPack, i, j, err)
			}
			encoded, err := graphcontract.MarshalCommit(normalized)
			if err != nil {
				restore()
				return InstallPullResult{}, fmt.Errorf("%w: encode pack %d commit %d: %v", ErrPullInvalidPack, i, j, err)
			}
			wireID := pushContentID(encoded)
			wireToLocal[wireID] = nextID
			currentLocalHead = nextID
			lastWireID = wireID
			installed++
		}
		if frame.Target.ID != "" && frame.Target.ID != lastWireID {
			restore()
			return InstallPullResult{}, fmt.Errorf("%w: pack %d declared target %q does not match computed head %q", ErrPullInvalidPack, i, frame.Target.ID, lastWireID)
		}
	}

	if expectedHead != "" && lastWireID != expectedHead {
		restore()
		return InstallPullResult{}, fmt.Errorf("%w: installed head %q does not match expected head %q", ErrPullInvalidPack, lastWireID, expectedHead)
	}

	packErr := r.objectBatch.publish()
	if packErr != nil && !packPublicationCommitted(packErr) {
		restore()
		return InstallPullResult{}, fmt.Errorf("repository: publish pulled immutable objects: %w", packErr)
	}

	r.branches[branch] = currentLocalHead
	if err := r.ensureBranchHeadProjectionsLocked(); err != nil {
		restore()
		return InstallPullResult{}, fmt.Errorf("repository: pin pulled snapshot: %w", err)
	}

	refErr := r.writeRefLocked(branch, head, currentLocalHead, "pull")
	if refErr != nil && !durableWriteCommitted(refErr) {
		restore()
		_ = r.ensureBranchHeadProjectionsLocked()
		return InstallPullResult{}, fmt.Errorf("repository: advance branch ref: %w", refErr)
	}
	projectionErr := r.maintainActiveProjectionLocked(branch)

	result := InstallPullResult{Branch: branch, CommitsInstalled: installed, HeadCommit: currentLocalHead}
	if packErr != nil || refErr != nil {
		return result, &CommittedWithWarningError{Result: CommitStagedMutationResult{Branch: branch, Commit: currentLocalHead}, err: fmt.Errorf("pull committed with durability warning: %w", errors.Join(packErr, refErr))}
	}
	if projectionErr != nil {
		return result, fmt.Errorf("pull completed but projection maintenance failed: %w", projectionErr)
	}
	return result, nil
}

func identitiesToObjectIDs(identities []pushCommitIdentity) []graphcontract.ObjectID {
	if len(identities) == 0 {
		return nil
	}
	ids := make([]graphcontract.ObjectID, len(identities))
	for i, identity := range identities {
		ids[i] = graphcontract.ObjectID(identity.ID)
	}
	return ids
}
