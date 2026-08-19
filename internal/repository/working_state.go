package repository

import (
	"errors"
	"fmt"
	"sort"
)

var (
	// ErrInvalidMutationBatch reports an empty, duplicate, malformed, or inapplicable mutation batch.
	ErrInvalidMutationBatch = errors.New("mutation batch is invalid")
	// ErrMissingEdgeEndpoint reports a mutation that leaves an edge without existing endpoints.
	ErrMissingEdgeEndpoint = errors.New("edge endpoint is missing")
	// ErrNoStagedMutations reports a commit request for a branch without staged changes.
	ErrNoStagedMutations = errors.New("branch has no staged mutations")
	// ErrStaleStagedBase reports staged mutations whose branch head has moved.
	ErrStaleStagedBase = errors.New("staged mutation base is stale")
)

// MutationOperation is one requested graph change in a staged batch.
type MutationOperation struct {
	// Action is "add", "update", or "delete".
	Action string `json:"action"`
	// Entity is "node" or "edge".
	Entity string `json:"entity"`
	// ID identifies the node or edge to change.
	ID string `json:"id"`
	// Title supplies the title for added or updated nodes.
	Title string `json:"title,omitempty"`
	// Source supplies the source node for added or updated edges.
	Source string `json:"source,omitempty"`
	// Target supplies the target node for added or updated edges.
	Target string `json:"target,omitempty"`
	// Labels supplies the labels for added or updated nodes.
	Labels []string `json:"labels"`
	// Type supplies the relationship type for added or updated edges.
	Type string `json:"type,omitempty"`
	// Properties supplies typed properties for added or updated nodes and edges.
	Properties map[string]PropertyValue `json:"properties"`
}

// Normalize returns the canonical representation of an operation's enriched
// node or edge fields while preserving its compatibility fields.
func (o MutationOperation) Normalize() (MutationOperation, error) {
	return o.normalize(true)
}

func (o MutationOperation) normalize(validateIngestion bool) (MutationOperation, error) {
	normalized := o
	switch o.Entity {
	case "node":
		node := Node{Labels: o.Labels, Properties: o.Properties}
		if validateIngestion {
			if err := validateNodeIngestion(node); err != nil {
				return MutationOperation{}, err
			}
		}
		node, err := node.Normalize()
		if err != nil {
			return MutationOperation{}, err
		}
		normalized.Labels, normalized.Properties = node.Labels, node.Properties
	case "edge":
		edge := Edge{Type: o.Type, Properties: o.Properties}
		if validateIngestion {
			if err := validateEdgeIngestion(edge); err != nil {
				return MutationOperation{}, err
			}
		}
		edge, err := edge.Normalize()
		if err != nil {
			return MutationOperation{}, err
		}
		normalized.Properties = edge.Properties
	}
	return normalized, nil
}

func normalizeMutationOperations(operations []MutationOperation) ([]MutationOperation, error) {
	return normalizeMutationOperationsWithIngestion(operations, true)
}

func normalizeStoredMutationOperations(operations []MutationOperation) ([]MutationOperation, error) {
	return normalizeMutationOperationsWithIngestion(operations, false)
}

func normalizeMutationOperationsWithIngestion(operations []MutationOperation, validateIngestion bool) ([]MutationOperation, error) {
	normalized := make([]MutationOperation, len(operations))
	for i, operation := range operations {
		var err error
		normalized[i], err = operation.normalize(validateIngestion)
		if err != nil {
			return nil, fmt.Errorf("%w: normalize operation %d: %w", ErrInvalidMutationBatch, i, err)
		}
	}
	return normalized, nil
}

// StagedMutationSet is the shared, durable staged change set for one branch.
type StagedMutationSet struct {
	// Branch identifies the branch that owns the shared staged set.
	Branch string `json:"branch"`
	// BaseCommit is the branch head validated when the set was staged.
	BaseCommit ObjectID `json:"baseCommit"`
	// Operations is the complete replacement set to materialize on commit.
	Operations []MutationOperation `json:"operations"`
	// TargetSchema is an optional canonical schema installed with Operations.
	// A nil value preserves the base snapshot schema.
	TargetSchema *SchemaSnapshot `json:"targetSchema,omitempty"`
}

