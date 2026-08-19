package repository

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"

	"github.com/fxamacker/cbor/v2"
	"github.com/gofrs/flock"
	"lukechampine.com/blake3"
)

type persistedMergeTransaction struct {
	Version      int              `json:"version"`
	TargetBranch string           `json:"targetBranch"`
	LeaseOwner   string           `json:"leaseOwner"`
	Transaction  mergeTransaction `json:"transaction"`
}

type persistedRepository struct {
	DefaultBranch   string                       `json:"defaultBranch"`
	ActiveBranch    string                       `json:"activeBranch"`
	Branches        map[string]ObjectID          `json:"branches"`
	Commits         map[ObjectID]commit          `json:"commits"`
	Snapshots       map[ObjectID]graphSnapshot   `json:"snapshots"`
	Projections     map[ObjectID]map[string]Node `json:"projections"`
	EdgeProjections map[ObjectID]map[string]Edge `json:"edgeProjections,omitempty"`
	Objects         map[ObjectID][]byte          `json:"objects"`
	StagedMutations map[string]StagedMutationSet `json:"stagedMutations,omitempty"`
}

type legacyNode struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type legacyEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
}

type durableWriteCommittedError struct{ err error }

// Error returns the underlying durability warning.
func (e durableWriteCommittedError) Error() string { return e.err.Error() }

// Unwrap returns the underlying durability warning.
func (e durableWriteCommittedError) Unwrap() error { return e.err }

// NewSeedRepositoryWithMergeState opens or creates stateDir, acquires its process lock, and recovers state.
func NewSeedRepositoryWithMergeState(stateDir string) (*Repository, error) {
	repo := NewSeedRepository()
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("create merge state directory: %w", err)
	}
	repo.mergeStateDir = stateDir
	repo.stateLock = flock.New(filepath.Join(stateDir, "repository.lock"))
	locked, err := repo.stateLock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("lock merge repository: %w", err)
	}
	if !locked {
		return nil, ErrMergeRepositoryLocked
	}
	if err := repo.loadPersistedRepository(); err != nil {
		return nil, unlockAfterFailedOpen(repo.stateLock, err)
	}
	if err := repo.RecoverMergeTransactions(); err != nil {
		return nil, unlockAfterFailedOpen(repo.stateLock, err)
	}
	return repo, nil
}

func unlockAfterFailedOpen(lock *flock.Flock, operationErr error) error {
	if err := lock.Unlock(); err != nil {
		return errors.Join(operationErr, fmt.Errorf("unlock merge repository after failed open: %w", err))
	}
	return operationErr
}

// InitializeRepository creates and durably stores a seeded repository.
func InitializeRepository(stateDir string) (*Repository, error) {
	if _, err := os.Stat(filepath.Join(stateDir, "repository.json")); err == nil {
		return nil, ErrRepositoryAlreadyInitialized
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect repository state: %w", err)
	}
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		return nil, err
	}
	repo.mu.Lock()
	if err := repo.persistRepositoryLocked(); err != nil {
		repo.mu.Unlock()
		_ = repo.Close()
		return nil, fmt.Errorf("persist initialized repository: %w", err)
	}
	repo.mu.Unlock()
	return repo, nil
}

// OpenRepository opens an initialized repository without creating state for a new target.
func OpenRepository(stateDir string) (*Repository, error) {
	if _, err := os.Stat(filepath.Join(stateDir, "repository.json")); os.IsNotExist(err) {
		return nil, ErrRepositoryNotInitialized
	} else if err != nil {
		return nil, fmt.Errorf("inspect repository state: %w", err)
	}
	return NewSeedRepositoryWithMergeState(stateDir)
}

// Close marks the repository unusable and releases its process lock; it is safe to call repeatedly.
func (r *Repository) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	lock := r.stateLock
	r.stateLock = nil
	r.mu.Unlock()
	if lock != nil {
		if err := lock.Unlock(); err != nil {
			return fmt.Errorf("unlock merge repository: %w", err)
		}
	}
	return nil
}

