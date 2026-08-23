package repository

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"unicode"

	"github.com/fxamacker/cbor/v2"
	"github.com/gofrs/flock"
	"github.com/pelletier/go-toml/v2"
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

const repositoryFormatVersion = 1

type repositoryConfig struct {
	FormatVersion            int    `toml:"format_version"`
	DefaultBranch            string `toml:"default_branch"`
	ReflogRetentionInventory bool   `toml:"reflog_retention_inventory"`
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

// NewSeedRepositoryWithMergeState opens stateDir or initializes a new seeded repository.
func NewSeedRepositoryWithMergeState(stateDir string) (*Repository, error) {
	if err := rejectLegacyRepositoryState(stateDir); err != nil {
		return nil, err
	}
	if err := ensureDurableDirectory(stateDir); err != nil {
		return nil, fmt.Errorf("create repository state directory: %w", err)
	}
	repo := newRepository()
	repo.mergeStateDir = stateDir
	repo.objectStore = newLooseObjectStore(stateDir, &repo.objects)
	repo.stateLock = flock.New(filepath.Join(stateDir, "repository.lock"))
	locked, err := repo.stateLock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("lock merge repository: %w", err)
	}
	if !locked {
		return nil, ErrMergeRepositoryLocked
	}
	loaded, err := repo.loadControlState()
	if err != nil {
		return nil, closeAfterFailedOpen(repo, err)
	}
	if !loaded {
		if err := repo.seed(); err != nil {
			return nil, closeAfterFailedOpen(repo, fmt.Errorf("seed repository: %w", err))
		}
		if err := repo.initializeControlStateLocked(); err != nil {
			return nil, closeAfterFailedOpen(repo, fmt.Errorf("initialize repository control state: %w", err))
		}
	}
	if err := repo.RecoverMergeTransactions(); err != nil {
		return nil, closeAfterFailedOpen(repo, err)
	}
	if err := repo.ensureProjectionForActiveBranchLocked(); err != nil {
		return nil, closeAfterFailedOpen(repo, err)
	}
	return repo, nil
}

func unlockAfterFailedOpen(lock *flock.Flock, operationErr error) error {
	if err := lock.Unlock(); err != nil {
		return errors.Join(operationErr, fmt.Errorf("unlock merge repository after failed open: %w", err))
	}
	return operationErr
}

func closeAfterFailedOpen(repo *Repository, operationErr error) error {
	projectionErr := repo.closeProjectionLocked()
	packErr := repo.objectStore.closePackGeneration()
	return unlockAfterFailedOpen(repo.stateLock, errors.Join(operationErr, projectionErr, packErr))
}

// InitializeRepository creates and durably stores a seeded repository.
func InitializeRepository(stateDir string) (*Repository, error) {
	if err := rejectLegacyRepositoryState(stateDir); err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(stateDir, "config.toml")); err == nil {
		return nil, ErrRepositoryAlreadyInitialized
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect repository configuration: %w", err)
	}
	return NewSeedRepositoryWithMergeState(stateDir)
}

// OpenRepository opens an initialized repository without creating state for a new target.
func OpenRepository(stateDir string) (*Repository, error) {
	if err := rejectLegacyRepositoryState(stateDir); err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(stateDir, "config.toml")); os.IsNotExist(err) {
		return nil, ErrRepositoryNotInitialized
	} else if err != nil {
		return nil, fmt.Errorf("inspect repository configuration: %w", err)
	}
	return openControlRepository(stateDir)
}