// StageMutationRequest replaces the staged mutations for Branch.
type StageMutationRequest struct {
	// Branch identifies the branch to stage against.
	Branch string `json:"branch"`
	// Operations is the complete, validated replacement mutation set.
	Operations []MutationOperation `json:"operations"`
}

// SchemaMigrationRequest atomically stages a schema replacement and its complete
// graph mutation batch against Branch.
type SchemaMigrationRequest struct {
	// Branch identifies the branch to migrate.
	Branch string `json:"branch"`
	// SchemaTOML contains the complete target schema definition.
	SchemaTOML []byte `json:"schemaToml"`
	// Operations transforms the base graph into one conforming to the target schema.
	// It may be empty when the base graph already conforms.
	Operations []MutationOperation `json:"operations"`
}

// StageMutationResult summarizes the persisted shared staged mutation set.
type StageMutationResult struct {
	// Branch identifies the branch whose mutations were staged.
	Branch string `json:"branch"`
	// BaseCommit is the branch head used for validation.
	BaseCommit ObjectID `json:"baseCommit"`
	// Operations is the number of staged operations.
	Operations int `json:"operations"`
}

// BranchStagingStatus describes the shared staged mutation delta for a branch.
type BranchStagingStatus struct {
	// Branch identifies the requested branch.
	Branch string `json:"branch"`
	// BaseCommit is the staged base commit, when changes exist.
	BaseCommit ObjectID `json:"baseCommit,omitempty"`
	// Operations is the current number of shared staged operations.
	Operations int `json:"operations"`
}

// CommitStagedMutationResult identifies the new commit created from a branch's staged mutations.
type CommitStagedMutationResult struct {
	// Branch identifies the branch advanced by the commit.
	Branch string `json:"branch"`
	// Commit identifies the newly materialized commit.
	Commit ObjectID `json:"commit"`
}

// CommitStagedMutationRequest describes the caller-provided metadata for a staged commit.
type CommitStagedMutationRequest struct {
	// Branch identifies the branch whose staged changes are committed.
	Branch string `json:"branch"`
	// Author optionally overrides the default commit author.
	Author string `json:"author,omitempty"`
	// Message optionally overrides the default commit message.
	Message string `json:"message,omitempty"`
}

// CommittedWithWarningError reports that a durable state write completed but its final sync failed.
type CommittedWithWarningError struct {
	// Result identifies the commit that succeeded before final directory synchronization failed.
	Result CommitStagedMutationResult
	err    error
}

// Error returns the underlying durability warning.
func (e *CommittedWithWarningError) Error() string { return e.err.Error() }

// Unwrap returns the underlying durability warning.
func (e *CommittedWithWarningError) Unwrap() error { return e.err }

// BranchStagingStatus returns the current shared staging summary for a branch.
func (r *Repository) BranchStagingStatus(branch string) (BranchStagingStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.ensureOpenLocked(); err != nil {
		return BranchStagingStatus{}, err
	}
	if _, exists := r.branches[branch]; !exists {
		return BranchStagingStatus{}, ErrBranchNotFound
	}

	status := BranchStagingStatus{Branch: branch}
	if staged, exists := r.stagedMutations[branch]; exists {
		status.BaseCommit = staged.BaseCommit
		status.Operations = len(staged.Operations)
	}
	return status, nil
}

// StageMutationBatch atomically replaces a branch's staged mutation set after
// validating every operation against the branch head and this batch's additions.
func (r *Repository) StageMutationBatch(request StageMutationRequest) (StageMutationResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return StageMutationResult{}, err
	}

	head, exists := r.branches[request.Branch]
	if !exists {
		return StageMutationResult{}, ErrBranchNotFound
	}
	operations, err := normalizeMutationOperations(request.Operations)
	if err != nil {
		return StageMutationResult{}, err
	}
	staged := StagedMutationSet{
		Branch:     request.Branch,
		BaseCommit: head,
		Operations: operations,
	}
	if _, _, err := r.candidateGraphLocked(head, staged); err != nil {
		return StageMutationResult{}, err
	}
	return r.replaceStagedMutationsLocked(staged)
}