// RecoverMergeTransactions restores valid durable merge transactions and discards invalid records.
func (r *Repository) RecoverMergeTransactions() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return err
	}
	if r.mergeStateDir == "" {
		return nil
	}
	entries, err := os.ReadDir(r.mergeStateDir)
	if err != nil {
		return fmt.Errorf("read merge state directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == filepath.Base(r.repositoryStatePath()) || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(r.mergeStateDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read merge state %q: %w", path, err)
		}
		var state persistedMergeTransaction
		if err := json.Unmarshal(data, &state); err == nil && r.validPersistedMergeTransactionLocked(state) {
			r.mergeLeases[state.TargetBranch] = state.LeaseOwner
			r.mergeTransactions[state.TargetBranch] = state.Transaction
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("discard invalid merge state %q: %w", path, err)
		}
		if err := syncMergeStateDirectory(r.mergeStateDir); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) validPersistedMergeTransactionLocked(state persistedMergeTransaction) bool {
	transaction := state.Transaction
	if state.Version != 1 || state.TargetBranch == "" || state.LeaseOwner == "" ||
		state.TargetBranch != transaction.TargetBranch || state.LeaseOwner != transaction.OwnerTransactionID ||
		transaction.SourceBranch == "" || transaction.Binding.MergeBase == "" ||
		transaction.Binding.SourceCommit == "" || transaction.Binding.TargetCommit == "" || transaction.OriginalTarget == "" ||
		transaction.Preview.ID == "" || transaction.Preview.Binding != transaction.Binding ||
		transaction.Preview.SourceBranch != transaction.SourceBranch || transaction.Preview.TargetBranch != transaction.TargetBranch {
		return false
	}
	if transaction.Restaged && !transaction.Resolved {
		return false
	}
	if transaction.Resolved {
		if transaction.StagedSnapshot == "" {
			return false
		}
		if _, ok := r.snapshots[transaction.StagedSnapshot]; !ok {
			return false
		}
	} else if transaction.StagedSnapshot != "" {
		return false
	}
	if _, ok := r.commits[transaction.Binding.MergeBase]; !ok {
		return false
	}
	if _, ok := r.commits[transaction.Binding.SourceCommit]; !ok {
		return false
	}
	if _, ok := r.commits[transaction.Binding.TargetCommit]; !ok {
		return false
	}
	if r.branches[transaction.SourceBranch] != transaction.Binding.SourceCommit ||
		r.branches[state.TargetBranch] != transaction.Binding.TargetCommit ||
		transaction.OriginalTarget != transaction.Binding.TargetCommit {
		return false
	}
	mergeBase, ok := r.mergeBaseLocked(transaction.Binding.SourceCommit, transaction.Binding.TargetCommit)
	if !ok || mergeBase != transaction.Binding.MergeBase {
		return false
	}
	candidate, err := r.previewMergeLocked(transaction.SourceBranch, state.TargetBranch)
	return err == nil && candidate.preview.ID == transaction.Preview.ID && reflect.DeepEqual(candidate.preview, transaction.Preview)
}

func (r *Repository) persistedRepositoryLocked() persistedRepository {
	return persistedRepository{
		DefaultBranch:   r.defaultBranch,
		ActiveBranch:    r.activeBranch,
		Branches:        r.branches,
		Commits:         r.commits,
		Snapshots:       r.snapshots,
		Projections:     r.projections,
		EdgeProjections: r.edgeProjections,
		Objects:         r.objects,
		StagedMutations: r.stagedMutations,
	}
}

func (r *Repository) loadPersistedRepository() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, err := os.ReadFile(r.repositoryStatePath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read repository state: %w", err)
	}
	var state persistedRepository
	if err := json.Unmarshal(data, &state); err != nil || !state.valid() {
		return fmt.Errorf("decode repository state: invalid durable repository")
	}
	r.restorePersistedRepositoryLocked(state)
	return nil
}

func (r *Repository) persistRepositoryLocked() error {
	if r.persistRepositoryFn != nil {
		return r.persistRepositoryFn()
	}
	if r.mergeStateDir == "" {
		return nil
	}
	data, err := json.Marshal(r.persistedRepositoryLocked())
	if err != nil {
		return fmt.Errorf("encode repository state: %w", err)
	}
	return writeDurableStateFile(r.repositoryStatePath(), data)
}