func openControlRepository(stateDir string) (*Repository, error) {
	repo := newRepository()
	repo.mergeStateDir = stateDir
	repo.objectStore = newLooseObjectStore(stateDir, &repo.objects)
	repo.stateLock = flock.New(filepath.Join(stateDir, "repository.lock"))
	locked, err := repo.stateLock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("lock repository: %w", err)
	}
	if !locked {
		return nil, ErrMergeRepositoryLocked
	}
	loaded, err := repo.loadControlState()
	if err != nil {
		return nil, closeAfterFailedOpen(repo, err)
	}
	if !loaded {
		return nil, closeAfterFailedOpen(repo, ErrRepositoryNotInitialized)
	}
	if err := repo.RecoverMergeTransactions(); err != nil {
		return nil, closeAfterFailedOpen(repo, err)
	}
	if err := repo.ensureProjectionForActiveBranchLocked(); err != nil {
		return nil, closeAfterFailedOpen(repo, err)
	}
	return repo, nil
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
	projectionErr := r.closeProjectionLocked()
	packErr := r.objectStore.closePackGeneration()
	r.mu.Unlock()
	if lock != nil {
		if err := lock.Unlock(); err != nil {
			return errors.Join(projectionErr, packErr, fmt.Errorf("unlock merge repository: %w", err))
		}
	}
	return errors.Join(projectionErr, packErr)
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
	entries, err := os.ReadDir(r.mergeDirectory())
	if err != nil {
		return fmt.Errorf("read merge state directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(r.mergeDirectory(), entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read merge state %q: %w", path, err)
		}
		var state persistedMergeTransaction
		if err := json.Unmarshal(data, &state); err == nil &&
			r.loadMergeTransactionSnapshotLocked(state) &&
			r.validPersistedMergeTransactionLocked(state) {
			r.mergeLeases[state.TargetBranch] = state.LeaseOwner
			r.mergeTransactions[state.TargetBranch] = state.Transaction
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("discard invalid merge state %q: %w", path, err)
		}
		if err := syncMergeStateDirectory(r.mergeDirectory()); err != nil {
			return err
		}
	}
	return nil
}

// loadMergeTransactionSnapshotLocked restores the otherwise-unreachable graph
// selected by a resolved merge before validating its durable transaction.
func (r *Repository) loadMergeTransactionSnapshotLocked(state persistedMergeTransaction) bool {
	transaction := state.Transaction
	if !transaction.Resolved {
		return true
	}
	if transaction.StagedSnapshot == "" || !validLooseObjectID(transaction.StagedSnapshot) {
		return false
	}
	return r.loadSnapshotLocked(transaction.StagedSnapshot) == nil
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
	// The immutable commits and their graph objects were reconstructed and
	// validated before recovery. Retain the durable preview rather than
	// regenerating it here: recovery must not alter a recorded transaction.
	return true
}

