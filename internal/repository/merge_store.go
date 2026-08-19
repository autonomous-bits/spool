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
	Preview            MergePreview
	OriginalTarget     ObjectID
	StagedSnapshot     ObjectID
	Resolved           bool
	Restaged           bool
}

// MergeTransactionStatus is the owner-gated public view of a conflicted merge.
type MergeTransactionStatus struct {
	Preview  MergePreview `json:"preview"`
	Resolved bool         `json:"resolved"`
	Restaged bool         `json:"restaged"`
}

// MergeResolutionSelection selects one side for a reported structural or schema conflict.
type MergeResolutionSelection struct {
	ConflictID string `json:"conflictId"`
	Choice     string `json:"choice"`
}

// ResolveConflictedMergeRequest supplies every conflict selection and optional corrective mutations.
type ResolveConflictedMergeRequest struct {
	TargetBranch  string                     `json:"targetBranch"`
	TransactionID string                     `json:"transactionId"`
	PreviewID     ObjectID                   `json:"previewId"`
	Selections    []MergeResolutionSelection `json:"selections"`
	Overrides     []MutationOperation        `json:"overrides,omitempty"`
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
	nextID, err := r.storeObject("commit", next)
	if err != nil {
		return "", fmt.Errorf("store branch advance commit: %w", err)
	}
	previousCommit, commitExisted := r.commits[nextID]
	r.commits[nextID] = next
	r.branches[branch] = nextID
	if err := r.writeRefLocked(branch, current, nextID, "advance"); err != nil {
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
	if err := r.validateSnapshotSchemaLocked(r.commits[r.branches[targetBranch]].Snapshot); err != nil {
		return "", err
	}
	candidate, err := r.previewMergeLocked(sourceBranch, targetBranch)
	if err != nil {
		return "", err
	}
	if !candidate.preview.Clean {
		return "", ErrMergePreviewNotClean
	}
	return r.applyCleanCandidateLocked(candidate, transactionID, "", "apply clean bound merge")
}

// ApplyMergePreview recomputes and atomically applies the exact clean preview identified by previewID.
func (r *Repository) ApplyMergePreview(sourceBranch, targetBranch, transactionID string, previewID ObjectID, author, message string) (ObjectID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return "", err
	}
	if _, active := r.mergeTransactions[targetBranch]; active {
		return "", ErrMergeTargetLeaseHeld
	}
	candidate, err := r.previewMergeLocked(sourceBranch, targetBranch)
	if err != nil {
		return "", err
	}
	if candidate.preview.ID != previewID {
		return "", ErrMergePreviewMismatch
	}
	if !candidate.preview.Clean {
		if err := r.applyConflictedPreviewLocked(candidate.preview, transactionID); err != nil {
			return "", err
		}
		return "", ErrMergeConflicted
	}
	return r.applyCleanCandidateLocked(candidate, transactionID, author, message)
}