func (r *Repository) restorePersistedRepositoryLocked(state persistedRepository) {
	r.defaultBranch, r.activeBranch = state.DefaultBranch, state.ActiveBranch
	r.branches, r.commits, r.snapshots = state.Branches, state.Commits, state.Snapshots
	r.projections, r.objects = state.Projections, state.Objects
	r.edgeProjections = state.EdgeProjections
	if r.edgeProjections == nil {
		r.edgeProjections = make(map[ObjectID]map[string]Edge, len(r.snapshots))
		for snapshotID, snapshot := range r.snapshots {
			edges, _ := state.canonicalEdgeProjection(snapshot)
			r.edgeProjections[snapshotID] = edges
		}
	}
	r.stagedMutations = state.StagedMutations
	if r.stagedMutations == nil {
		r.stagedMutations = make(map[string]StagedMutationSet)
	}
}

func (r persistedRepository) valid() bool {
	if r.DefaultBranch == "" || r.ActiveBranch == "" || r.Branches == nil || r.Commits == nil || r.Snapshots == nil || r.Projections == nil || r.Objects == nil {
		return false
	}
	if _, ok := r.Branches[r.DefaultBranch]; !ok {
		return false
	}
	if _, ok := r.Branches[r.ActiveBranch]; !ok {
		return false
	}
	for _, head := range r.Branches {
		if _, ok := r.Commits[head]; !ok {
			return false
		}
	}
	for _, commit := range r.Commits {
		if _, ok := r.Snapshots[commit.Snapshot]; !ok {
			return false
		}
		for _, parent := range commit.Parents {
			if _, ok := r.Commits[parent]; !ok {
				return false
			}
		}
	}
	referencedProjections := make(map[ObjectID]struct{}, len(r.Snapshots))
	for snapshotID, snapshot := range r.Snapshots {
		projection, ok := r.Projections[snapshot.NodeRoot]
		if !ok || !r.validStoredObject(snapshotID, "graph-snapshot", snapshot) || !r.validProjection(snapshot.NodeRoot, projection) || !r.validSnapshotRoots(snapshot) {
			return false
		}
		if !r.validSnapshotSchema(snapshot) {
			return false
		}
		if r.EdgeProjections != nil && !r.validEdgeProjection(snapshotID, snapshot) {
			return false
		}
		referencedProjections[snapshot.NodeRoot] = struct{}{}
	}
	for commitID, commit := range r.Commits {
		if !r.validStoredObject(commitID, "commit", commit) {
			return false
		}
	}
	for nodeRoot := range r.Projections {
		if _, ok := referencedProjections[nodeRoot]; !ok {
			return false
		}
	}
	for branch, staged := range r.StagedMutations {
		if branch == "" || staged.Branch != branch || staged.BaseCommit == "" ||
			(len(staged.Operations) == 0 && staged.TargetSchema == nil) {
			return false
		}
		if _, ok := r.Branches[branch]; !ok {
			return false
		}
		if _, ok := r.Commits[staged.BaseCommit]; !ok {
			return false
		}
		normalized, err := normalizeStoredMutationOperations(staged.Operations)
		if err != nil || !reflect.DeepEqual(staged.Operations, normalized) {
			return false
		}
		if staged.TargetSchema != nil {
			normalizedSchema, err := staged.TargetSchema.Normalize()
			if err != nil || !reflect.DeepEqual(*staged.TargetSchema, normalizedSchema) {
				return false
			}
		}
		edgeProjections := r.EdgeProjections
		if edgeProjections == nil {
			edgeProjections = make(map[ObjectID]map[string]Edge, len(r.Snapshots))
			for snapshotID, snapshot := range r.Snapshots {
				edges, ok := r.canonicalEdgeProjection(snapshot)
				if !ok {
					return false
				}
				edgeProjections[snapshotID] = edges
			}
		}
		validator := Repository{
			commits: r.Commits, snapshots: r.Snapshots, projections: r.Projections,
			edgeProjections: edgeProjections, objects: r.Objects,
		}
		if _, _, err := validator.candidateGraphLocked(staged.BaseCommit, staged); err != nil {
			return false
		}
	}
	return true
}

func (r persistedRepository) validSnapshotSchema(snapshot graphSnapshot) bool {
	nodes, ok := r.canonicalNodeProjection(snapshot.NodeRoot)
	if !ok {
		return false
	}
	edges, ok := r.canonicalEdgeProjection(snapshot)
	if !ok {
		return false
	}
	schema, err := (&Repository{objects: r.Objects}).schemaSnapshotLocked(snapshot.SchemaRoot)
	return err == nil && ValidateSchemaSnapshot(schema, nodes, edges) == nil
}