// persistRepositoryLocked exists for test injection and for callers that need
// to reconcile every mutable control file. Normal mutation paths use the
// narrower helpers below so each ref transition has one reflog entry.
func (r *Repository) persistRepositoryLocked() error {
	if err := r.persistenceHook(); err != nil {
		return err
	}
	if r.mergeStateDir == "" {
		return nil
	}
	if err := r.writeConfigLocked(); err != nil {
		return err
	}
	if err := writeDurableStateFile(r.headPath(), []byte(r.activeBranch+"\n")); err != nil {
		return err
	}
	for name, commitID := range r.branches {
		if err := r.writeRefValueLocked(name, commitID); err != nil {
			return err
		}
	}
	for name, staged := range r.stagedMutations {
		if err := r.writeStagedLocked(name, &staged); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) persistedRepositoryLocked() persistedRepository {
	return persistedRepository{
		DefaultBranch: r.defaultBranch, ActiveBranch: r.activeBranch, Branches: r.branches,
		Commits: r.commits, Snapshots: r.snapshots, Projections: r.projections,
		EdgeProjections: r.edgeProjections, Objects: r.objects, StagedMutations: r.stagedMutations,
	}
}

func rejectLegacyRepositoryState(stateDir string) error {
	if _, err := os.Stat(filepath.Join(stateDir, "repository.json")); err == nil {
		return fmt.Errorf("%w: %s", ErrLegacyRepositoryState, filepath.Join(stateDir, "repository.json"))
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect legacy repository state: %w", err)
	}
	return nil
}

func (r *Repository) initializeControlStateLocked() error {
	if err := r.ensureControlDirectories(); err != nil {
		return err
	}
	if err := r.writeReflogRetentionInventoryLocked(nil); err != nil {
		return err
	}
	if err := r.writeConfigLocked(); err != nil {
		return err
	}
	if err := r.writeRefLocked(r.defaultBranch, "", r.branches[r.defaultBranch], "initialize"); err != nil {
		return err
	}
	return r.writeHeadLocked("", r.activeBranch, "initialize")
}

func (r *Repository) loadControlState() (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, err := os.ReadFile(r.configPath())
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read repository configuration: %w", err)
	}
	var config repositoryConfig
	if err := toml.Unmarshal(data, &config); err != nil || config.FormatVersion != repositoryFormatVersion || !validRefName(config.DefaultBranch) {
		return false, fmt.Errorf("decode repository configuration: invalid durable repository")
	}
	head, err := readControlValue(r.headPath())
	if err != nil || !validRefName(head) {
		return false, fmt.Errorf("read HEAD: invalid durable repository")
	}
	branches, err := r.readRefsLocked()
	if err != nil {
		return false, err
	}
	if len(branches) == 0 || branches[config.DefaultBranch] == "" || branches[head] == "" {
		return false, fmt.Errorf("decode repository control state: invalid durable repository")
	}
	if err := r.initializeReflogRetentionInventoryLocked(config.ReflogRetentionInventory); err != nil {
		return false, fmt.Errorf("load reflog retention inventory: %w", err)
	}
	r.defaultBranch, r.activeBranch, r.branches = config.DefaultBranch, head, branches
	r.commits, r.snapshots = make(map[ObjectID]commit), make(map[ObjectID]graphSnapshot)
	r.projections, r.edgeProjections = make(map[ObjectID]map[string]Node), make(map[ObjectID]map[string]Edge)
	for _, commitID := range branches {
		if err := r.loadCommitLocked(commitID, make(map[ObjectID]bool)); err != nil {
			return false, fmt.Errorf("load repository objects: %w", err)
		}
	}
	staged, err := r.readStagedMutationsLocked()
	if err != nil {
		return false, err
	}
	r.stagedMutations = staged
	for branch, mutationSet := range staged {
		if _, exists := branches[branch]; !exists || mutationSet.Branch != branch || mutationSet.BaseCommit == "" ||
			(len(mutationSet.Operations) == 0 && mutationSet.TargetSchema == nil) {
			return false, fmt.Errorf("load staged mutations: invalid durable repository")
		}
		normalized, err := normalizeStoredMutationOperations(mutationSet.Operations)
		if err != nil || !reflect.DeepEqual(normalized, mutationSet.Operations) {
			return false, fmt.Errorf("load staged mutations: invalid durable repository")
		}
		if mutationSet.TargetSchema != nil {
			normalizedSchema, err := mutationSet.TargetSchema.Normalize()
			if err != nil || !reflect.DeepEqual(normalizedSchema, *mutationSet.TargetSchema) {
				return false, fmt.Errorf("load staged mutations: invalid durable repository")
			}
		}
		if _, _, err := r.candidateGraphLocked(mutationSet.BaseCommit, mutationSet); err != nil {
			return false, fmt.Errorf("load staged mutations: invalid durable repository")
		}
	}
	return true, nil
}

func (r *Repository) loadCommitLocked(id ObjectID, visiting map[ObjectID]bool) error {
	if _, loaded := r.commits[id]; loaded {
		return nil
	}
	if visiting[id] {
		return errors.New("commit ancestry contains a cycle")
	}
	visiting[id] = true
	defer delete(visiting, id)
	var value commit
	if err := r.loadObject(id, "commit", &value); err != nil {
		return err
	}
	if value.Snapshot == "" {
		return errors.New("commit has no snapshot")
	}
	for _, parent := range value.Parents {
		if err := r.loadCommitLocked(parent, visiting); err != nil {
			return err
		}
	}
	if err := r.loadSnapshotLocked(value.Snapshot); err != nil {
		return err
	}
	r.commits[id] = value
	return nil
}