// StageSchemaMigration atomically replaces a branch's staged set with a parsed
// canonical target schema and the complete graph mutations needed to conform to it.
func (r *Repository) StageSchemaMigration(request SchemaMigrationRequest) (StageMutationResult, error) {
	target, err := DecodeSchemaTOML(request.SchemaTOML)
	if err != nil {
		return StageMutationResult{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return StageMutationResult{}, err
	}
	head, exists := r.branches[request.Branch]
	if !exists {
		return StageMutationResult{}, ErrBranchNotFound
	}
	operations, err := normalizeMutationOperations(request.Operations)
	if err != nil {
		return StageMutationResult{}, err
	}
	staged := StagedMutationSet{
		Branch: request.Branch, BaseCommit: head, Operations: operations, TargetSchema: &target,
	}
	if _, _, err := r.candidateGraphLocked(head, staged); err != nil {
		return StageMutationResult{}, err
	}
	return r.replaceStagedMutationsLocked(staged)
}

// StageSchemaMigrationBatch is an alias for StageSchemaMigration.
func (r *Repository) StageSchemaMigrationBatch(request SchemaMigrationRequest) (StageMutationResult, error) {
	return r.StageSchemaMigration(request)
}

func (r *Repository) replaceStagedMutationsLocked(staged StagedMutationSet) (StageMutationResult, error) {
	previous, hadPrevious := r.stagedMutations[staged.Branch]
	r.stagedMutations[staged.Branch] = staged
	if err := r.persistRepositoryLocked(); err != nil {
		if durableWriteCommitted(err) {
			return StageMutationResult{
				Branch: staged.Branch, BaseCommit: staged.BaseCommit, Operations: len(staged.Operations),
			}, fmt.Errorf("mutation batch staged but directory sync failed: %w", err)
		}
		if hadPrevious {
			r.stagedMutations[staged.Branch] = previous
		} else {
			delete(r.stagedMutations, staged.Branch)
		}
		return StageMutationResult{}, err
	}
	return StageMutationResult{
		Branch: staged.Branch, BaseCommit: staged.BaseCommit, Operations: len(staged.Operations),
	}, nil
}

func (r *Repository) validateMutationBatchLocked(head ObjectID, operations []MutationOperation) error {
	if len(operations) == 0 {
		return ErrInvalidMutationBatch
	}

	snapshot := r.snapshots[r.commits[head].Snapshot]
	existingNodes := r.projections[snapshot.NodeRoot]
	if existingNodes == nil {
		return ErrInvalidMutationBatch
	}
	existingEdges := r.edgeProjections[r.commits[head].Snapshot]

	addedNodes := make(map[string]struct{})
	deletedNodes := make(map[string]struct{})
	for _, operation := range operations {
		if operation.Action == "add" && operation.Entity == "node" && operation.ID != "" {
			addedNodes[operation.ID] = struct{}{}
		}
		if operation.Action == "delete" && operation.Entity == "node" && operation.ID != "" {
			deletedNodes[operation.ID] = struct{}{}
		}
	}
	operationIDs := make(map[string]struct{}, len(operations))
	edgeOperations := make(map[string]MutationOperation)
	var genericInvalid bool
	var missingEndpoint bool
	for _, operation := range operations {
		if operation.Action != "add" && operation.Action != "update" && operation.Action != "delete" {
			genericInvalid = true
			continue
		}
		if operation.Entity != "node" && operation.Entity != "edge" {
			genericInvalid = true
			continue
		}
		if operation.ID == "" {
			genericInvalid = true
			continue
		}
		key := operation.Entity + ":" + operation.ID
		if _, duplicate := operationIDs[key]; duplicate {
			genericInvalid = true
			continue
		}
		operationIDs[key] = struct{}{}

		switch operation.Entity {
		case "node":
			_, exists := existingNodes[operation.ID]
			switch operation.Action {
			case "add":
				if exists || operation.Title == "" {
					genericInvalid = true
					continue
				}
			case "update", "delete":
				if !exists {
					genericInvalid = true
				}
			}
		case "edge":
			edgeOperations[operation.ID] = operation
			_, exists := existingEdges[operation.ID]
			switch operation.Action {
			case "add":
				if exists || operation.Source == "" || operation.Target == "" {
					genericInvalid = true
					continue
				}
				if !mutationNodeExists(operation.Source, existingNodes, addedNodes, deletedNodes) {
					missingEndpoint = true
				}
				if !mutationNodeExists(operation.Target, existingNodes, addedNodes, deletedNodes) {
					missingEndpoint = true
				}
			case "update":
				if !exists || operation.Source == "" || operation.Target == "" {
					genericInvalid = true
					continue
				}
				if !mutationNodeExists(operation.Source, existingNodes, addedNodes, deletedNodes) {
					missingEndpoint = true
				}
				if !mutationNodeExists(operation.Target, existingNodes, addedNodes, deletedNodes) {
					missingEndpoint = true
				}
			case "delete":
				if !exists {
					genericInvalid = true
				}
			}
		}
	}
	for edgeID, edge := range existingEdges {
		if operation, changed := edgeOperations[edgeID]; changed {
			if operation.Action == "delete" {
				continue
			}
			if operation.Action == "update" {
				edge.Source, edge.Target = operation.Source, operation.Target
			}
		}
		if !mutationNodeExists(edge.Source, existingNodes, addedNodes, deletedNodes) ||
			!mutationNodeExists(edge.Target, existingNodes, addedNodes, deletedNodes) {
			missingEndpoint = true
		}
	}
	if missingEndpoint {
		return ErrMissingEdgeEndpoint
	}
	if genericInvalid {
		return ErrInvalidMutationBatch
	}
	return nil
}

// candidateGraphLocked applies staged operations to an isolated graph and
// validates the complete candidate against the schema it will commit with.
func (r *Repository) candidateGraphLocked(head ObjectID, staged StagedMutationSet) (map[string]Node, map[string]Edge, error) {
	if len(staged.Operations) == 0 {
		if staged.TargetSchema == nil {
			return nil, nil, ErrInvalidMutationBatch
		}
	} else if err := r.validateMutationBatchLocked(head, staged.Operations); err != nil {
		return nil, nil, err
	}
	base, ok := r.commits[head]
	if !ok {
		return nil, nil, ErrCommitNotFound
	}
	snapshot, ok := r.snapshots[base.Snapshot]
	if !ok {
		return nil, nil, ErrInvalidMutationBatch
	}
	nodes, edges := cloneNodes(r.projections[snapshot.NodeRoot]), cloneEdges(r.edgeProjections[base.Snapshot])
	applyMutationOperations(nodes, edges, staged.Operations)
	if err := validateCandidateValues(nodes, edges); err != nil {
		return nil, nil, err
	}

	var schema SchemaSnapshot
	var err error
	if staged.TargetSchema != nil {
		schema, err = staged.TargetSchema.Normalize()
	} else {
		schema, err = r.schemaSnapshotLocked(snapshot.SchemaRoot)
	}
	if err != nil {
		return nil, nil, err
	}
	if err := ValidateSchemaSnapshot(schema, nodes, edges); err != nil {
		return nil, nil, err
	}
	return nodes, edges, nil
}

func validateCandidateValues(nodes map[string]Node, edges map[string]Edge) error {
	for _, id := range sortedNodeIDs(nodes) {
		if _, err := nodes[id].Normalize(); err != nil {
			return fmt.Errorf("normalize candidate node %q: %w", id, err)
		}
	}
	for _, id := range sortedEdgeIDs(edges) {
		if _, err := edges[id].Normalize(); err != nil {
			return fmt.Errorf("normalize candidate edge %q: %w", id, err)
		}
	}
	return nil
}

func mutationNodeExists(id string, existing map[string]Node, added, deleted map[string]struct{}) bool {
	if _, removed := deleted[id]; removed {
		return false
	}
	if _, exists := existing[id]; exists {
		return true
	}
	_, exists := added[id]
	return exists
}

// CommitStagedMutations materializes and commits the branch's current staged mutation set.
func (r *Repository) CommitStagedMutations(branch string) (CommitStagedMutationResult, error) {
	return r.CommitStagedMutationBatch(CommitStagedMutationRequest{Branch: branch})
}

// CommitStagedMutationBatch materializes and commits staged mutations with caller metadata.
func (r *Repository) CommitStagedMutationBatch(request CommitStagedMutationRequest) (CommitStagedMutationResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return CommitStagedMutationResult{}, err
	}
	if _, held := r.mergeLeases[request.Branch]; held {
		return CommitStagedMutationResult{}, ErrMergeTargetLeaseHeld
	}
	head, exists := r.branches[request.Branch]
	if !exists {
		return CommitStagedMutationResult{}, ErrBranchNotFound
	}
	staged, exists := r.stagedMutations[request.Branch]
	if !exists {
		return CommitStagedMutationResult{}, ErrNoStagedMutations
	}
	if staged.BaseCommit != head {
		return CommitStagedMutationResult{}, ErrStaleStagedBase
	}

	nodes, edges, err := r.candidateGraphLocked(head, staged)
	if err != nil {
		return CommitStagedMutationResult{}, err
	}
	base := r.commits[head].Snapshot

	objects, snapshots, projections, edgeProjections := r.objects, r.snapshots, r.projections, r.edgeProjections
	commits, branches, stagedMutations := r.commits, r.branches, r.stagedMutations
	r.objects, r.snapshots = cloneObjects(r.objects), cloneSnapshots(r.snapshots)
	r.projections, r.edgeProjections = cloneProjectionMap(r.projections), cloneEdgeProjectionMap(r.edgeProjections)
	r.commits, r.branches, r.stagedMutations = cloneCommits(r.commits), cloneBranches(r.branches), cloneStagedMutations(r.stagedMutations)

	schemaRoot := r.snapshots[base].SchemaRoot
	if staged.TargetSchema != nil {
		schemaRoot = r.store("schema-root", *staged.TargetSchema)
	}
	snapshot, err := r.materializeSnapshotLocked(nodes, edges, schemaRoot)
	if err != nil {
		r.objects, r.snapshots, r.projections, r.edgeProjections = objects, snapshots, projections, edgeProjections
		r.commits, r.branches, r.stagedMutations = commits, branches, stagedMutations
		return CommitStagedMutationResult{}, fmt.Errorf("materialize staged mutations: %w", err)
	}
	snapshotID := r.store("graph-snapshot", snapshot)
	r.snapshots[snapshotID], r.edgeProjections[snapshotID] = snapshot, edges
	if _, exists := r.projections[snapshot.NodeRoot]; !exists {
		r.projections[snapshot.NodeRoot] = nodes
	}
	next := r.newCommit(snapshotID, []ObjectID{head}, request.Author, request.Message)
	nextID := r.store("commit", next)
	r.commits[nextID], r.branches[request.Branch] = next, nextID
	delete(r.stagedMutations, request.Branch)
	result := CommitStagedMutationResult{Branch: request.Branch, Commit: nextID}
	if err := r.persistRepositoryLocked(); err != nil {
		if durableWriteCommitted(err) {
			return result, &CommittedWithWarningError{Result: result, err: fmt.Errorf("staged mutations committed but directory sync failed: %w", err)}
		}
		r.objects, r.snapshots, r.projections, r.edgeProjections = objects, snapshots, projections, edgeProjections
		r.commits, r.branches, r.stagedMutations = commits, branches, stagedMutations
		return CommitStagedMutationResult{}, err
	}
	return result, nil
}

