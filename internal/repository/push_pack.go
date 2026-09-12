package repository

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/autonomous-bits/spool/graphcontract"
	"github.com/autonomous-bits/spool/internal/repository/asset"
	"github.com/fxamacker/cbor/v2"
	"lukechampine.com/blake3"
)

// Rack push wire-format constants and framing, mirroring the canonical v2
// push contract already shipped in the spool-rack repository
// (internal/server/sync/framing.go and internal/server/review/snapshot.go).
// This is deliberately duplicated here rather than shared through a module
// dependency: goal-cli-native-push is scoped to this repository only, and
// reproducing the wire byte layout is the minimal coupling needed to
// interoperate with Rack's already-accepted push validation pipeline.
const (
	// PushCommitFormatV2 identifies a commit ID derived from the v2 commit
	// frame, matching Rack's sync.CommitFormatV2.
	PushCommitFormatV2 uint32 = 2
	// PushPackFormatV2 identifies a canonical Rack push pack frame, matching
	// Rack's sync.PackFormatV2.
	PushPackFormatV2 uint32 = 2
	// PushPackFormatV3 identifies a canonical Rack push pack frame supporting general
	// DAG commit histories, matching Rack's sync.PackFormatV3.
	PushPackFormatV3 uint32 = 3
	// pushSnapshotEnvelopeVersion identifies Rack's materialized graph
	// snapshot envelope (review.SnapshotEnvelopeVersion): the full node/edge
	// map representation Rack's push validator decodes, distinct from this
	// repository's own Prolly-tree snapshot pointers.
	pushSnapshotEnvelopeVersion uint32 = 3
)

// Push errors.
var (
	// ErrNothingToPush reports that the branch head is already the given
	// base commit, so there is no new history to publish.
	ErrNothingToPush = errors.New("repository: nothing to push")
	// ErrPushBaseNotFound reports that the given base commit does not name
	// any commit in the branch's local DAG history, so the CLI
	// cannot determine which local commits are new.
	ErrPushBaseNotFound = errors.New("repository: push base commit not found in local branch history")
)

var pushCanonicalCBOR, _ = cbor.CanonicalEncOptions().EncMode()

// pushCommitIdentity mirrors Rack's sync.CommitIdentity wire framing.
type pushCommitIdentity struct {
	Format uint32 `cbor:"1,keyasint"`
	ID     string `cbor:"2,keyasint"`
}

// pushCommitFrame mirrors Rack's sync.CommitFrameV2 wire framing.
type pushCommitFrame struct {
	Version      uint32               `cbor:"1,keyasint"`
	Parents      []pushCommitIdentity `cbor:"2,keyasint"`
	SnapshotRoot string               `cbor:"3,keyasint"`
	Author       string               `cbor:"4,keyasint"`
	Message      string               `cbor:"5,keyasint"`
	Time         time.Time            `cbor:"6,keyasint"`
}

// pushPackObject mirrors Rack's sync.PackObjectV2 wire framing.
type pushPackObject struct {
	ID   string `cbor:"1,keyasint"`
	Data []byte `cbor:"2,keyasint"`
}

// pushPackFrame mirrors Rack's sync.PackFrameV2 wire framing.
type pushPackFrame struct {
	Version uint32             `cbor:"1,keyasint"`
	Base    pushCommitIdentity `cbor:"2,keyasint"`
	Target  pushCommitIdentity `cbor:"3,keyasint"`
	Commits []pushCommitFrame  `cbor:"4,keyasint"`
	Objects []pushPackObject   `cbor:"5,keyasint"`
}

// pushSnapshotEnvelope mirrors Rack's private review.snapshotEnvelope: the
// materialized graph (full node/edge maps plus schema) that Rack's push
// validator decodes at each commit's SnapshotRoot. This is a different
// representation than this repository's own graphcontract.Snapshot pointer
// object, which names Prolly-tree roots rather than materialized content.
type pushSnapshotEnvelope struct {
	Version uint32                        `cbor:"1,keyasint"`
	Schema  graphcontract.SchemaSnapshot  `cbor:"2,keyasint"`
	Nodes   map[string]graphcontract.Node `cbor:"3,keyasint"`
	Edges   map[string]graphcontract.Edge `cbor:"4,keyasint"`
}