func (r *Repository) loadSnapshotLocked(id ObjectID) error {
	if _, loaded := r.snapshots[id]; loaded {
		return nil
	}
	var snapshot graphSnapshot
	if err := r.loadObject(id, "graph-snapshot", &snapshot); err != nil {
		return err
	}
	var schema SchemaSnapshot
	if err := r.loadObject(snapshot.SchemaRoot, "schema-root", &schema); err != nil {
		return err
	}
	normalizedSchema, err := schema.Normalize()
	if err != nil || !reflect.DeepEqual(schema, normalizedSchema) {
		return errors.New("invalid snapshot schema")
	}
	r.snapshots[id] = snapshot
	if err := r.reconstructSnapshotProjectionsLocked(id, snapshot); err != nil {
		delete(r.snapshots, id)
		return err
	}
	nodes, edges := r.projections[snapshot.NodeRoot], r.edgeProjections[id]
	if err := ValidateSchemaSnapshot(schema, nodes, edges); err != nil {
		delete(r.snapshots, id)
		delete(r.projections, snapshot.NodeRoot)
		delete(r.edgeProjections, id)
		return fmt.Errorf("validate snapshot schema: %w", err)
	}
	return nil
}

func (r *Repository) loadObject(id ObjectID, objectType string, target any) error {
	data, err := r.objectStore.get(id, objectType)
	if err != nil {
		return err
	}
	if err := cbor.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode %s %s: %w", objectType, id, err)
	}
	encoded, err := canonicalObjectEncoding(reflect.ValueOf(target).Elem().Interface())
	if err != nil || !bytes.Equal(data, encoded) {
		return fmt.Errorf("decode %s %s: non-canonical object", objectType, id)
	}
	return nil
}

func (r *Repository) readRefsLocked() (map[string]ObjectID, error) {
	refs := make(map[string]ObjectID)
	root := r.refsDirectory()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return errors.New("non-regular branch ref")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		if !validRefName(name) {
			return errors.New("invalid branch ref name")
		}
		value, err := readControlValue(path)
		if err != nil || !validLooseObjectID(ObjectID(value)) {
			return errors.New("invalid branch ref")
		}
		refs[name] = ObjectID(value)
		return nil
	})
	if os.IsNotExist(err) {
		return nil, errors.New("branch refs directory is missing")
	}
	if err != nil {
		return nil, fmt.Errorf("read branch refs: %w", err)
	}
	return refs, nil
}

func (r *Repository) readStagedMutationsLocked() (map[string]StagedMutationSet, error) {
	staged := make(map[string]StagedMutationSet)
	root := r.stagedDirectory()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() || filepath.Ext(path) != ".json" {
			return errors.New("invalid staged mutation file")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(filepath.ToSlash(relative), ".json")
		if !validRefName(name) {
			return errors.New("invalid staged mutation branch")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var mutationSet StagedMutationSet
		if json.Unmarshal(data, &mutationSet) != nil || mutationSet.Branch != name {
			return errors.New("invalid staged mutation file")
		}
		staged[name] = mutationSet
		return nil
	})
	if os.IsNotExist(err) {
		return nil, errors.New("staged mutations directory is missing")
	}
	if err != nil {
		return nil, fmt.Errorf("read staged mutations: %w", err)
	}
	return staged, nil
}

func (r *Repository) persistenceHook() error {
	if r.persistRepositoryFn != nil {
		return r.persistRepositoryFn()
	}
	return nil
}

