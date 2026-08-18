// Package repository provides durable graph storage and repository lifecycle operations.
package repository

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/gofrs/flock"
	"github.com/autonomous-bits/spool/internal/repository/branch"
	"lukechampine.com/blake3"
)

// SeedNodeID is the stable identifier of the node in every seeded repository.
const SeedNodeID = "11111111-1111-4111-8111-111111111111"
const defaultBranchName = "main"
const defaultCommitAuthor = "spl-local"

var (
	// ErrRepositoryNotInitialized reports an attempt to open repository state that does not exist.
	ErrRepositoryNotInitialized = errors.New("repository is not initialized")
	// ErrRepositoryAlreadyInitialized reports an attempt to initialize existing repository state.
	ErrRepositoryAlreadyInitialized = errors.New("repository is already initialized")
	// ErrBranchNotFound reports a requested branch that is absent from the repository.
	ErrBranchNotFound = errors.New("branch not found")
	// ErrCommitNotFound reports a requested commit that is absent from the repository.
	ErrCommitNotFound = errors.New("commit not found")
	// ErrCommitNotReachable reports a commit that is not reachable from its selected branch.
	ErrCommitNotReachable = errors.New("commit is not reachable from branch")
	// ErrNodeNotFound reports a node that is absent from the selected snapshot.
	ErrNodeNotFound = errors.New("node not found in snapshot")
	// ErrMissingMergePreviewBinding reports an incomplete merge preview binding.
	ErrMissingMergePreviewBinding = errors.New("merge preview binding is required")
	// ErrMissingMergeTransactionID reports an empty merge transaction identifier.
	ErrMissingMergeTransactionID = errors.New("merge transaction ID is required")
	// ErrStaleMergePreview reports a preview whose branches or merge base have changed.
	ErrStaleMergePreview = errors.New("merge preview binding is stale")
	// ErrMergeConflicted reports that a merge transaction was recorded for manual resolution.
	ErrMergeConflicted = errors.New("merge preview contains conflicts")
	// ErrMergeLeaseHeldByOther reports a merge target leased by another transaction.
	ErrMergeLeaseHeldByOther = errors.New("merge transaction lease is held by another transaction")
	// ErrMergeOperationNotOwner reports an operation attempted by a non-owning transaction.
	ErrMergeOperationNotOwner = errors.New("merge operation is not owned by this transaction")
	// ErrMergeNotInProgress reports an operation for a missing merge transaction.
	ErrMergeNotInProgress = errors.New("merge transaction is not in progress")
	// ErrMergeResolutionIncomplete reports finalization before resolution and restaging complete.
	ErrMergeResolutionIncomplete = errors.New("merge transaction resolution is incomplete")
	// ErrMergeStagedSnapshotMissing reports a resolution snapshot absent from repository state.
	ErrMergeStagedSnapshotMissing = errors.New("merge staged snapshot was not found")
	// ErrMergeTargetLeaseHeld reports an operation blocked by an active target merge transaction.
	ErrMergeTargetLeaseHeld = errors.New("merge target branch has an active transaction")
	// ErrMergeRepositoryLocked reports repository state locked by another process.
	ErrMergeRepositoryLocked = errors.New("merge repository is locked by another process")
	// ErrMergeRepositoryClosed reports use after Close.
	ErrMergeRepositoryClosed = errors.New("merge repository is closed")
	canonicalCBOR, _         = cbor.CanonicalEncOptions().EncMode()
)

// ObjectID is the content-derived identifier of a durable repository object.
type ObjectID string

// Node is the immutable node representation stored in a graph snapshot.
type Node struct {
	// ID uniquely identifies the node within a graph snapshot.
	ID string `json:"id"`
	// Title is the node's display value.
	Title string `json:"title"`
}

// Edge is the immutable edge representation stored in a graph snapshot.
type Edge struct {
	// ID uniquely identifies the edge within a graph snapshot.
	ID string `json:"id"`
	// Source identifies the edge's originating node.
	Source string `json:"source"`
	// Target identifies the edge's destination node.
	Target string `json:"target"`
}