func (r *Repository) materializeSnapshotLocked(nodes map[string]Node, edges map[string]Edge, schemaRoot ObjectID) (graphSnapshot, error) {
	nodeIDs := sortedNodeIDs(nodes)
	nodeObjects := make([]ObjectID, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		node, err := nodes[id].Normalize()
		if err != nil {
			return graphSnapshot{}, fmt.Errorf("normalize node %q: %w", id, err)
		}
		nodes[id] = node
		nodeObjects = append(nodeObjects, r.store("node", node))
	}
	edgeObjects, err := edgeObjectIDs(r, edges, func(a, b Edge) bool { return a.ID < b.ID })
	if err != nil {
		return graphSnapshot{}, err
	}
	outObjects, err := edgeObjectIDs(r, edges, func(a, b Edge) bool {
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		return a.ID < b.ID
	})
	if err != nil {
		return graphSnapshot{}, err
	}
	inObjects, err := edgeObjectIDs(r, edges, func(a, b Edge) bool {
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		return a.ID < b.ID
	})
	if err != nil {
		return graphSnapshot{}, err
	}
	return graphSnapshot{
		NodeRoot: r.store("prolly-node-root", nodeObjects), EdgeRoot: r.store("prolly-edge-root", edgeObjects),
		OutAdjRoot: r.store("prolly-out-adjacency-root", outObjects), InAdjRoot: r.store("prolly-in-adjacency-root", inObjects),
		SchemaRoot: schemaRoot,
	}, nil
}