func (r *Repository) writeConfigLocked() error {
	if r.mergeStateDir == "" {
		return nil
	}
	data, err := toml.Marshal(repositoryConfig{
		FormatVersion: repositoryFormatVersion, DefaultBranch: r.defaultBranch, ReflogRetentionInventory: true,
	})
	if err != nil {
		return fmt.Errorf("encode repository configuration: %w", err)
	}
	return writeDurableStateFile(r.configPath(), data)
}

func (r *Repository) writeHeadLocked(previous, next, action string) error {
	if err := r.persistenceHook(); err != nil {
		return err
	}
	if r.mergeStateDir == "" {
		return nil
	}
	return r.replaceThenReflogLocked(
		func() error { return writeDurableStateFile(r.headPath(), []byte(next+"\n")) },
		"HEAD", ObjectID(previous), ObjectID(next), action,
	)
}

func (r *Repository) writeRefLocked(branch string, previous, next ObjectID, action string) error {
	if !validRefName(branch) {
		return fmt.Errorf("invalid branch name %q", branch)
	}
	if err := r.persistenceHook(); err != nil {
		return err
	}
	if r.mergeStateDir == "" {
		return nil
	}
	return r.replaceThenReflogLocked(
		func() error { return r.writeRefValueLocked(branch, next) },
		filepath.ToSlash(filepath.Join("refs", "heads", branch)), previous, next, action,
	)
}

func (r *Repository) writeRefValueLocked(branch string, next ObjectID) error {
	path, err := r.refPath(branch)
	if err != nil {
		return err
	}
	if err := ensureDurableDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("create branch ref directory: %w", err)
	}
	return writeDurableStateFile(path, []byte(next+"\n"))
}

func (r *Repository) deleteRefLocked(branch string, previous ObjectID, action string) error {
	if err := r.persistenceHook(); err != nil {
		return err
	}
	if r.mergeStateDir == "" {
		return nil
	}
	path, err := r.refPath(branch)
	if err != nil {
		return err
	}
	return r.replaceThenReflogLocked(func() error {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove branch ref: %w", err)
		}
		if err := syncMergeStateDirectory(filepath.Dir(path)); err != nil {
			return durableWriteCommittedError{err: err}
		}
		return nil
	}, filepath.ToSlash(filepath.Join("refs", "heads", branch)), previous, "", action)
}

func (r *Repository) writeStagedLocked(branch string, staged *StagedMutationSet) error {
	if err := r.persistenceHook(); err != nil {
		return err
	}
	if r.mergeStateDir == "" {
		return nil
	}
	path, err := r.stagedPath(branch)
	if err != nil {
		return err
	}
	if staged == nil {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove staged mutations: %w", err)
		}
		if err := syncMergeStateDirectory(filepath.Dir(path)); err != nil {
			return durableWriteCommittedError{err: err}
		}
		return nil
	}
	data, err := json.Marshal(staged)
	if err != nil {
		return fmt.Errorf("encode staged mutations: %w", err)
	}
	if err := ensureDurableDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("create staged mutations directory: %w", err)
	}
	return writeDurableStateFile(path, data)
}