// Resolution is an immutable view of a node resolved from a pinned commit.
type Resolution struct {
	// Node is the immutable node value read from the pinned commit.
	Node Node
	// Commit identifies the commit from which Node was resolved.
	Commit ObjectID
	// Snapshot identifies the graph snapshot containing Node.
	Snapshot ObjectID
	// NodeRoot identifies the durable root of the snapshot's node projection.
	NodeRoot ObjectID
}

type graphSnapshot struct {
	NodeRoot   ObjectID `cbor:"1,keyasint"`
	EdgeRoot   ObjectID `cbor:"2,keyasint"`
	OutAdjRoot ObjectID `cbor:"3,keyasint"`
	InAdjRoot  ObjectID `cbor:"4,keyasint"`
	SchemaRoot ObjectID `cbor:"5,keyasint"`
}

type commit struct {
	Snapshot ObjectID   `cbor:"1,keyasint"`
	Parents  []ObjectID `cbor:"2,keyasint"`
	Message  string     `cbor:"3,keyasint"`
	Author   string     `cbor:"4,keyasint"`
	Time     time.Time  `cbor:"5,keyasint"`
}

// Repository provides concurrency-safe access to durable graph, branch, and merge state.
type Repository struct {
	mu                  sync.RWMutex
	defaultBranch       string
	activeBranch        string
	branches            map[string]ObjectID
	commits             map[ObjectID]commit
	snapshots           map[ObjectID]graphSnapshot
	projections         map[ObjectID]map[string]Node
	edgeProjections     map[ObjectID]map[string]Edge
	objects             map[ObjectID][]byte
	stagedMutations     map[string]StagedMutationSet
	mergeLeases         map[string]string
	mergeTransactions   map[string]mergeTransaction
	mergeStateDir       string
	persistStateFn      func(string, string, *mergeTransaction) error
	persistRepositoryFn func() error
	stateLock           *flock.Flock
	closed              bool
	now                 func() time.Time
}

// Initialization identifies the repository's default and currently active branches.
type Initialization struct {
	// DefaultBranch is the branch that cannot be deleted.
	DefaultBranch string `json:"defaultBranch"`
	// ActiveBranch is the branch currently selected for repository operations.
	ActiveBranch string `json:"activeBranch"`
}

// NewSeedRepository returns an in-memory repository initialized with the seed graph.
func NewSeedRepository() *Repository {
	repo := &Repository{
		defaultBranch:     defaultBranchName,
		activeBranch:      defaultBranchName,
		branches:          make(map[string]ObjectID),
		commits:           make(map[ObjectID]commit),
		snapshots:         make(map[ObjectID]graphSnapshot),
		projections:       make(map[ObjectID]map[string]Node),
		edgeProjections:   make(map[ObjectID]map[string]Edge),
		objects:           make(map[ObjectID][]byte),
		stagedMutations:   make(map[string]StagedMutationSet),
		mergeLeases:       make(map[string]string),
		mergeTransactions: make(map[string]mergeTransaction),
		now:               time.Now,
	}

	node := Node{ID: SeedNodeID, Title: "EDG walking skeleton"}
	nodeID := repo.store("node", node)
	nodeRoot := repo.store("prolly-node-root", []ObjectID{nodeID})
	edgeRoot := repo.store("prolly-edge-root", []ObjectID{})
	outAdjRoot := repo.store("prolly-out-adjacency-root", []ObjectID{})
	inAdjRoot := repo.store("prolly-in-adjacency-root", []ObjectID{})
	schemaRoot := repo.store("schema-root", map[string]string{"version": "v1"})
	snapshot := graphSnapshot{
		NodeRoot: nodeRoot, EdgeRoot: edgeRoot, OutAdjRoot: outAdjRoot,
		InAdjRoot: inAdjRoot, SchemaRoot: schemaRoot,
	}
	snapshotID := repo.store("graph-snapshot", snapshot)
	repo.snapshots[snapshotID] = snapshot
	repo.projections[nodeRoot] = map[string]Node{node.ID: node}
	repo.edgeProjections[snapshotID] = map[string]Edge{}

	seedCommit := commit{Snapshot: snapshotID, Message: "seed resolve snapshot", Author: defaultCommitAuthor, Time: time.Unix(0, 0).UTC()}
	commitID := repo.store("commit", seedCommit)
	repo.commits[commitID] = seedCommit
	repo.branches[defaultBranchName] = commitID
	return repo
}