// PushObject is one immutable, content-addressed object carried by a push
// pack (a materialized graph snapshot referenced by a pushed commit).
type PushObject struct {
	ID   string
	Data []byte
}

// PushCommitRecord is one commit to register with Rack, matching the
// "commits" push metadata field Rack's gateway expects.
type PushCommitRecord struct {
	ID     string               `json:"id"`
	Commit graphcontract.Commit `json:"commit"`
}

// PushPack is a fully built, ready-to-send native push payload.
type PushPack struct {
	// Branch is the branch this pack advances.
	Branch string
	// BaseCommit is Rack's current wire commit ID for Branch, or "" for an
	// initial push publishing the branch's entire history.
	BaseCommit string
	// TargetCommit is the wire commit ID the push advances Branch to.
	TargetCommit string
	// Commits are the new commits to register, oldest-ancestor first.
	Commits []PushCommitRecord
	// Objects are the new content-addressed objects (materialized graph
	// snapshots) the pushed commits reference.
	Objects []PushObject
	// PackFormat identifies the pack frame encoding (always PushPackFormatV2).
	PackFormat uint32
	// PackHash is the content-addressed hash of PackData.
	PackHash string
	// PackData is the canonical CBOR-encoded pack frame to upload verbatim.
	PackData []byte
	// AssetHashes contains the sorted unique list of candidate asset hashes referenced
	// by nodes across the pushed commits.
	AssetHashes []string
}

// pushChainEntry is one recomputed commit along the branch's local
// first-parent history, oldest-ancestor first.
type pushChainEntry struct {
	localID      ObjectID
	wireID       string
	wireParents  []pushCommitIdentity
	snapshotRoot string
	snapshotData []byte
	commit       graphcontract.Commit
}