func (r *Repository) applyCleanCandidateLocked(candidate mergeCandidate, transactionID, author, message string) (ObjectID, error) {
	targetCommit := candidate.preview.Binding.TargetCommit
	if err := r.acquireMergeLeaseLocked(candidate.preview.TargetBranch, transactionID); err != nil {
		return "", err
	}
	defer delete(r.mergeLeases, candidate.preview.TargetBranch)

	objects, snapshots, projections, edgeProjections := r.objects, r.snapshots, r.projections, r.edgeProjections
	commits, branches := r.commits, r.branches
	r.objects, r.snapshots = cloneObjects(r.objects), cloneSnapshots(r.snapshots)
	r.projections, r.edgeProjections = cloneProjectionMap(r.projections), cloneEdgeProjectionMap(r.edgeProjections)
	r.commits, r.branches = cloneCommits(r.commits), cloneBranches(r.branches)

	snapshot, err := r.materializeSnapshotLocked(candidate.nodes, candidate.edges, candidate.schemaRoot)
	if err != nil {
		r.objects, r.snapshots, r.projections, r.edgeProjections = objects, snapshots, projections, edgeProjections
		r.commits, r.branches = commits, branches
		return "", fmt.Errorf("materialize merge result: %w", err)
	}
	snapshotID, err := r.storeObject("graph-snapshot", snapshot)
	if err != nil {
		r.objects, r.snapshots, r.projections, r.edgeProjections = objects, snapshots, projections, edgeProjections
		r.commits, r.branches = commits, branches
		return "", fmt.Errorf("store merge snapshot: %w", err)
	}
	r.snapshots[snapshotID] = snapshot
	if err := r.reconstructSnapshotProjectionsLocked(snapshotID, snapshot); err != nil {
		r.objects, r.snapshots, r.projections, r.edgeProjections = objects, snapshots, projections, edgeProjections
		r.commits, r.branches = commits, branches
		return "", fmt.Errorf("reconstruct merge result: %w", err)
	}
	merged := r.newCommit(snapshotID, []ObjectID{targetCommit, candidate.preview.Binding.SourceCommit}, author, message)
	mergedID, err := r.storeObject("commit", merged)
	if err != nil {
		r.objects, r.snapshots, r.projections, r.edgeProjections = objects, snapshots, projections, edgeProjections
		r.commits, r.branches = commits, branches
		return "", fmt.Errorf("store merge commit: %w", err)
	}
	r.commits[mergedID], r.branches[candidate.preview.TargetBranch] = merged, mergedID
	if err := r.writeRefLocked(candidate.preview.TargetBranch, targetCommit, mergedID, "merge"); err != nil {
		if durableWriteCommitted(err) {
			return mergedID, fmt.Errorf("clean merge committed but directory sync failed: %w", err)
		}
		r.objects, r.snapshots, r.projections, r.edgeProjections = objects, snapshots, projections, edgeProjections
		r.commits, r.branches = commits, branches
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
	candidate, err := r.previewMergeLocked(sourceBranch, targetBranch)
	if err != nil {
		return err
	}
	return r.applyConflictedPreviewLocked(candidate.preview, transactionID)
}

func (r *Repository) applyConflictedPreviewLocked(preview MergePreview, transactionID string) error {
	targetBranch := preview.TargetBranch
	if transactionID == "" {
		return ErrMissingMergeTransactionID
	}
	if holder, held := r.mergeLeases[targetBranch]; held && holder != transactionID {
		return ErrMergeLeaseHeldByOther
	}
	if _, active := r.mergeTransactions[targetBranch]; !active {
		transaction := mergeTransaction{
			OwnerTransactionID: transactionID, SourceBranch: preview.SourceBranch, TargetBranch: targetBranch,
			Binding: preview.Binding, Preview: preview, OriginalTarget: r.branches[targetBranch],
		}
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

// InspectMergeTransaction returns the persisted preview and resolution state to its owner.
func (r *Repository) InspectMergeTransaction(targetBranch, callerTransactionID string) (MergeTransactionStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.ensureOpenLocked(); err != nil {
		return MergeTransactionStatus{}, err
	}
	transaction, err := r.ownedMergeTransactionLocked(targetBranch, callerTransactionID)
	if err != nil {
		return MergeTransactionStatus{}, err
	}
	return MergeTransactionStatus{Preview: transaction.Preview, Resolved: transaction.Resolved, Restaged: transaction.Restaged}, nil
}

// ResolveConflictedMerge materializes an owner-selected, schema-valid resolution snapshot.
func (r *Repository) ResolveConflictedMerge(request ResolveConflictedMergeRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return err
	}
	transaction, err := r.ownedMergeTransactionLocked(request.TargetBranch, request.TransactionID)
	if err != nil {
		return err
	}
	if transaction.Preview.ID != request.PreviewID {
		return ErrMergeResolutionPreviewMismatch
	}
	if err := r.validateMergePreviewBindingLocked(transaction.SourceBranch, request.TargetBranch, transaction.Binding); err != nil {
		return err
	}
	candidate, err := r.previewMergeLocked(transaction.SourceBranch, request.TargetBranch)
	if err != nil {
		return err
	}
	if candidate.preview.ID != transaction.Preview.ID {
		return ErrStaleMergePreview
	}
	choices, err := mergeResolutionChoices(transaction.Preview.Conflicts, request.Selections)
	if err != nil {
		return err
	}
	sourceSnapshot := r.snapshots[r.commits[transaction.Binding.SourceCommit].Snapshot]
	sourceNodes := r.projections[sourceSnapshot.NodeRoot]
	sourceEdges := r.edgeProjections[r.commits[transaction.Binding.SourceCommit].Snapshot]
	for _, conflict := range transaction.Preview.Conflicts {
		if choices[conflict.ConflictID] == "source" {
			applySourceResolution(conflict, candidate.nodes, candidate.edges, sourceNodes, sourceEdges, &candidate.schemaRoot, sourceSnapshot.SchemaRoot)
		}
	}
	overrides, err := normalizeMutationOperations(request.Overrides)
	if err != nil {
		return err
	}
	if err := validateResolutionOverrides(candidate.nodes, candidate.edges, overrides); err != nil {
		return err
	}
	applyMutationOperations(candidate.nodes, candidate.edges, overrides)
	if err := validateCandidateValues(candidate.nodes, candidate.edges); err != nil {
		return err
	}
	schema, err := r.schemaSnapshotLocked(candidate.schemaRoot)
	if err != nil {
		return err
	}
	if err := ValidateSchemaSnapshot(schema, candidate.nodes, candidate.edges); err != nil {
		return err
	}

	objects, snapshots, projections, edgeProjections := r.objects, r.snapshots, r.projections, r.edgeProjections
	r.objects, r.snapshots = cloneObjects(r.objects), cloneSnapshots(r.snapshots)
	r.projections, r.edgeProjections = cloneProjectionMap(r.projections), cloneEdgeProjectionMap(r.edgeProjections)
	snapshot, err := r.materializeSnapshotLocked(candidate.nodes, candidate.edges, candidate.schemaRoot)
	if err != nil {
		r.objects, r.snapshots, r.projections, r.edgeProjections = objects, snapshots, projections, edgeProjections
		return fmt.Errorf("materialize resolved merge: %w", err)
	}
	snapshotID, err := r.storeObject("graph-snapshot", snapshot)
	if err != nil {
		r.objects, r.snapshots, r.projections, r.edgeProjections = objects, snapshots, projections, edgeProjections
		return fmt.Errorf("store resolved merge snapshot: %w", err)
	}
	r.snapshots[snapshotID] = snapshot
	if err := r.reconstructSnapshotProjectionsLocked(snapshotID, snapshot); err != nil {
		r.objects, r.snapshots, r.projections, r.edgeProjections = objects, snapshots, projections, edgeProjections
		return fmt.Errorf("reconstruct resolved merge: %w", err)
	}
	transaction.StagedSnapshot, transaction.Resolved, transaction.Restaged = snapshotID, true, true
	transactionErr := r.persistMergeTransactionLocked(request.TargetBranch, request.TransactionID, &transaction)
	if transactionErr != nil && !durableWriteCommitted(transactionErr) {
		r.objects, r.snapshots, r.projections, r.edgeProjections = objects, snapshots, projections, edgeProjections
		return transactionErr
	}
	r.mergeTransactions[request.TargetBranch] = transaction
	if transactionErr != nil {
		return fmt.Errorf("merge resolution recorded but directory sync failed: %w", transactionErr)
	}
	return nil
}

func mergeResolutionChoices(conflicts []MergeConflict, selections []MergeResolutionSelection) (map[string]string, error) {
	if len(conflicts) != len(selections) {
		return nil, ErrMergeResolutionSelection
	}
	known := make(map[string]struct{}, len(conflicts))
	for _, conflict := range conflicts {
		known[conflict.ConflictID] = struct{}{}
	}
	result := make(map[string]string, len(selections))
	for _, selection := range selections {
		if _, exists := known[selection.ConflictID]; !exists || (selection.Choice != "source" && selection.Choice != "target") {
			return nil, ErrMergeResolutionSelection
		}
		if _, duplicate := result[selection.ConflictID]; duplicate {
			return nil, ErrMergeResolutionSelection
		}
		result[selection.ConflictID] = selection.Choice
	}
	return result, nil
}

func applySourceResolution(conflict MergeConflict, nodes map[string]Node, edges map[string]Edge, sourceNodes map[string]Node, sourceEdges map[string]Edge, schemaRoot *ObjectID, sourceSchemaRoot ObjectID) {
	if conflict.Entity == "schema" {
		*schemaRoot = sourceSchemaRoot
		return
	}
	if conflict.Entity == "node" {
		source, exists := sourceNodes[conflict.ID]
		if conflict.Field == "existence" {
			if exists {
				nodes[conflict.ID] = source.clone()
			} else {
				delete(nodes, conflict.ID)
			}
			return
		}
		node := nodes[conflict.ID]
		switch conflict.Field {
		case "title":
			node.Title = source.Title
		case "labels":
			node.Labels = append([]string(nil), source.Labels...)
		default:
			if len(conflict.Field) > len("properties.") {
				key := conflict.Field[len("properties."):]
				if source.Properties == nil {
					delete(node.Properties, key)
				} else if value, ok := source.Properties[key]; ok {
					if node.Properties == nil {
						node.Properties = make(map[string]PropertyValue)
					}
					node.Properties[key] = value.clone()
				} else {
					delete(node.Properties, key)
				}
			}
		}
		nodes[conflict.ID] = node
		return
	}
	if conflict.Entity == "edge" {
		source, exists := sourceEdges[conflict.ID]
		if conflict.Field == "existence" {
			if exists {
				edges[conflict.ID] = source.clone()
			} else {
				delete(edges, conflict.ID)
			}
			return
		}
		edge := edges[conflict.ID]
		switch conflict.Field {
		case "source":
			edge.Source = source.Source
		case "target":
			edge.Target = source.Target
		case "type":
			edge.Type = source.Type
		default:
			if len(conflict.Field) > len("properties.") {
				key := conflict.Field[len("properties."):]
				if source.Properties == nil {
					delete(edge.Properties, key)
				} else if value, ok := source.Properties[key]; ok {
					if edge.Properties == nil {
						edge.Properties = make(map[string]PropertyValue)
					}
					edge.Properties[key] = value.clone()
				} else {
					delete(edge.Properties, key)
				}
			}
		}
		edges[conflict.ID] = edge
	}
}

func validateResolutionOverrides(nodes map[string]Node, edges map[string]Edge, operations []MutationOperation) error {
	seen := make(map[string]struct{}, len(operations))
	for _, operation := range operations {
		if operation.ID == "" || (operation.Entity != "node" && operation.Entity != "edge") ||
			(operation.Action != "add" && operation.Action != "update" && operation.Action != "delete") {
			return ErrInvalidMutationBatch
		}
		key := operation.Entity + ":" + operation.ID
		if _, duplicate := seen[key]; duplicate {
			return ErrInvalidMutationBatch
		}
		seen[key] = struct{}{}
		if operation.Entity == "node" {
			_, exists := nodes[operation.ID]
			if (operation.Action == "add" && exists) || ((operation.Action == "update" || operation.Action == "delete") && !exists) {
				return ErrInvalidMutationBatch
			}
			continue
		}
		_, exists := edges[operation.ID]
		if (operation.Action == "add" && exists) || ((operation.Action == "update" || operation.Action == "delete") && !exists) {
			return ErrInvalidMutationBatch
		}
		if operation.Action != "delete" && (operation.Source == "" || operation.Target == "") {
			return ErrInvalidMutationBatch
		}
	}
	return nil
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
	if err := r.validateSnapshotSchemaLocked(transaction.StagedSnapshot); err != nil {
		return "", err
	}
	merged := r.newCommit(transaction.StagedSnapshot, []ObjectID{transaction.OriginalTarget, transaction.Binding.SourceCommit}, "", "finalize resolved merge")
	previousObject, objectExisted := r.objects[r.objectID("commit", merged)]
	mergedID, err := r.storeObject("commit", merged)
	if err != nil {
		return "", fmt.Errorf("store finalized merge commit: %w", err)
	}
	previousCommit, commitExisted := r.commits[mergedID]
	r.commits[mergedID], r.branches[targetBranch] = merged, mergedID
	persistErr := r.writeRefLocked(targetBranch, transaction.OriginalTarget, mergedID, "merge-finalize")
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

func (r *Repository) validateSnapshotSchemaLocked(snapshotID ObjectID) error {
	snapshot, ok := r.snapshots[snapshotID]
	if !ok {
		return ErrInvalidSchemaSnapshot
	}
	state := persistedRepository{Objects: r.objects}
	nodes, ok := state.canonicalNodeProjection(snapshot.NodeRoot)
	if !ok {
		return ErrInvalidSchemaSnapshot
	}
	edges, ok := state.canonicalEdgeProjection(snapshot)
	if !ok {
		return ErrInvalidSchemaSnapshot
	}
	schema, err := r.schemaSnapshotLocked(snapshot.SchemaRoot)
	if err != nil {
		return err
	}
	return ValidateSchemaSnapshot(schema, nodes, edges)
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
