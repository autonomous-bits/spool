package repository

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/fxamacker/cbor/v2"
)

// RetentionScan is the verified object set that a future maintenance operation
// may retain. Objects contains every object reachable from Roots.
type RetentionScan struct {
	Roots            []ObjectID
	Objects          map[ObjectID]struct{}
	RootCount        uint64
	ReachableObjects uint64
}

type retentionRoots struct {
	commits   map[ObjectID]struct{}
	snapshots map[ObjectID]struct{}
}

func newRetentionRoots() retentionRoots {
	return retentionRoots{
		commits: make(map[ObjectID]struct{}), snapshots: make(map[ObjectID]struct{}),
	}
}

func (roots retentionRoots) addCommit(id ObjectID) error {
	if !validLooseObjectID(id) {
		return fmt.Errorf("invalid commit root %q", id)
	}
	if _, snapshot := roots.snapshots[id]; snapshot {
		return fmt.Errorf("object %s is both a commit and snapshot root", id)
	}
	roots.commits[id] = struct{}{}
	return nil
}

func (roots retentionRoots) addSnapshot(id ObjectID) error {
	if !validLooseObjectID(id) {
		return fmt.Errorf("invalid snapshot root %q", id)
	}
	if _, commit := roots.commits[id]; commit {
		return fmt.Errorf("object %s is both a commit and snapshot root", id)
	}
	roots.snapshots[id] = struct{}{}
	return nil
}

