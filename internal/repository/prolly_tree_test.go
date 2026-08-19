package repository

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

func TestProllySnapshotDeterministicAcrossInsertionOrders(t *testing.T) {
	firstNodes, firstEdges := graphForProllyTest(97, 79, false)
	secondNodes, secondEdges := graphForProllyTest(97, 79, true)
	first := newRepository()
	second := newRepository()
	schemaRoot := first.store("schema-root", BuiltinSchemaSnapshot())
	if other := second.store("schema-root", BuiltinSchemaSnapshot()); other != schemaRoot {
		t.Fatalf("schema roots differ: %s != %s", other, schemaRoot)
	}

	left, err := first.materializeSnapshotLocked(firstNodes, firstEdges, schemaRoot)
	if err != nil {
		t.Fatalf("materialize first snapshot: %v", err)
	}
	right, err := second.materializeSnapshotLocked(secondNodes, secondEdges, schemaRoot)
	if err != nil {
		t.Fatalf("materialize second snapshot: %v", err)
	}
	if left != right {
		t.Fatalf("mutation insertion order changed snapshot roots:\nleft:  %#v\nright: %#v", left, right)
	}
}

func TestProllySnapshotTraversesMultipleLevelsAndReconstructsProjections(t *testing.T) {
	nodes, edges := graphForProllyTest(prollyTreeFanout*prollyTreeFanout+1, 97, false)
	repo := newRepository()
	schemaRoot := repo.store("schema-root", BuiltinSchemaSnapshot())
	snapshot, err := repo.materializeSnapshotLocked(nodes, edges, schemaRoot)
	if err != nil {
		t.Fatalf("materialize snapshot: %v", err)
	}
	if snapshot.NodeCount != uint64(len(nodes)) || snapshot.EdgeCount != uint64(len(edges)) {
		t.Fatalf("snapshot counts = (%d, %d), want (%d, %d)", snapshot.NodeCount, snapshot.EdgeCount, len(nodes), len(edges))
	}
	if depth := assertBoundedProllyTree(t, repo, snapshot.NodeRoot); depth < 3 {
		t.Fatalf("node tree depth = %d, want at least 3", depth)
	}
	for _, root := range []ObjectID{snapshot.EdgeRoot, snapshot.OutAdjRoot, snapshot.InAdjRoot} {
		assertBoundedProllyTree(t, repo, root)
	}

	nodeEntries, err := repo.loadProllyTreeLocked(snapshot.NodeRoot)
	if err != nil {
		t.Fatalf("traverse node tree: %v", err)
	}
	edgeEntries, err := repo.loadProllyTreeLocked(snapshot.EdgeRoot)
	if err != nil {
		t.Fatalf("traverse edge tree: %v", err)
	}
	outEntries, err := repo.loadProllyTreeLocked(snapshot.OutAdjRoot)
	if err != nil {
		t.Fatalf("traverse outgoing tree: %v", err)
	}
	inEntries, err := repo.loadProllyTreeLocked(snapshot.InAdjRoot)
	if err != nil {
		t.Fatalf("traverse incoming tree: %v", err)
	}
	assertSortedEntries(t, nodeEntries)
	assertSortedEntries(t, edgeEntries)
	assertSortedEntries(t, outEntries)
	assertSortedEntries(t, inEntries)
	if len(nodeEntries) != len(nodes) || len(edgeEntries) != len(edges) {
		t.Fatalf("tree entry counts = (%d, %d), want (%d, %d)", len(nodeEntries), len(edgeEntries), len(nodes), len(edges))
	}

	expectedOut, expectedIn := expectedAdjacencyEntries(edgeEntries, edges)
	if !sameProllyEntries(outEntries, expectedOut) || !sameProllyEntries(inEntries, expectedIn) {
		t.Fatal("adjacency entries do not preserve their canonical endpoint-and-edge ordering")
	}

	snapshotID := repo.store("graph-snapshot", snapshot)
	repo.snapshots[snapshotID] = snapshot
	delete(repo.projections, snapshot.NodeRoot)
	delete(repo.edgeProjections, snapshotID)
	if err := repo.reconstructSnapshotProjectionsLocked(snapshotID, snapshot); err != nil {
		t.Fatalf("reconstruct projections: %v", err)
	}
	if !reflect.DeepEqual(repo.projections[snapshot.NodeRoot], nodes) {
		t.Fatal("reconstructed nodes differ from materialized nodes")
	}
	if !reflect.DeepEqual(repo.edgeProjections[snapshotID], edges) {
		t.Fatal("reconstructed edges differ from materialized edges")
	}
}

