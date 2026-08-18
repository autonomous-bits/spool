package repository

import "fmt"

// MergePreviewBinding pins the commits and merge base inspected by a merge preview.
type MergePreviewBinding struct {
	// MergeBase is the common ancestor used to produce the preview.
	MergeBase ObjectID
	// SourceCommit is the source branch head inspected by the preview.
	SourceCommit ObjectID
	// TargetCommit is the target branch head inspected by the preview.
	TargetCommit ObjectID
}

type mergeTransaction struct {
	OwnerTransactionID string
	SourceBranch       string
	TargetBranch       string
	Binding            MergePreviewBinding
	OriginalTarget     ObjectID
	StagedSnapshot     ObjectID
	Resolved           bool
	Restaged           bool
}

// AdvanceBranch creates and persists a no-content commit on branch unless it is merge leased.
func (r *Repository) AdvanceBranch(branch string) (ObjectID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return "", err
	}
	if _, held := r.mergeLeases[branch]; held {
		return "", ErrMergeTargetLeaseHeld
	}
	current, ok := r.branches[branch]
	if !ok {
		return "", ErrBranchNotFound
	}
	previous := r.commits[current]
	next := r.newCommit(previous.Snapshot, []ObjectID{current}, "", "advance branch for a subsequent request")
	previousObject, objectExisted := r.objects[r.objectID("commit", next)]
	nextID := r.store("commit", next)
	previousCommit, commitExisted := r.commits[nextID]
	r.commits[nextID] = next
	r.branches[branch] = nextID
	if err := r.persistRepositoryLocked(); err != nil {
		if durableWriteCommitted(err) {
			return nextID, fmt.Errorf("branch advance committed but directory sync failed: %w", err)
		}
		if commitExisted {
			r.commits[nextID] = previousCommit
		} else {
			delete(r.commits, nextID)
		}
		r.branches[branch] = current
		if objectExisted {
			r.objects[nextID] = previousObject
		} else {
			delete(r.objects, nextID)
		}
		return "", err
	}
	return nextID, nil
}

// ApplyCleanBoundMerge validates binding and atomically commits a clean merge to targetBranch.
func (r *Repository) ApplyCleanBoundMerge(sourceBranch, targetBranch, transactionID string, binding MergePreviewBinding) (ObjectID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return "", err
	}
	if err := r.validateMergePreviewBindingLocked(sourceBranch, targetBranch, binding); err != nil {
		return "", err
	}
	if transaction, active := r.mergeTransactions[targetBranch]; active && transaction.OwnerTransactionID == transactionID {
		return "", ErrMergeNotInProgress
	}
	if err := r.acquireMergeLeaseLocked(targetBranch, transactionID); err != nil {
		return "", err
	}
	defer delete(r.mergeLeases, targetBranch)
	sourceCommit := r.branches[sourceBranch]
	targetCommit := r.branches[targetBranch]
	merged := r.newCommit(r.commits[targetCommit].Snapshot, []ObjectID{targetCommit, sourceCommit}, "", "apply clean bound merge")
	previousObject, objectExisted := r.objects[r.objectID("commit", merged)]
	mergedID := r.store("commit", merged)
	previousCommit, commitExisted := r.commits[mergedID]
	r.commits[mergedID] = merged
	r.branches[targetBranch] = mergedID
	if err := r.persistRepositoryLocked(); err != nil {
		if durableWriteCommitted(err) {
			return mergedID, fmt.Errorf("clean merge committed but directory sync failed: %w", err)
		}
		if commitExisted {
			r.commits[mergedID] = previousCommit
		} else {
			delete(r.commits, mergedID)
		}
		r.branches[targetBranch] = targetCommit
		if objectExisted {
			r.objects[mergedID] = previousObject
		} else {
			delete(r.objects, mergedID)
		}
		return "", err
	}
	return mergedID, nil
}