func (roots retentionRoots) ids() []ObjectID {
	ids := make([]ObjectID, 0, len(roots.commits)+len(roots.snapshots))
	for id := range roots.commits {
		ids = append(ids, id)
	}
	for id := range roots.snapshots {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// ScanRetention collects durable retention roots and verifies their complete
// object graph. It fails closed: callers must not delete objects on an error.
func (r *Repository) ScanRetention() (RetentionScan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.ensureOpenLocked(); err != nil {
		return RetentionScan{}, err
	}
	scan, err := r.scanRetentionLocked()
	if err != nil {
		return RetentionScan{}, fmt.Errorf("%w: %w", ErrGCCorrupt, err)
	}
	return scan.public(), nil
}

func (r *Repository) scanRetentionLocked() (*retentionScanner, error) {
	roots, err := r.collectRetentionRootsLocked()
	if err != nil {
		return nil, err
	}
	scanner := newRetentionScanner(r.objectStore)
	for _, id := range roots.ids() {
		if _, commit := roots.commits[id]; commit {
			if err := scanner.visitCommit(id); err != nil {
				return nil, err
			}
			continue
		}
		if err := scanner.visitSnapshot(id); err != nil {
			return nil, err
		}
	}
	scanner.roots = roots.ids()
	return scanner, nil
}

func (r *Repository) collectRetentionRootsLocked() (retentionRoots, error) {
	roots := newRetentionRoots()
	if r.mergeStateDir == "" {
		for _, id := range r.branches {
			if err := roots.addCommit(id); err != nil {
				return retentionRoots{}, err
			}
		}
		for target, transaction := range r.mergeTransactions {
			state := persistedMergeTransaction{
				Version: 1, TargetBranch: target, LeaseOwner: r.mergeLeases[target], Transaction: transaction,
			}
			if err := validatePersistedRetentionMergeTransaction(state, r.branches); err != nil {
				return retentionRoots{}, err
			}
			if transaction.Resolved {
				if err := roots.addSnapshot(transaction.StagedSnapshot); err != nil {
					return retentionRoots{}, err
				}
			}
		}
		return roots, nil
	}

	refs, err := r.readRefsLocked()
	if err != nil {
		return retentionRoots{}, err
	}
	for _, id := range refs {
		if err := roots.addCommit(id); err != nil {
			return retentionRoots{}, err
		}
	}
	if err := collectReflogRetentionRoots(r.reflogDirectory(), r.reflogRetentionInventoryPath(), &roots); err != nil {
		return retentionRoots{}, err
	}
	if err := collectMergeRetentionRoots(r.mergeDirectory(), refs, &roots); err != nil {
		return retentionRoots{}, err
	}
	return roots, nil
}

func collectReflogRetentionRoots(root, inventoryPath string, roots *retentionRoots) error {
	paths, err := validateReflogRetentionInventory(root, inventoryPath)
	if err != nil {
		return fmt.Errorf("validate reflog retention inventory: %w", err)
	}
	for _, relative := range paths {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if relative == "HEAD" {
			if err := validateHeadReflog(path); err != nil {
				return fmt.Errorf("read reflog %q: %w", relative, err)
			}
			continue
		}
		if err := readObjectReflog(path, roots); err != nil {
			return fmt.Errorf("read reflog %q: %w", relative, err)
		}
	}
	return nil
}

func readObjectReflog(path string, roots *retentionRoots) error {
	lines, err := readReflogLines(path)
	if err != nil {
		return err
	}
	for _, fields := range lines {
		for _, value := range fields[:2] {
			if value == "" {
				continue
			}
			if err := roots.addCommit(ObjectID(value)); err != nil {
				return fmt.Errorf("invalid reflog object ID: %w", err)
			}
		}
	}
	return nil
}

func validateHeadReflog(path string) error {
	lines, err := readReflogLines(path)
	if err != nil {
		return err
	}
	for _, fields := range lines {
		for _, value := range fields[:2] {
			if value != "" && !validRefName(value) {
				return fmt.Errorf("invalid HEAD reflog reference %q", value)
			}
		}
	}
	return nil
}

func readReflogLines(path string) ([][3]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseReflogLines(data)
}

func readValidatedReflogContent(path, ref string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("reflog is not a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines, err := parseReflogLines(data)
	if err != nil {
		return nil, err
	}
	for _, fields := range lines {
		for _, value := range fields[:2] {
			if value == "" {
				continue
			}
			if ref == "HEAD" {
				if !validRefName(value) {
					return nil, fmt.Errorf("invalid HEAD reflog reference %q", value)
				}
				continue
			}
			if !validLooseObjectID(ObjectID(value)) {
				return nil, fmt.Errorf("invalid reflog object ID %q", value)
			}
		}
	}
	return data, nil
}

func parseReflogLines(data []byte) ([][3]string, error) {
	if len(data) == 0 {
		return [][3]string{}, nil
	}
	if data[len(data)-1] != '\n' {
		return nil, errors.New("reflog is missing a final newline")
	}
	rawLines := strings.Split(string(data[:len(data)-1]), "\n")
	lines := make([][3]string, 0, len(rawLines))
	for _, line := range rawLines {
		fields := strings.Split(line, " ")
		if len(fields) != 3 || fields[2] == "" || strings.ContainsAny(fields[2], "\r\n\t") {
			return nil, fmt.Errorf("invalid reflog entry %q", line)
		}
		lines = append(lines, [3]string{fields[0], fields[1], fields[2]})
	}
	return lines, nil
}

func collectMergeRetentionRoots(directory string, refs map[string]ObjectID, roots *retentionRoots) error {
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return errors.New("merge transaction directory is missing")
	}
	if err != nil {
		return err
	}
	targets := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("merge transaction %q is not a regular file", entry.Name())
		}
		data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return err
		}
		var state persistedMergeTransaction
		if err := json.Unmarshal(data, &state); err != nil {
			return fmt.Errorf("decode merge transaction %q: %w", entry.Name(), err)
		}
		if err := validatePersistedRetentionMergeTransaction(state, refs); err != nil {
			return fmt.Errorf("invalid merge transaction %q: %w", entry.Name(), err)
		}
		if entry.Name() != mergeStateFilename(state.TargetBranch) {
			return fmt.Errorf("merge transaction %q does not match its target branch", entry.Name())
		}
		if _, duplicate := targets[state.TargetBranch]; duplicate {
			return fmt.Errorf("duplicate merge transaction for branch %q", state.TargetBranch)
		}
		targets[state.TargetBranch] = struct{}{}
		if state.Transaction.Resolved {
			if err := roots.addSnapshot(state.Transaction.StagedSnapshot); err != nil {
				return err
			}
		}
	}
	return nil
}