// BuildPushPack builds a native push pack advancing branch from baseCommit
// (Rack's currently reported wire commit ID for the branch, or "" if Rack
// has no history for it yet) to the branch's current local head.
//
// BuildPushPack always recomputes wire commit IDs and materialized graph
// snapshots from scratch across the branch's entire local first-parent
// history; this repository does not persist any local-to-wire commit ID
// mapping (that is remote-branch-tracking state, a separate capability).
// Recomputation is a pure function of each commit's content, so it is
// always correct, at the cost of walking the full history on every push.
//
// History is traversed as a DAG and topologically sorted (ancestors before
// descendants), supporting multi-parent merge commits.
func (r *Repository) BuildPushPack(ctx context.Context, branch, baseCommit string) (PushPack, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return PushPack{}, err
	}
	head, ok := r.branches[branch]
	if !ok {
		return PushPack{}, ErrBranchNotFound
	}

	chain, err := r.recomputePushChainLocked(ctx, head)
	if err != nil {
		return PushPack{}, err
	}

	var toPush []pushChainEntry
	base := pushCommitIdentity{}
	if baseCommit != "" {
		found := -1
		for i, entry := range chain {
			if entry.wireID == baseCommit {
				found = i
				break
			}
		}
		if found == -1 {
			return PushPack{}, ErrPushBaseNotFound
		}
		base = pushCommitIdentity{Format: PushCommitFormatV2, ID: baseCommit}

		// Filter out baseCommit and all of its ancestors (already on Rack).
		knownCommits := make(map[ObjectID]struct{})
		queue := []ObjectID{chain[found].localID}
		knownCommits[chain[found].localID] = struct{}{}
		for len(queue) > 0 {
			curr := queue[0]
			queue = queue[1:]
			if c, ok := r.commits[curr]; ok {
				for _, p := range c.Parents {
					if _, seen := knownCommits[p]; !seen {
						knownCommits[p] = struct{}{}
						queue = append(queue, p)
					}
				}
			}
		}

		toPush = make([]pushChainEntry, 0, len(chain))
		for _, entry := range chain {
			if _, seen := knownCommits[entry.localID]; !seen {
				toPush = append(toPush, entry)
			}
		}
	} else {
		toPush = chain
	}
	if len(toPush) == 0 {
		return PushPack{}, ErrNothingToPush
	}

	commits := make([]PushCommitRecord, 0, len(toPush))
	frames := make([]pushCommitFrame, 0, len(toPush))
	objects := make([]PushObject, 0, len(toPush))
	seenObjects := make(map[string]struct{}, len(toPush))
	for _, entry := range toPush {
		commits = append(commits, PushCommitRecord{ID: entry.wireID, Commit: entry.commit})
		frames = append(frames, pushCommitFrame{
			Version:      PushCommitFormatV2,
			Parents:      entry.wireParents,
			SnapshotRoot: entry.snapshotRoot,
			Author:       entry.commit.Author,
			Message:      entry.commit.Message,
			Time:         entry.commit.Time,
		})
		if _, ok := seenObjects[entry.snapshotRoot]; !ok {
			seenObjects[entry.snapshotRoot] = struct{}{}
			objects = append(objects, PushObject{ID: entry.snapshotRoot, Data: entry.snapshotData})
		}
	}
	target := toPush[len(toPush)-1].wireID

	seenAssets := make(map[string]struct{})
	var assetHashes []string
	for _, entry := range toPush {
		if localCommit, ok := r.commits[entry.localID]; ok {
			if s, ok := r.snapshots[localCommit.Snapshot]; ok {
				if nodes, ok := r.projections[s.NodeRoot]; ok {
					for _, h := range asset.ExtractAssetHashes(nodes) {
						if _, seen := seenAssets[h]; !seen {
							seenAssets[h] = struct{}{}
							assetHashes = append(assetHashes, h)
						}
					}
				}
			}
		}
	}
	sort.Strings(assetHashes)

	packFormat := PushPackFormatV2
	if !isLinearPush(baseCommit, toPush) {
		packFormat = PushPackFormatV3
	}

	frame := pushPackFrame{
		Version: packFormat,
		Base:    base,
		Target:  pushCommitIdentity{Format: PushCommitFormatV2, ID: target},
		Commits: frames,
		Objects: make([]pushPackObject, 0, len(objects)),
	}
	for _, object := range objects {
		frame.Objects = append(frame.Objects, pushPackObject(object))
	}
	packData, err := pushCanonicalCBOR.Marshal(frame)
	if err != nil {
		return PushPack{}, fmt.Errorf("repository: marshal push pack: %w", err)
	}

	return PushPack{
		Branch:       branch,
		BaseCommit:   baseCommit,
		TargetCommit: target,
		Commits:      commits,
		Objects:      objects,
		PackFormat:   packFormat,
		PackHash:     pushContentID(packData),
		PackData:     packData,
		AssetHashes:  assetHashes,
	}, nil
}

// isLinearPush reports whether toPush forms a strictly linear first-parent chain
// extending from baseCommit (or empty base).
func isLinearPush(baseCommit string, toPush []pushChainEntry) bool {
	if len(toPush) == 0 {
		return true
	}
	previous := baseCommit
	for i, entry := range toPush {
		if len(entry.commit.Parents) > 1 {
			return false
		}
		if len(entry.commit.Parents) == 0 {
			if i != 0 || previous != "" {
				return false
			}
		} else {
			if string(entry.commit.Parents[0]) != previous {
				return false
			}
		}
		previous = entry.wireID
	}
	return true
}