func sortedNodeIDs(nodes map[string]Node) []string {
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func edgeObjectIDs(r *Repository, edges map[string]Edge, less func(Edge, Edge) bool) ([]ObjectID, error) {
	ids := make([]string, 0, len(edges))
	for id := range edges {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return less(edges[ids[i]], edges[ids[j]]) })
	objects := make([]ObjectID, 0, len(ids))
	for _, id := range ids {
		edge, err := edges[id].Normalize()
		if err != nil {
			return nil, fmt.Errorf("normalize edge %q: %w", id, err)
		}
		edges[id] = edge
		objects = append(objects, r.store("edge", edge))
	}
	return objects, nil
}

func cloneNodes(source map[string]Node) map[string]Node {
	result := make(map[string]Node, len(source))
	for id, value := range source {
		result[id] = value.clone()
	}
	return result
}

func cloneEdges(source map[string]Edge) map[string]Edge {
	result := make(map[string]Edge, len(source))
	for id, value := range source {
		result[id] = value.clone()
	}
	return result
}

func cloneObjects(source map[ObjectID][]byte) map[ObjectID][]byte {
	result := make(map[ObjectID][]byte, len(source))
	for id, value := range source {
		result[id] = value
	}
	return result
}

func cloneSnapshots(source map[ObjectID]graphSnapshot) map[ObjectID]graphSnapshot {
	result := make(map[ObjectID]graphSnapshot, len(source))
	for id, value := range source {
		result[id] = value
	}
	return result
}