func (r *Repository) newCommit(snapshot ObjectID, parents []ObjectID, author, message string) commit {
	if author == "" {
		author = defaultCommitAuthor
	}
	if message == "" {
		message = "commit staged mutations"
	}
	return commit{
		Snapshot: snapshot,
		Parents:  append([]ObjectID(nil), parents...),
		Message:  message,
		Author:   author,
		Time:     r.now().UTC(),
	}
}

// Initialization returns the current default and active branches, or an error if closed.
func (r *Repository) Initialization() (Initialization, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.ensureOpenLocked(); err != nil {
		return Initialization{}, err
	}
	return Initialization{DefaultBranch: r.defaultBranch, ActiveBranch: r.activeBranch}, nil
}

func (r *Repository) ensureOpenLocked() error {
	if r.closed {
		return ErrMergeRepositoryClosed
	}
	return nil
}

func (r *Repository) store(objectType string, value any) ObjectID {
	encoded, err := canonicalCBOR.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("encode %s: %v", objectType, err))
	}
	header := objectType + " " + strconv.Itoa(len(encoded)) + "\x00"
	sum := blake3.Sum256(append([]byte(header), encoded...))
	id := ObjectID(hex.EncodeToString(sum[:]))
	r.objects[id] = encoded
	return id
}

func (r *Repository) objectID(objectType string, value any) ObjectID {
	return persistedObjectID(objectType, value)
}

// CreateBranch atomically creates name at source and persists the new branch when durable.
func (r *Repository) CreateBranch(name string, source branch.Source) (branch.CreateResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return branch.CreateResult{}, err
	}

	sourceCommit, err := r.resolveBranchSourceLocked(source)
	if err != nil {
		return branch.CreateResult{}, err
	}
	if _, exists := r.branches[name]; exists {
		return branch.CreateResult{}, branch.ErrAlreadyExists
	}

	r.branches[name] = sourceCommit
	if err := r.persistRepositoryLocked(); err != nil {
		if durableWriteCommitted(err) {
			return branch.CreateResult{Name: name, Commit: string(sourceCommit)}, fmt.Errorf("branch creation committed but directory sync failed: %w", err)
		}
		delete(r.branches, name)
		return branch.CreateResult{}, err
	}
	return branch.CreateResult{Name: name, Commit: string(sourceCommit)}, nil
}

// ListBranches returns lexically ordered branch names, or an error if the repository is closed.
func (r *Repository) ListBranches() (branch.ListResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.ensureOpenLocked(); err != nil {
		return branch.ListResult{}, err
	}

	branches := make([]string, 0, len(r.branches))
	for name := range r.branches {
		branches = append(branches, name)
	}
	sort.Strings(branches)
	return branch.ListResult{Branches: branches}, nil
}

// DeleteBranch atomically deletes a non-default, inactive branch and its staged mutations.
func (r *Repository) DeleteBranch(name string) (branch.DeleteResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return branch.DeleteResult{}, err
	}
	if name == defaultBranchName {
		return branch.DeleteResult{}, branch.ErrDefaultProtected
	}
	if name == r.activeBranch {
		return branch.DeleteResult{}, branch.ErrActiveProtected
	}

	commitID, exists := r.branches[name]
	if !exists {
		return branch.DeleteResult{}, branch.ErrNotFound
	}
	staged, hadStaged := r.stagedMutations[name]
	delete(r.branches, name)
	delete(r.stagedMutations, name)
	if err := r.persistRepositoryLocked(); err != nil {
		if durableWriteCommitted(err) {
			return branch.DeleteResult{Name: name}, fmt.Errorf("branch deletion committed but directory sync failed: %w", err)
		}
		r.branches[name] = commitID
		if hadStaged {
			r.stagedMutations[name] = staged
		}
		return branch.DeleteResult{}, err
	}
	return branch.DeleteResult{Name: name}, nil
}