// ApplyConflictedBoundMerge persists a target lease and transaction, then returns ErrMergeConflicted.
func (r *Repository) ApplyConflictedBoundMerge(sourceBranch, targetBranch, transactionID string, binding MergePreviewBinding) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return err
	}
	if err := r.validateMergePreviewBindingLocked(sourceBranch, targetBranch, binding); err != nil {
		return err
	}
	if transactionID == "" {
		return ErrMissingMergeTransactionID
	}
	if holder, held := r.mergeLeases[targetBranch]; held && holder != transactionID {
		return ErrMergeLeaseHeldByOther
	}
	if _, active := r.mergeTransactions[targetBranch]; !active {
		transaction := mergeTransaction{OwnerTransactionID: transactionID, SourceBranch: sourceBranch, TargetBranch: targetBranch, Binding: binding, OriginalTarget: r.branches[targetBranch]}
		if err := r.persistRepositoryLocked(); err != nil {
			return err
		}
		persistErr := r.persistMergeTransactionLocked(targetBranch, transactionID, &transaction)
		if persistErr != nil && !durableWriteCommitted(persistErr) {
			return persistErr
		}
		r.mergeLeases[targetBranch] = transactionID
		r.mergeTransactions[targetBranch] = transaction
		if persistErr != nil {
			return fmt.Errorf("merge transaction recorded but directory sync failed: %w", persistErr)
		}
	}
	return ErrMergeConflicted
}

func (r *Repository) acquireMergeLeaseLocked(targetBranch, transactionID string) error {
	if transactionID == "" {
		return ErrMissingMergeTransactionID
	}
	if holder, held := r.mergeLeases[targetBranch]; held && holder != transactionID {
		return ErrMergeLeaseHeldByOther
	}
	r.mergeLeases[targetBranch] = transactionID
	return nil
}

// ResolveMergeTransaction records an existing resolution snapshot for the owning transaction.
func (r *Repository) ResolveMergeTransaction(targetBranch, callerTransactionID string, stagedSnapshot ObjectID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return err
	}
	transaction, err := r.ownedMergeTransactionLocked(targetBranch, callerTransactionID)
	if err != nil {
		return err
	}
	if _, ok := r.snapshots[stagedSnapshot]; !ok {
		return ErrMergeStagedSnapshotMissing
	}
	transaction.StagedSnapshot, transaction.Resolved, transaction.Restaged = stagedSnapshot, true, false
	persistErr := r.persistMergeTransactionLocked(targetBranch, callerTransactionID, &transaction)
	if persistErr != nil && !durableWriteCommitted(persistErr) {
		return persistErr
	}
	r.mergeTransactions[targetBranch] = transaction
	if persistErr != nil {
		return fmt.Errorf("merge resolution recorded but directory sync failed: %w", persistErr)
	}
	return nil
}

// RestageMergeTransaction records that the owning transaction's resolution was restaged.
func (r *Repository) RestageMergeTransaction(targetBranch, callerTransactionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return err
	}
	transaction, err := r.ownedMergeTransactionLocked(targetBranch, callerTransactionID)
	if err != nil {
		return err
	}
	if !transaction.Resolved {
		return ErrMergeResolutionIncomplete
	}
	transaction.Restaged = true
	persistErr := r.persistMergeTransactionLocked(targetBranch, callerTransactionID, &transaction)
	if persistErr != nil && !durableWriteCommitted(persistErr) {
		return persistErr
	}
	r.mergeTransactions[targetBranch] = transaction
	if persistErr != nil {
		return fmt.Errorf("merge restage recorded but directory sync failed: %w", persistErr)
	}
	return nil
}

// FinalizeMergeTransaction atomically commits a resolved, restaged transaction and releases its lease.
func (r *Repository) FinalizeMergeTransaction(targetBranch, callerTransactionID string) (ObjectID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return "", err
	}
	transaction, err := r.ownedMergeTransactionLocked(targetBranch, callerTransactionID)
	if err != nil {
		return "", err
	}
	if !transaction.Resolved || !transaction.Restaged {
		return "", ErrMergeResolutionIncomplete
	}
	if err := r.validateMergePreviewBindingLocked(transaction.SourceBranch, targetBranch, transaction.Binding); err != nil {
		return "", err
	}
	if r.branches[targetBranch] != transaction.OriginalTarget {
		return "", ErrStaleMergePreview
	}
	merged := r.newCommit(transaction.StagedSnapshot, []ObjectID{transaction.OriginalTarget, transaction.Binding.SourceCommit}, "", "finalize resolved merge")
	previousObject, objectExisted := r.objects[r.objectID("commit", merged)]
	mergedID := r.store("commit", merged)
	previousCommit, commitExisted := r.commits[mergedID]
	r.commits[mergedID], r.branches[targetBranch] = merged, mergedID
	persistErr := r.persistRepositoryLocked()
	if persistErr != nil && !durableWriteCommitted(persistErr) {
		if commitExisted {
			r.commits[mergedID] = previousCommit
		} else {
			delete(r.commits, mergedID)
		}
		r.branches[targetBranch] = transaction.OriginalTarget
		if objectExisted {
			r.objects[mergedID] = previousObject
		} else {
			delete(r.objects, mergedID)
		}
		return "", persistErr
	}
	cleanupErr := r.persistMergeTransactionLocked(targetBranch, "", nil)
	if cleanupErr == nil || durableWriteCommitted(cleanupErr) {
		delete(r.mergeTransactions, targetBranch)
		delete(r.mergeLeases, targetBranch)
	}
	if cleanupErr != nil {
		return mergedID, fmt.Errorf("merge committed but transaction cleanup failed: %w", cleanupErr)
	}
	if persistErr != nil {
		return mergedID, fmt.Errorf("merge committed but directory sync failed: %w", persistErr)
	}
	return mergedID, nil
}