// recomputePushChainLocked walks branch's commit DAG backwards from head
// traversing all parent paths until root commits are reached, topologically
// sorts all commits (ancestors before descendants), and recomputes each
// commit's Rack wire ID and materialized graph snapshot along the way.
// The returned chain is ordered oldest-ancestor first.
func (r *Repository) recomputePushChainLocked(ctx context.Context, head ObjectID) ([]pushChainEntry, error) {
	type visitState uint8
	const (
		visitUnvisited visitState = iota
		visitVisiting
		visitVisited
	)

	var order []ObjectID
	state := make(map[ObjectID]visitState)

	type stackFrame struct {
		id          ObjectID
		parentIndex int
	}

	stack := []stackFrame{{id: head, parentIndex: 0}}
	state[head] = visitVisiting

	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		top := &stack[len(stack)-1]
		localCommit, ok := r.commits[top.id]
		if !ok {
			return nil, ErrCommitNotFound
		}
		if top.parentIndex < len(localCommit.Parents) {
			parentID := localCommit.Parents[top.parentIndex]
			top.parentIndex++
			switch state[parentID] {
			case visitVisiting:
				return nil, fmt.Errorf("repository: cycle detected in commit graph at %s", parentID)
			case visitVisited:
				continue
			default:
				state[parentID] = visitVisiting
				stack = append(stack, stackFrame{id: parentID, parentIndex: 0})
			}
		} else {
			state[top.id] = visitVisited
			order = append(order, top.id)
			stack = stack[:len(stack)-1]
		}
	}

	wireIDMap := make(map[ObjectID]string, len(order))
	chain := make([]pushChainEntry, len(order))

	for i, localID := range order {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		localCommit, ok := r.commits[localID]
		if !ok {
			return nil, ErrCommitNotFound
		}

		snapshotData, err := r.materializePushSnapshotLocked(localCommit.Snapshot)
		if err != nil {
			return nil, err
		}
		snapshotRoot := pushContentID(snapshotData)

		wireParents := make([]graphcontract.ObjectID, len(localCommit.Parents))
		wireIdentities := make([]pushCommitIdentity, len(localCommit.Parents))
		for j, p := range localCommit.Parents {
			wireParentID, ok := wireIDMap[p]
			if !ok {
				return nil, fmt.Errorf("repository: missing wire ID for parent commit %s", p)
			}
			wireParents[j] = graphcontract.ObjectID(wireParentID)
			wireIdentities[j] = pushCommitIdentity{
				Format: PushCommitFormatV2,
				ID:     wireParentID,
			}
		}

		commit := graphcontract.Commit{
			Snapshot: graphcontract.ObjectID(snapshotRoot),
			Parents:  wireParents,
			Message:  localCommit.Message,
			Author:   localCommit.Author,
			Time:     localCommit.Time,
		}

		normalized, err := commit.Normalize()
		if err != nil {
			return nil, fmt.Errorf("repository: normalize push commit %s: %w", localID, err)
		}
		encoded, err := graphcontract.MarshalCommit(normalized)
		if err != nil {
			return nil, fmt.Errorf("repository: encode push commit %s: %w", localID, err)
		}
		wireID := pushContentID(encoded)
		wireIDMap[localID] = wireID

		chain[i] = pushChainEntry{
			localID:      localID,
			wireID:       wireID,
			wireParents:  wireIdentities,
			snapshotRoot: snapshotRoot,
			snapshotData: snapshotData,
			commit:       normalized,
		}
	}

	return chain, nil
}

// materializePushSnapshotLocked builds the canonical CBOR encoding of the
// full materialized graph (schema, every node, every edge) at the given
// local snapshot ID, in the wire envelope shape Rack's push validator
// decodes.
func (r *Repository) materializePushSnapshotLocked(snapshotID ObjectID) ([]byte, error) {
	if err := r.ensureSnapshotProjectionLocked(snapshotID); err != nil {
		return nil, err
	}
	snapshot, ok := r.snapshots[snapshotID]
	if !ok {
		return nil, fmt.Errorf("repository: snapshot %q: %w", snapshotID, ErrCommitNotFound)
	}
	schema, err := r.schemaSnapshotLocked(snapshot.SchemaRoot)
	if err != nil {
		return nil, err
	}
	nodes := r.projections[snapshot.NodeRoot]
	edges := r.edgeProjections[snapshotID]

	nodeCopy := make(map[string]graphcontract.Node, len(nodes))
	for id, node := range nodes {
		nodeCopy[id] = node
	}
	edgeCopy := make(map[string]graphcontract.Edge, len(edges))
	for id, edge := range edges {
		edgeCopy[id] = edge
	}

	envelope := pushSnapshotEnvelope{
		Version: pushSnapshotEnvelopeVersion,
		Schema:  schema,
		Nodes:   nodeCopy,
		Edges:   edgeCopy,
	}
	data, err := pushCanonicalCBOR.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("repository: marshal push snapshot %q: %w", snapshotID, err)
	}
	return data, nil
}

// pushContentID returns the lowercase BLAKE3-256 content ID Rack's push
// protocol uses for pack, object, and commit identity: plain
// blake3(data), with no local object-store header.
func pushContentID(data []byte) string {
	sum := blake3.Sum256(data)
	return hex.EncodeToString(sum[:])
}