func validatePersistedRetentionMergeTransaction(state persistedMergeTransaction, refs map[string]ObjectID) error {
	transaction := state.Transaction
	if state.Version != 1 || !validRefName(state.TargetBranch) || state.LeaseOwner == "" ||
		state.TargetBranch != transaction.TargetBranch || state.LeaseOwner != transaction.OwnerTransactionID ||
		!validRefName(transaction.SourceBranch) || transaction.Binding.MergeBase == "" ||
		transaction.Binding.SourceCommit == "" || transaction.Binding.TargetCommit == "" || transaction.OriginalTarget == "" ||
		transaction.Preview.ID == "" || transaction.Preview.Binding != transaction.Binding ||
		transaction.Preview.SourceBranch != transaction.SourceBranch || transaction.Preview.TargetBranch != transaction.TargetBranch {
		return errors.New("invalid transaction fields")
	}
	for _, id := range []ObjectID{
		transaction.Binding.MergeBase, transaction.Binding.SourceCommit,
		transaction.Binding.TargetCommit, transaction.OriginalTarget,
	} {
		if !validLooseObjectID(id) {
			return fmt.Errorf("invalid transaction object ID %q", id)
		}
	}
	if refs[transaction.SourceBranch] != transaction.Binding.SourceCommit ||
		refs[state.TargetBranch] != transaction.Binding.TargetCommit ||
		transaction.OriginalTarget != transaction.Binding.TargetCommit {
		return errors.New("transaction binding is stale")
	}
	return validateRetentionMergeTransaction(transaction)
}

func validateRetentionMergeTransaction(transaction mergeTransaction) error {
	if transaction.Resolved {
		if !validLooseObjectID(transaction.StagedSnapshot) {
			return errors.New("resolved transaction has an invalid staged snapshot")
		}
	} else if transaction.StagedSnapshot != "" || transaction.Restaged {
		return errors.New("unresolved transaction has staged resolution state")
	}
	return nil
}

type visitState uint8

const (
	visiting visitState = iota + 1
	visited
)

type verifiedObject struct {
	objectType string
	data       []byte
}

type retentionScanner struct {
	store     *looseObjectStore
	state     map[ObjectID]visitState
	objects   map[ObjectID]verifiedObject
	reachable map[ObjectID]struct{}
	roots     []ObjectID
}

func newRetentionScanner(store *looseObjectStore) *retentionScanner {
	return &retentionScanner{
		store: store, state: make(map[ObjectID]visitState), objects: make(map[ObjectID]verifiedObject),
		reachable: make(map[ObjectID]struct{}),
	}
}

func (s *retentionScanner) public() RetentionScan {
	objects := make(map[ObjectID]struct{}, len(s.reachable))
	for id := range s.reachable {
		objects[id] = struct{}{}
	}
	return RetentionScan{
		Roots: append([]ObjectID(nil), s.roots...), Objects: objects,
		RootCount: uint64(len(s.roots)), ReachableObjects: uint64(len(objects)),
	}
}

func (s *retentionScanner) enter(id ObjectID) (verifiedObject, bool, error) {
	if !validLooseObjectID(id) {
		return verifiedObject{}, false, fmt.Errorf("invalid object ID %q", id)
	}
	switch s.state[id] {
	case visiting:
		return verifiedObject{}, false, fmt.Errorf("object graph contains a cycle at %s", id)
	case visited:
		return s.objects[id], true, nil
	}
	objectType, data, err := s.store.getAnyDurable(id)
	if err != nil {
		return verifiedObject{}, false, fmt.Errorf("read object %s: %w", id, err)
	}
	object := verifiedObject{objectType: objectType, data: data}
	s.state[id], s.objects[id] = visiting, object
	s.reachable[id] = struct{}{}
	return object, false, nil
}

func (s *retentionScanner) finish(id ObjectID) { s.state[id] = visited }

func (s *retentionScanner) visitTyped(id ObjectID, expected string) ([]byte, error) {
	object, done, err := s.enter(id)
	if err != nil {
		return nil, err
	}
	if object.objectType != expected {
		return nil, fmt.Errorf("object %s is %q, want %q", id, object.objectType, expected)
	}
	if !done {
		s.finish(id)
	}
	return object.data, nil
}

func (s *retentionScanner) visitCommit(id ObjectID) (err error) {
	object, done, err := s.enter(id)
	if err != nil {
		return err
	}
	if object.objectType != "commit" {
		return fmt.Errorf("commit root %s is %q", id, object.objectType)
	}
	if done {
		return nil
	}
	defer func() {
		if err == nil {
			s.finish(id)
		}
	}()
	var value commit
	if err = decodeCanonicalObject(object.data, &value); err != nil {
		return fmt.Errorf("decode commit %s: %w", id, err)
	}
	if !validLooseObjectID(value.Snapshot) {
		return fmt.Errorf("commit %s has an invalid snapshot", id)
	}
	parents := make(map[ObjectID]struct{}, len(value.Parents))
	for _, parent := range value.Parents {
		if !validLooseObjectID(parent) {
			return fmt.Errorf("commit %s has an invalid parent", id)
		}
		if _, duplicate := parents[parent]; duplicate {
			return fmt.Errorf("commit %s lists a parent more than once", id)
		}
		parents[parent] = struct{}{}
		if err = s.visitCommit(parent); err != nil {
			return err
		}
	}
	return s.visitSnapshot(value.Snapshot)
}