// replaceThenReflogLocked records a reflog entry only after the corresponding
// ref replacement has happened. Once replacement succeeds, every later error
// is a committed-with-warning error: rolling back memory would otherwise lie
// about the durable ref.
func (r *Repository) replaceThenReflogLocked(replace func() error, ref string, previous, next ObjectID, action string) error {
	reflogCreated, reflogErr := r.prepareReflogLocked(ref)
	if reflogErr != nil && !durableWriteCommitted(reflogErr) {
		cleanupErr := r.removePreparedReflogLocked(ref, reflogCreated)
		if cleanupErr != nil {
			return fmt.Errorf("prepare reflog: %w (cleanup failed: %v)", reflogErr, cleanupErr)
		}
		return fmt.Errorf("prepare reflog: %w", reflogErr)
	}
	inventoryErr := r.recordReflogRetentionPathLocked(ref, reflogCreated)
	if inventoryErr != nil && !durableWriteCommitted(inventoryErr) {
		cleanupErr := r.removePreparedReflogLocked(ref, reflogCreated)
		if reflogErr != nil {
			if cleanupErr != nil {
				return fmt.Errorf("record reflog retention inventory after reflog preparation warning (%v; cleanup failed: %v): %w", reflogErr, cleanupErr, inventoryErr)
			}
			return fmt.Errorf("record reflog retention inventory after reflog preparation warning (%v): %w", reflogErr, inventoryErr)
		}
		if cleanupErr != nil {
			return fmt.Errorf("record reflog retention inventory: %w (cleanup failed: %v)", inventoryErr, cleanupErr)
		}
		return fmt.Errorf("record reflog retention inventory: %w", inventoryErr)
	}
	replaceErr := replace()
	if replaceErr != nil && !durableWriteCommitted(replaceErr) {
		if reflogErr != nil || inventoryErr != nil {
			// The reflog and inventory may have reached disk even though this
			// ref replacement did not. Do not classify this as a committed ref.
			return fmt.Errorf("replace ref after reflog preparation or retention inventory warning (%v): %w", errors.Join(reflogErr, inventoryErr), replaceErr)
		}
		return replaceErr
	}
	appendErr := r.appendReflogLocked(ref, previous, next, action)
	if reflogErr == nil && inventoryErr == nil && replaceErr == nil && appendErr == nil {
		return nil
	}
	return durableWriteCommittedError{err: errors.Join(reflogErr, inventoryErr, replaceErr, appendErr)}
}