// SwitchBranch atomically makes an existing branch active and persists that selection.
func (r *Repository) SwitchBranch(name string) (branch.SwitchResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return branch.SwitchResult{}, err
	}
	if _, exists := r.branches[name]; !exists {
		return branch.SwitchResult{}, branch.ErrNotFound
	}
	if name == r.activeBranch {
		return branch.SwitchResult{ActiveBranch: name}, nil
	}

	previousActiveBranch := r.activeBranch
	r.activeBranch = name
	if err := r.persistRepositoryLocked(); err != nil {
		if durableWriteCommitted(err) {
			return branch.SwitchResult{ActiveBranch: name}, fmt.Errorf("branch switch committed but directory sync failed: %w", err)
		}
		r.activeBranch = previousActiveBranch
		return branch.SwitchResult{}, err
	}
	return branch.SwitchResult{ActiveBranch: name}, nil
}

func (r *Repository) resolveBranchSourceLocked(source branch.Source) (ObjectID, error) {
	if err := branch.ValidateSource(source); err != nil {
		return "", err
	}
	if source.Branch != "" {
		commitID, ok := r.branches[source.Branch]
		if !ok {
			return "", branch.ErrSourceNotFound
		}
		return commitID, nil
	}
	commitID := ObjectID(source.Commit)
	if _, ok := r.commits[commitID]; !ok {
		return "", branch.ErrSourceNotFound
	}
	return commitID, nil
}

// PinBranch returns the current immutable commit for a branch. The returned
// ID remains valid if the branch moves after it has been pinned.
func (r *Repository) PinBranch(name string) (ObjectID, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.ensureOpenLocked(); err != nil {
		return "", err
	}
	commitID, ok := r.branches[name]
	if !ok {
		return "", ErrBranchNotFound
	}
	return commitID, nil
}

// ResolveExplicitCommit validates an explicit commit selector against a branch.
func (r *Repository) ResolveExplicitCommit(branch string, requested ObjectID, allowDetached bool) (ObjectID, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.ensureOpenLocked(); err != nil {
		return "", err
	}
	head, exists := r.branches[branch]
	if !exists {
		return "", ErrBranchNotFound
	}
	if _, exists := r.commits[requested]; !exists {
		return "", ErrCommitNotFound
	}
	if allowDetached {
		return requested, nil
	}

	seen := map[ObjectID]struct{}{head: {}}
	queue := []ObjectID{head}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == requested {
			return requested, nil
		}
		for _, parent := range r.commits[current].Parents {
			if _, seen := seen[parent]; seen {
				continue
			}
			seen[parent] = struct{}{}
			queue = append(queue, parent)
		}
	}
	return "", ErrCommitNotReachable
}

// ResolvePinned reads a node from a previously pinned commit.
func (r *Repository) ResolvePinned(commitID ObjectID, nodeID string) (Resolution, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.ensureOpenLocked(); err != nil {
		return Resolution{}, err
	}
	commit, ok := r.commits[commitID]
	if !ok {
		return Resolution{}, ErrBranchNotFound
	}
	snapshot := r.snapshots[commit.Snapshot]
	node, ok := r.projections[snapshot.NodeRoot][nodeID]
	if !ok {
		return Resolution{}, ErrNodeNotFound
	}
	return Resolution{
		Node:     node,
		Commit:   commitID,
		Snapshot: commit.Snapshot,
		NodeRoot: snapshot.NodeRoot,
	}, nil
}