func (s *retentionScanner) visitSnapshot(id ObjectID) (err error) {
	object, done, err := s.enter(id)
	if err != nil {
		return err
	}
	if object.objectType != "graph-snapshot" {
		return fmt.Errorf("snapshot root %s is %q", id, object.objectType)
	}
	if done {
		return nil
	}
	defer func() {
		if err == nil {
			s.finish(id)
		}
	}()
	var snapshot graphSnapshot
	if err = decodeCanonicalObject(object.data, &snapshot); err != nil {
		return fmt.Errorf("decode snapshot %s: %w", id, err)
	}
	for _, root := range []ObjectID{snapshot.NodeRoot, snapshot.EdgeRoot, snapshot.OutAdjRoot, snapshot.InAdjRoot, snapshot.SchemaRoot} {
		if !validLooseObjectID(root) {
			return fmt.Errorf("snapshot %s has an invalid root", id)
		}
	}
	schemaData, err := s.visitTyped(snapshot.SchemaRoot, "schema-root")
	if err != nil {
		return err
	}
	var schema SchemaSnapshot
	if err = decodeCanonicalObject(schemaData, &schema); err != nil {
		return fmt.Errorf("decode schema %s: %w", snapshot.SchemaRoot, err)
	}
	normalizedSchema, normalizeErr := schema.Normalize()
	if normalizeErr != nil || !reflect.DeepEqual(schema, normalizedSchema) {
		return fmt.Errorf("schema %s is not normalized", snapshot.SchemaRoot)
	}

	nodeEntries, err := s.visitProllyTree(snapshot.NodeRoot, "node")
	if err != nil {
		return err
	}
	edgeEntries, err := s.visitProllyTree(snapshot.EdgeRoot, "edge")
	if err != nil {
		return err
	}
	outEntries, err := s.visitProllyTree(snapshot.OutAdjRoot, "edge")
	if err != nil {
		return err
	}
	inEntries, err := s.visitProllyTree(snapshot.InAdjRoot, "edge")
	if err != nil {
		return err
	}
	if uint64(len(nodeEntries)) != snapshot.NodeCount || uint64(len(edgeEntries)) != snapshot.EdgeCount {
		return fmt.Errorf("snapshot %s counts do not match its trees", id)
	}
	nodes, err := s.nodesForEntries(nodeEntries)
	if err != nil {
		return err
	}
	edges, err := s.edgesForEntries(edgeEntries)
	if err != nil {
		return err
	}
	expectedOut, expectedIn := make([]prollyTreeEntry, 0, len(edgeEntries)), make([]prollyTreeEntry, 0, len(edgeEntries))
	for _, entry := range edgeEntries {
		edge := edges[entry.Key]
		if _, ok := nodes[edge.Source]; !ok {
			return fmt.Errorf("edge %s references a missing source node", entry.Value)
		}
		if _, ok := nodes[edge.Target]; !ok {
			return fmt.Errorf("edge %s references a missing target node", entry.Value)
		}
		expectedOut = append(expectedOut, prollyTreeEntry{Key: adjacencyKey(edge.Source, edge.ID), Value: entry.Value})
		expectedIn = append(expectedIn, prollyTreeEntry{Key: adjacencyKey(edge.Target, edge.ID), Value: entry.Value})
	}
	if !sameProllyEntries(sortedProllyEntries(expectedOut), outEntries) ||
		!sameProllyEntries(sortedProllyEntries(expectedIn), inEntries) {
		return fmt.Errorf("snapshot %s adjacency trees do not match its edge tree", id)
	}
	if err := ValidateSchemaSnapshot(schema, nodes, edges); err != nil {
		return fmt.Errorf("validate snapshot %s schema: %w", id, err)
	}
	return nil
}

func (s *retentionScanner) nodesForEntries(entries []prollyTreeEntry) (map[string]Node, error) {
	nodes := make(map[string]Node, len(entries))
	for _, entry := range entries {
		data, err := s.visitTyped(entry.Value, "node")
		if err != nil {
			return nil, err
		}
		var node Node
		if err := decodeCanonicalObject(data, &node); err != nil {
			return nil, fmt.Errorf("decode node %s: %w", entry.Value, err)
		}
		normalized, err := node.Normalize()
		if err != nil || node.ID == "" || node.ID != entry.Key ||
			!reflect.DeepEqual(canonicalNodeCollections(node), canonicalNodeCollections(normalized)) {
			return nil, fmt.Errorf("node %s does not match its tree key or canonical form", entry.Value)
		}
		if _, duplicate := nodes[node.ID]; duplicate {
			return nil, fmt.Errorf("duplicate node %q", node.ID)
		}
		nodes[node.ID] = node
	}
	return nodes, nil
}