// prepareReflogLocked creates and syncs an empty, valid reflog before it is
// listed by the retention inventory. Thus a ref replacement failure can leave
// a harmless tracked empty log, rather than an inventory entry with no log.
func (r *Repository) prepareReflogLocked(ref string) (bool, error) {
	if r.mergeStateDir == "" {
		return false, nil
	}
	path, err := r.reflogPath(ref)
	if err != nil {
		return false, err
	}
	if err := ensureDurableDirectory(filepath.Dir(path)); err != nil {
		return false, fmt.Errorf("create reflog directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		info, statErr := os.Lstat(path)
		if statErr != nil {
			return false, fmt.Errorf("inspect existing reflog: %w", statErr)
		}
		if !info.Mode().IsRegular() {
			return false, errors.New("existing reflog is not a regular file")
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create reflog: %w", err)
	}
	if err := file.Sync(); err != nil {
		return true, closeAfterWriteFailure(file, fmt.Errorf("sync empty reflog: %w", err))
	}
	if err := file.Close(); err != nil {
		return true, fmt.Errorf("close empty reflog: %w", err)
	}
	if err := syncMergeStateDirectory(filepath.Dir(path)); err != nil {
		return true, durableWriteCommittedError{err: fmt.Errorf("sync empty reflog directory: %w", err)}
	}
	return true, nil
}

func (r *Repository) removePreparedReflogLocked(ref string, created bool) error {
	if !created {
		return nil
	}
	path, err := r.reflogPath(ref)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect prepared reflog: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() != 0 {
		return errors.New("prepared reflog changed before cleanup")
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove prepared reflog: %w", err)
	}
	if err := syncMergeStateDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync prepared reflog directory: %w", err)
	}
	return nil
}

func (r *Repository) appendReflogLocked(ref string, previous, next ObjectID, action string) error {
	if r.appendReflogFn != nil {
		return r.appendReflogFn(ref, previous, next, action)
	}
	if r.mergeStateDir == "" {
		return nil
	}
	path, err := r.reflogPath(ref)
	if err != nil {
		return err
	}
	data, err := readValidatedReflogContent(path, ref)
	if err != nil {
		return fmt.Errorf("read reflog: %w", err)
	}
	data = append(data, fmt.Sprintf("%s %s %s\n", previous, next, action)...)
	if r.writeReflogFn != nil {
		return r.writeReflogFn(path, data)
	}
	return writeDurableStateFile(path, data)
}

func (r *Repository) ensureControlDirectories() error {
	if r.mergeStateDir == "" {
		return nil
	}
	for _, path := range []string{r.refsDirectory(), r.reflogDirectory(), r.stagedDirectory(), r.mergeDirectory()} {
		if err := ensureDurableDirectory(path); err != nil {
			return fmt.Errorf("create repository control directory: %w", err)
		}
	}
	return nil
}

func (r *Repository) configPath() string { return filepath.Join(r.mergeStateDir, "config.toml") }
func (r *Repository) headPath() string   { return filepath.Join(r.mergeStateDir, "HEAD") }
func (r *Repository) refsDirectory() string {
	return filepath.Join(r.mergeStateDir, "refs", "heads")
}
func (r *Repository) reflogDirectory() string { return filepath.Join(r.mergeStateDir, "logs") }
func (r *Repository) stagedDirectory() string { return filepath.Join(r.mergeStateDir, "staged") }
func (r *Repository) mergeDirectory() string  { return filepath.Join(r.mergeStateDir, "merge") }

func (r *Repository) refPath(branch string) (string, error) {
	return safeControlPath(r.refsDirectory(), branch)
}

func (r *Repository) stagedPath(branch string) (string, error) {
	return safeControlPath(r.stagedDirectory(), branch+".json")
}

func (r *Repository) reflogPath(ref string) (string, error) {
	if ref == "HEAD" {
		return filepath.Join(r.reflogDirectory(), "HEAD"), nil
	}
	return safeControlPath(r.reflogDirectory(), ref)
}

func validRefName(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
		return false
	}
	if strings.IndexFunc(name, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) >= 0 {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func safeControlPath(root, relative string) (string, error) {
	if !validRefName(relative) && relative != "refs/heads" {
		return "", fmt.Errorf("unsafe control path %q", relative)
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	cleanRelative, err := filepath.Rel(root, path)
	if err != nil || cleanRelative == ".." || strings.HasPrefix(cleanRelative, ".."+string(filepath.Separator)) || filepath.IsAbs(cleanRelative) {
		return "", fmt.Errorf("unsafe control path %q", relative)
	}
	return path, nil
}

func readControlValue(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	value := strings.TrimSuffix(string(data), "\n")
	if value == "" || strings.Contains(value, "\n") || strings.Contains(value, "\r") {
		return "", errors.New("invalid control value")
	}
	return value, nil
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
	if _, tree := r.prollyEntries(snapshot.NodeRoot); tree &&
		(snapshot.NodeCount != uint64(len(nodes)) || snapshot.EdgeCount != uint64(len(edges))) {
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
		return r.validTreeAdjacency(snapshot)
	}
	var legacySchema map[string]string
	return cbor.Unmarshal(schemaBytes, &legacySchema) == nil && legacySchema["version"] == "v1" &&
		r.validLegacyStoredObject(snapshot.SchemaRoot, "schema-root", legacySchema) && r.validTreeAdjacency(snapshot)
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
		if entries, tree := r.prollyEntries(nodeRoot); tree {
			found := false
			for _, entry := range entries {
				if entry.Value == nodeID {
					found = entry.Key == node.ID
					break
				}
			}
			if !found {
				return nil, false
			}
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
		if entries, tree := r.prollyEntries(snapshot.EdgeRoot); tree {
			found := false
			for _, entry := range entries {
				if entry.Value == edgeID {
					found = entry.Key == edge.ID
					break
				}
			}
			if !found {
				return nil, false
			}
		}
		canonical[edge.ID] = edge
	}
	return canonical, true
}

func (r persistedRepository) canonicalRoot(rootID ObjectID, objectType string) ([]ObjectID, bool) {
	if entries, ok := r.prollyEntries(rootID); ok {
		values := make([]ObjectID, len(entries))
		for i, entry := range entries {
			values[i] = entry.Value
		}
		return values, true
	}
	var entries []ObjectID
	encoded, ok := r.Objects[rootID]
	if !ok || cbor.Unmarshal(encoded, &entries) != nil || !r.validStoredObject(rootID, objectType, entries) {
		return nil, false
	}
	return entries, true
}

func (r persistedRepository) prollyEntries(rootID ObjectID) ([]prollyTreeEntry, bool) {
	encoded, ok := r.Objects[rootID]
	if !ok {
		return nil, false
	}
	var leaf prollyTreeLeaf
	if cbor.Unmarshal(encoded, &leaf) == nil && r.validStoredObject(rootID, prollyTreeLeafType, leaf) && validProllyEntries(leaf.Entries, true) {
		return append([]prollyTreeEntry(nil), leaf.Entries...), true
	}
	var internal prollyTreeInternal
	if cbor.Unmarshal(encoded, &internal) != nil || !r.validStoredObject(rootID, prollyTreeInternalType, internal) || !validProllyChildren(internal.Children) {
		return nil, false
	}
	entries := make([]prollyTreeEntry, 0)
	for _, child := range internal.Children {
		childEntries, ok := r.prollyEntries(child.Object)
		if !ok || len(childEntries) == 0 || childEntries[len(childEntries)-1].Key != child.LastKey {
			return nil, false
		}
		entries = append(entries, childEntries...)
	}
	return entries, validProllyEntries(entries, false)
}

func (r persistedRepository) validTreeAdjacency(snapshot graphSnapshot) bool {
	edgeEntries, tree := r.prollyEntries(snapshot.EdgeRoot)
	if !tree {
		return true // Flat roots are retained only for legacy repository compatibility.
	}
	outEntries, outTree := r.prollyEntries(snapshot.OutAdjRoot)
	inEntries, inTree := r.prollyEntries(snapshot.InAdjRoot)
	if !outTree || !inTree || len(edgeEntries) != len(outEntries) || len(edgeEntries) != len(inEntries) {
		return false
	}
	expectedOut := make([]prollyTreeEntry, 0, len(edgeEntries))
	expectedIn := make([]prollyTreeEntry, 0, len(edgeEntries))
	for _, entry := range edgeEntries {
		data, ok := r.Objects[entry.Value]
		if !ok {
			return false
		}
		var edge Edge
		if cbor.Unmarshal(data, &edge) != nil || edge.ID != entry.Key {
			return false
		}
		expectedOut = append(expectedOut, prollyTreeEntry{Key: adjacencyKey(edge.Source, edge.ID), Value: entry.Value})
		expectedIn = append(expectedIn, prollyTreeEntry{Key: adjacencyKey(edge.Target, edge.ID), Value: entry.Value})
	}
	return reflect.DeepEqual(sortedProllyEntries(expectedOut), outEntries) &&
		reflect.DeepEqual(sortedProllyEntries(expectedIn), inEntries)
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
		if err := syncMergeStateDirectory(r.mergeDirectory()); err != nil {
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
	return filepath.Join(r.mergeDirectory(), mergeStateFilename(targetBranch))
}

func mergeStateFilename(targetBranch string) string {
	sum := sha256.Sum256([]byte(targetBranch))
	return hex.EncodeToString(sum[:]) + ".json"
}

func persistedObjectID(objectType string, value any) ObjectID {
	encoded, err := canonicalObjectEncoding(value)
	if err != nil {
		panic(fmt.Sprintf("encode %s: %v", objectType, err))
	}
	return objectIDForEncoded(objectType, encoded)
}

func (r persistedRepository) validLegacyStoredObject(id ObjectID, objectType string, value any) bool {
	encoded, err := canonicalCBOR.Marshal(value)
	return err == nil && id == legacyPersistedObjectID(objectType, encoded) && bytes.Equal(r.Objects[id], encoded)
}

func legacyPersistedObjectID(objectType string, encoded []byte) ObjectID {
	return objectIDForEncoded(objectType, encoded)
}