func (r persistedRepository) validStoredObject(id ObjectID, objectType string, value any) bool {
	encoded, err := canonicalObjectEncoding(value)
	return err == nil && id == persistedObjectID(objectType, value) && bytes.Equal(r.Objects[id], encoded)
}

func (r persistedRepository) validProjection(nodeRoot ObjectID, projection map[string]Node) bool {
	canonical, ok := r.canonicalNodeProjection(nodeRoot)
	if !ok || len(projection) != len(canonical) {
		return false
	}
	for nodeID, node := range canonical {
		if !projection[nodeID].Equal(node) {
			return false
		}
	}
	return true
}

func (r persistedRepository) validSnapshotRoots(snapshot graphSnapshot) bool {
	if _, ok := r.canonicalRoot(snapshot.EdgeRoot, "prolly-edge-root"); !ok {
		return false
	}
	if _, ok := r.canonicalRoot(snapshot.OutAdjRoot, "prolly-out-adjacency-root"); !ok {
		return false
	}
	if _, ok := r.canonicalRoot(snapshot.InAdjRoot, "prolly-in-adjacency-root"); !ok {
		return false
	}
	var schema SchemaSnapshot
	schemaBytes, ok := r.Objects[snapshot.SchemaRoot]
	if !ok {
		return false
	}
	if cbor.Unmarshal(schemaBytes, &schema) == nil && r.validStoredObject(snapshot.SchemaRoot, "schema-root", schema) {
		return true
	}
	var legacySchema map[string]string
	return cbor.Unmarshal(schemaBytes, &legacySchema) == nil && legacySchema["version"] == "v1" &&
		r.validLegacyStoredObject(snapshot.SchemaRoot, "schema-root", legacySchema)
}

func (r persistedRepository) validEdgeProjection(snapshotID ObjectID, snapshot graphSnapshot) bool {
	canonical, ok := r.canonicalEdgeProjection(snapshot)
	if !ok || len(r.EdgeProjections[snapshotID]) != len(canonical) {
		return false
	}
	for edgeID, edge := range canonical {
		if !r.EdgeProjections[snapshotID][edgeID].Equal(edge) {
			return false
		}
	}
	return true
}

func (r persistedRepository) canonicalNodeProjection(nodeRoot ObjectID) (map[string]Node, bool) {
	nodeIDs, ok := r.canonicalRoot(nodeRoot, "prolly-node-root")
	if !ok {
		return nil, false
	}
	canonical := make(map[string]Node, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		var node Node
		nodeBytes, ok := r.Objects[nodeID]
		if !ok {
			return nil, false
		}
		if cbor.Unmarshal(nodeBytes, &node) != nil || !r.validStoredObject(nodeID, "node", node) {
			var legacy legacyNode
			if cbor.Unmarshal(nodeBytes, &legacy) != nil || !r.validLegacyStoredObject(nodeID, "node", legacy) {
				return nil, false
			}
			node = Node{ID: legacy.ID, Title: legacy.Title}
		}
		if _, duplicate := canonical[node.ID]; duplicate {
			return nil, false
		}
		canonical[node.ID] = node
	}
	return canonical, true
}

func (r persistedRepository) canonicalEdgeProjection(snapshot graphSnapshot) (map[string]Edge, bool) {
	edgeIDs, ok := r.canonicalRoot(snapshot.EdgeRoot, "prolly-edge-root")
	if !ok {
		return nil, false
	}
	canonical := make(map[string]Edge, len(edgeIDs))
	for _, edgeID := range edgeIDs {
		var edge Edge
		edgeBytes, ok := r.Objects[edgeID]
		if !ok {
			return nil, false
		}
		if cbor.Unmarshal(edgeBytes, &edge) != nil || !r.validStoredObject(edgeID, "edge", edge) {
			var legacy legacyEdge
			if cbor.Unmarshal(edgeBytes, &legacy) != nil || !r.validLegacyStoredObject(edgeID, "edge", legacy) {
				return nil, false
			}
			edge = Edge{ID: legacy.ID, Source: legacy.Source, Target: legacy.Target}
		}
		if edge.ID == "" || edge.Source == "" || edge.Target == "" {
			return nil, false
		}
		if _, duplicate := canonical[edge.ID]; duplicate {
			return nil, false
		}
		canonical[edge.ID] = edge
	}
	return canonical, true
}