func TestProllySnapshotReconstructsDurableProjections(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("create durable repository: %v", err)
	}
	if _, err := repo.StageMutationBatch(StageMutationRequest{
		Branch: "main",
		Operations: []MutationOperation{{
			Action: "add", Entity: "node", ID: "node-durable", Title: "Durable node",
		}},
	}); err != nil {
		t.Fatalf("stage node: %v", err)
	}
	committed, err := repo.CommitStagedMutations("main")
	if err != nil {
		t.Fatalf("commit node: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("close repository: %v", err)
	}
	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("reopen repository: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	snapshot := reopened.snapshots[reopened.commits[committed.Commit].Snapshot]
	if node, ok := reopened.projections[snapshot.NodeRoot]["node-durable"]; !ok || node.Title != "Durable node" {
		t.Fatalf("durably reconstructed node = %#v, exists=%v", node, ok)
	}
	if got := snapshot.NodeCount; got != 2 {
		t.Fatalf("durably reconstructed NodeCount = %d, want 2", got)
	}
}

func TestProllyTreeRejectsNonCanonicalObjectPayload(t *testing.T) {
	repo := newRepository()
	nonCanonical := []byte{0xbf, 0x01, 0x80, 0xff} // {_ 1: [] _}
	var decoded prollyTreeLeaf
	if err := cbor.Unmarshal(nonCanonical, &decoded); err != nil {
		t.Fatalf("decode non-canonical fixture: %v", err)
	}
	canonical, err := canonicalObjectEncoding(decoded)
	if err != nil {
		t.Fatalf("encode canonical leaf: %v", err)
	}
	if string(nonCanonical) == string(canonical) {
		t.Fatal("fixture unexpectedly has canonical encoding")
	}

	id := objectIDForEncoded(prollyTreeLeafType, nonCanonical)
	repo.objects[id] = nonCanonical
	if _, err := repo.loadProllyTreeLocked(id); err == nil {
		t.Fatal("non-canonical prolly tree payload was accepted")
	}
}

func TestAdjacencyKeyIsInjectiveAndPreservesTupleOrder(t *testing.T) {
	pairs := []struct {
		endpoint string
		edgeID   string
	}{
		{endpoint: "a", edgeID: "a"},
		{endpoint: "a", edgeID: "a\x00"},
		{endpoint: "a", edgeID: "b"},
		{endpoint: "a", edgeID: "\xff"},
		{endpoint: "a\x00", edgeID: "a"},
		{endpoint: "a\x00", edgeID: "a\x00"},
		{endpoint: "a\x00", edgeID: "b"},
		{endpoint: "a\x00b", edgeID: "a"},
		{endpoint: "b", edgeID: "a"},
	}
	keys := make(map[string]struct{}, len(pairs))
	previous := ""
	for i, pair := range pairs {
		key := adjacencyKey(pair.endpoint, pair.edgeID)
		if _, duplicate := keys[key]; duplicate {
			t.Fatalf("duplicate key for endpoint %q and edge %q", pair.endpoint, pair.edgeID)
		}
		if i > 0 && previous >= key {
			t.Fatalf("keys are out of endpoint-and-edge order: %q >= %q", previous, key)
		}
		keys[key] = struct{}{}
		previous = key
	}
}

func graphForProllyTest(nodeCount, edgeCount int, reverse bool) (map[string]Node, map[string]Edge) {
	nodes := make(map[string]Node, nodeCount)
	edges := make(map[string]Edge, edgeCount)
	addNode := func(index int) {
		id := fmt.Sprintf("node-%04d", index)
		nodes[id] = Node{ID: id, Title: fmt.Sprintf("Node %d", index)}
	}
	addEdge := func(index int) {
		id := fmt.Sprintf("edge-%04d", index)
		edges[id] = Edge{
			ID: id, Source: fmt.Sprintf("node-%04d", index%nodeCount),
			Target: fmt.Sprintf("node-%04d", (index*7+3)%nodeCount),
		}
	}
	if reverse {
		for i := nodeCount - 1; i >= 0; i-- {
			addNode(i)
		}
		for i := edgeCount - 1; i >= 0; i-- {
			addEdge(i)
		}
	} else {
		for i := 0; i < nodeCount; i++ {
			addNode(i)
		}
		for i := 0; i < edgeCount; i++ {
			addEdge(i)
		}
	}
	return nodes, edges
}

func expectedAdjacencyEntries(edgeEntries []prollyTreeEntry, edges map[string]Edge) ([]prollyTreeEntry, []prollyTreeEntry) {
	out := make([]prollyTreeEntry, 0, len(edgeEntries))
	in := make([]prollyTreeEntry, 0, len(edgeEntries))
	for _, entry := range edgeEntries {
		edge := edges[entry.Key]
		out = append(out, prollyTreeEntry{Key: adjacencyKey(edge.Source, edge.ID), Value: entry.Value})
		in = append(in, prollyTreeEntry{Key: adjacencyKey(edge.Target, edge.ID), Value: entry.Value})
	}
	return sortedProllyEntries(out), sortedProllyEntries(in)
}

func assertSortedEntries(t *testing.T, entries []prollyTreeEntry) {
	t.Helper()
	if !validProllyEntries(entries, false) {
		t.Fatalf("invalid sorted entries: %#v", entries)
	}
}

func assertBoundedProllyTree(t *testing.T, repo *Repository, root ObjectID) int {
	t.Helper()
	data, err := repo.objectStore.get(root, prollyTreeLeafType)
	if err == nil {
		var leaf prollyTreeLeaf
		if err := cbor.Unmarshal(data, &leaf); err != nil || !validProllyEntries(leaf.Entries, true) {
			t.Fatalf("invalid leaf %s: %v", root, err)
		}
		return 1
	}
	data, err = repo.objectStore.get(root, prollyTreeInternalType)
	if err != nil {
		t.Fatalf("read tree object %s: %v", root, err)
	}
	var internal prollyTreeInternal
	if err := cbor.Unmarshal(data, &internal); err != nil || !validProllyChildren(internal.Children) {
		t.Fatalf("invalid internal %s: %v", root, err)
	}
	depth := 0
	for _, child := range internal.Children {
		childDepth := assertBoundedProllyTree(t, repo, child.Object)
		if childDepth > depth {
			depth = childDepth
		}
	}
	return depth + 1
}