func (s *retentionScanner) edgesForEntries(entries []prollyTreeEntry) (map[string]Edge, error) {
	edges := make(map[string]Edge, len(entries))
	for _, entry := range entries {
		data, err := s.visitTyped(entry.Value, "edge")
		if err != nil {
			return nil, err
		}
		var edge Edge
		if err := decodeCanonicalObject(data, &edge); err != nil {
			return nil, fmt.Errorf("decode edge %s: %w", entry.Value, err)
		}
		normalized, err := edge.Normalize()
		if err != nil || edge.ID == "" || edge.ID != entry.Key || edge.Source == "" || edge.Target == "" ||
			!reflect.DeepEqual(canonicalEdgeProperties(edge), canonicalEdgeProperties(normalized)) {
			return nil, fmt.Errorf("edge %s does not match its tree key or canonical form", entry.Value)
		}
		if _, duplicate := edges[edge.ID]; duplicate {
			return nil, fmt.Errorf("duplicate edge %q", edge.ID)
		}
		edges[edge.ID] = edge
	}
	return edges, nil
}

func (s *retentionScanner) visitProllyTree(id ObjectID, entityType string) (entries []prollyTreeEntry, err error) {
	object, done, err := s.enter(id)
	if err != nil {
		return nil, err
	}
	if object.objectType != prollyTreeLeafType && object.objectType != prollyTreeInternalType {
		return nil, fmt.Errorf("tree object %s has unknown type %q", id, object.objectType)
	}
	if done {
		return s.decodeProllyEntries(id, object, entityType)
	}
	defer func() {
		if err == nil {
			s.finish(id)
		}
	}()
	return s.decodeProllyEntries(id, object, entityType)
}

func (s *retentionScanner) decodeProllyEntries(id ObjectID, object verifiedObject, entityType string) ([]prollyTreeEntry, error) {
	if object.objectType == prollyTreeLeafType {
		var leaf prollyTreeLeaf
		if err := decodeCanonicalObject(object.data, &leaf); err != nil {
			return nil, fmt.Errorf("decode prolly leaf %s: %w", id, err)
		}
		if !validProllyEntries(leaf.Entries, true) {
			return nil, fmt.Errorf("invalid prolly leaf %s", id)
		}
		for _, entry := range leaf.Entries {
			if !validLooseObjectID(entry.Value) {
				return nil, fmt.Errorf("invalid prolly entry object %q", entry.Value)
			}
			if _, err := s.visitTyped(entry.Value, entityType); err != nil {
				return nil, err
			}
		}
		return append([]prollyTreeEntry(nil), leaf.Entries...), nil
	}
	var internal prollyTreeInternal
	if err := decodeCanonicalObject(object.data, &internal); err != nil {
		return nil, fmt.Errorf("decode prolly internal %s: %w", id, err)
	}
	if !validProllyChildren(internal.Children) {
		return nil, fmt.Errorf("invalid prolly internal %s", id)
	}
	entries := make([]prollyTreeEntry, 0)
	for _, child := range internal.Children {
		if !validLooseObjectID(child.Object) {
			return nil, fmt.Errorf("invalid prolly child object %q", child.Object)
		}
		childEntries, err := s.visitProllyTree(child.Object, entityType)
		if err != nil {
			return nil, err
		}
		if len(childEntries) == 0 || childEntries[len(childEntries)-1].Key != child.LastKey {
			return nil, fmt.Errorf("invalid prolly child boundary %s", child.Object)
		}
		entries = append(entries, childEntries...)
	}
	if !validProllyEntries(entries, false) {
		return nil, fmt.Errorf("invalid prolly ordering at %s", id)
	}
	return entries, nil
}

func decodeCanonicalObject(data []byte, target any) error {
	if err := cbor.Unmarshal(data, target); err != nil {
		return err
	}
	encoded, err := canonicalObjectEncoding(reflect.ValueOf(target).Elem().Interface())
	if err != nil || !bytes.Equal(data, encoded) {
		return errors.New("non-canonical object payload")
	}
	return nil
}