func (r persistedRepository) canonicalRoot(rootID ObjectID, objectType string) ([]ObjectID, bool) {
	var entries []ObjectID
	encoded, ok := r.Objects[rootID]
	if !ok || cbor.Unmarshal(encoded, &entries) != nil || !r.validStoredObject(rootID, objectType, entries) {
		return nil, false
	}
	return entries, true
}

func (r *Repository) persistMergeTransactionLocked(targetBranch, leaseOwner string, transaction *mergeTransaction) error {
	if r.mergeStateDir == "" {
		return nil
	}
	if r.persistStateFn != nil {
		return r.persistStateFn(targetBranch, leaseOwner, transaction)
	}
	path := r.mergeStatePath(targetBranch)
	if transaction == nil {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove merge state: %w", err)
		}
		if err := syncMergeStateDirectory(r.mergeStateDir); err != nil {
			return durableWriteCommittedError{err: err}
		}
		return nil
	}
	data, err := json.Marshal(persistedMergeTransaction{Version: 1, TargetBranch: targetBranch, LeaseOwner: leaseOwner, Transaction: *transaction})
	if err != nil {
		return fmt.Errorf("encode merge state: %w", err)
	}
	return writeDurableStateFile(path, data)
}

func writeDurableStateFile(path string, data []byte) (err error) {
	temp, err := os.CreateTemp(filepath.Dir(path), ".merge-state-*")
	if err != nil {
		return fmt.Errorf("create merge state temp file: %w", err)
	}
	tempPath := temp.Name()
	defer func() {
		if removeErr := os.Remove(tempPath); removeErr != nil && !os.IsNotExist(removeErr) {
			removeErr = fmt.Errorf("remove merge state temp file: %w", removeErr)
			if err == nil {
				err = removeErr
			} else {
				err = errors.Join(err, removeErr)
			}
		}
	}()
	if _, err := temp.Write(data); err != nil {
		return closeAfterWriteFailure(temp, fmt.Errorf("write merge state: %w", err))
	}
	if err := temp.Sync(); err != nil {
		return closeAfterWriteFailure(temp, fmt.Errorf("sync merge state: %w", err))
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close merge state: %w", err)
	}
	if err := replaceDurableStateFile(tempPath, path); err != nil {
		return fmt.Errorf("replace merge state: %w", err)
	}
	return nil
}

func closeAfterWriteFailure(temp *os.File, operationErr error) error {
	if err := temp.Close(); err != nil {
		return errors.Join(operationErr, fmt.Errorf("close merge state after write failure: %w", err))
	}
	return operationErr
}

func durableWriteCommitted(err error) bool {
	var committed durableWriteCommittedError
	return errors.As(err, &committed)
}

func (r *Repository) mergeStatePath(targetBranch string) string {
	sum := sha256.Sum256([]byte(targetBranch))
	return filepath.Join(r.mergeStateDir, hex.EncodeToString(sum[:])+".json")
}

func (r *Repository) repositoryStatePath() string {
	return filepath.Join(r.mergeStateDir, "repository.json")
}

func persistedObjectID(objectType string, value any) ObjectID {
	encoded, err := canonicalObjectEncoding(value)
	if err != nil {
		panic(fmt.Sprintf("encode %s: %v", objectType, err))
	}
	header := objectType + " " + strconv.Itoa(len(encoded)) + "\x00"
	sum := blake3.Sum256(append([]byte(header), encoded...))
	return ObjectID(hex.EncodeToString(sum[:]))
}

func (r persistedRepository) validLegacyStoredObject(id ObjectID, objectType string, value any) bool {
	encoded, err := canonicalCBOR.Marshal(value)
	return err == nil && id == legacyPersistedObjectID(objectType, encoded) && bytes.Equal(r.Objects[id], encoded)
}

func legacyPersistedObjectID(objectType string, encoded []byte) ObjectID {
	header := objectType + " " + strconv.Itoa(len(encoded)) + "\x00"
	sum := blake3.Sum256(append([]byte(header), encoded...))
	return ObjectID(hex.EncodeToString(sum[:]))
}