func cloneCommits(source map[ObjectID]commit) map[ObjectID]commit {
	result := make(map[ObjectID]commit, len(source))
	for id, value := range source {
		result[id] = value
	}
	return result
}

func cloneBranches(source map[string]ObjectID) map[string]ObjectID {
	result := make(map[string]ObjectID, len(source))
	for id, value := range source {
		result[id] = value
	}
	return result
}

func cloneProjectionMap(source map[ObjectID]map[string]Node) map[ObjectID]map[string]Node {
	result := make(map[ObjectID]map[string]Node, len(source))
	for id, value := range source {
		result[id] = cloneNodes(value)
	}
	return result
}

func cloneEdgeProjectionMap(source map[ObjectID]map[string]Edge) map[ObjectID]map[string]Edge {
	result := make(map[ObjectID]map[string]Edge, len(source))
	for id, value := range source {
		result[id] = cloneEdges(value)
	}
	return result
}

func cloneStagedMutations(source map[string]StagedMutationSet) map[string]StagedMutationSet {
	result := make(map[string]StagedMutationSet, len(source))
	for id, value := range source {
		value.Operations = append([]MutationOperation(nil), value.Operations...)
		for index, operation := range value.Operations {
			value.Operations[index].Labels = cloneStringSlice(operation.Labels)
			value.Operations[index].Properties = cloneProperties(operation.Properties)
		}
		if value.TargetSchema != nil {
			target := *value.TargetSchema
			value.TargetSchema = &target
		}
		result[id] = value
	}
	return result
}

func cloneStringSlice(source []string) []string {
	if source == nil {
		return nil
	}
	result := make([]string, len(source))
	copy(result, source)
	return result
}

func cloneProperties(source map[string]PropertyValue) map[string]PropertyValue {
	if source == nil {
		return nil
	}
	result := make(map[string]PropertyValue, len(source))
	for key, value := range source {
		result[key] = value.clone()
	}
	return result
}