// AbortMergeTransaction durably removes an owning transaction and releases its target lease.
func (r *Repository) AbortMergeTransaction(targetBranch, callerTransactionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return err
	}
	if _, err := r.ownedMergeTransactionLocked(targetBranch, callerTransactionID); err != nil {
		return err
	}
	persistErr := r.persistMergeTransactionLocked(targetBranch, "", nil)
	if persistErr != nil && !durableWriteCommitted(persistErr) {
		return persistErr
	}
	delete(r.mergeTransactions, targetBranch)
	delete(r.mergeLeases, targetBranch)
	if persistErr != nil {
		return fmt.Errorf("merge transaction removal committed but directory sync failed: %w", persistErr)
	}
	return nil
}

func (r *Repository) ownedMergeTransactionLocked(targetBranch, callerTransactionID string) (mergeTransaction, error) {
	if callerTransactionID == "" {
		return mergeTransaction{}, ErrMissingMergeTransactionID
	}
	transaction, active := r.mergeTransactions[targetBranch]
	if !active || r.mergeLeases[targetBranch] == "" {
		return mergeTransaction{}, ErrMergeNotInProgress
	}
	if transaction.OwnerTransactionID != callerTransactionID || r.mergeLeases[targetBranch] != callerTransactionID {
		return mergeTransaction{}, ErrMergeOperationNotOwner
	}
	return transaction, nil
}

func (r *Repository) validateMergePreviewBindingLocked(sourceBranch, targetBranch string, binding MergePreviewBinding) error {
	if binding.MergeBase == "" || binding.SourceCommit == "" || binding.TargetCommit == "" {
		return ErrMissingMergePreviewBinding
	}
	sourceCommit, ok := r.branches[sourceBranch]
	if !ok {
		return ErrBranchNotFound
	}
	targetCommit, ok := r.branches[targetBranch]
	if !ok {
		return ErrBranchNotFound
	}
	if sourceCommit != binding.SourceCommit || targetCommit != binding.TargetCommit {
		return ErrStaleMergePreview
	}
	mergeBase, ok := r.mergeBaseLocked(sourceCommit, targetCommit)
	if !ok || mergeBase != binding.MergeBase {
		return ErrStaleMergePreview
	}
	return nil
}

func (r *Repository) mergeBaseLocked(left, right ObjectID) (ObjectID, bool) {
	leftDistances, rightDistances := r.ancestorDistancesLocked(left), r.ancestorDistancesLocked(right)
	var base ObjectID
	bestDistance, found := 0, false
	for ancestor, leftDistance := range leftDistances {
		rightDistance, ok := rightDistances[ancestor]
		if !ok {
			continue
		}
		distance := leftDistance + rightDistance
		if !found || distance < bestDistance || (distance == bestDistance && ancestor < base) {
			base, bestDistance, found = ancestor, distance, true
		}
	}
	return base, found
}

func (r *Repository) ancestorDistancesLocked(start ObjectID) map[ObjectID]int {
	distances, queue := map[ObjectID]int{start: 0}, []ObjectID{start}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, parent := range r.commits[current].Parents {
			if _, seen := distances[parent]; seen {
				continue
			}
			distances[parent] = distances[current] + 1
			queue = append(queue, parent)
		}
	}
	return distances
}
