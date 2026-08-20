package repository

import (
	"errors"
	"reflect"
	"testing"
)

func TestResolvePinnedReturnsStoredSchemaVersionAndClonedTypedNode(t *testing.T) {
	repo := NewSeedRepository()
	head := repo.branches["main"]
	base := repo.snapshots[repo.commits[head].Snapshot]
	node := richReadNode(SeedNodeID, "Seed")
	nodes := map[string]Node{SeedNodeID: node}
	edges := map[string]Edge{}
	schemaRoot := repo.store("schema-root", SchemaSnapshot{Version: 7, Permissive: true})
	commit := commitReadSurfaceSnapshot(t, repo, nodes, edges, schemaRoot)

	result, err := repo.ResolvePinned(commit, SeedNodeID)
	if err != nil {
		t.Fatalf("ResolvePinned: %v", err)
	}
	if result.SchemaVersion != 7 || !result.Node.Equal(node) {
		t.Fatalf("resolution = %#v, want schema 7 and node %#v", result, node)
	}

	result.Node.Labels[0] = "mutated"
	result.Node.Properties["metadata"] = StringPropertyValue("mutated")
	again, err := repo.ResolvePinned(commit, SeedNodeID)
	if err != nil {
		t.Fatalf("ResolvePinned again: %v", err)
	}
	if !again.Node.Equal(node) {
		t.Fatalf("resolved node was mutated through result: %#v", again.Node)
	}
	if base.SchemaRoot == schemaRoot {
		t.Fatal("test did not use a distinct schema root")
	}
}

func TestReadSurfacesRetainTypedGraphFields(t *testing.T) {
	repo := NewSeedRepository()
	nodes := map[string]Node{
		SeedNodeID: richReadNode(SeedNodeID, "Seed"),
		"node-2":   richReadNode("node-2", "Related"),
	}
	edges := map[string]Edge{
		"edge-context": richReadEdge("edge-context", SeedNodeID, "node-2", "RELATES_TO"),
		"edge-updated": richReadEdge("edge-updated", SeedNodeID, "node-2", "DEPENDS_ON"),
	}
	base := commitReadSurfaceSnapshot(t, repo, nodes, edges, repo.snapshots[repo.commits[repo.branches["main"]].Snapshot].SchemaRoot)

	targetNodes, targetEdges := cloneNodes(nodes), cloneEdges(edges)
	targetNodes[SeedNodeID] = Node{
		ID:         SeedNodeID,
		Title:      "Updated seed",
		Labels:     []string{"Changed", "Requirement"},
		Properties: map[string]PropertyValue{"metadata": StringPropertyValue("updated")},
	}
	targetEdges["edge-updated"] = richReadEdge("edge-updated", SeedNodeID, "node-2", "BLOCKS")
	target := commitReadSurfaceSnapshot(t, repo, targetNodes, targetEdges, repo.snapshots[repo.commits[base].Snapshot].SchemaRoot)

	diff, err := repo.Diff(DiffRequest{
		Base: base, Target: target,
		IncludeOneHop: true, MaxRows: 10, MaxResponseBytes: 1 << 20,
	})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if got := diffNodeByID(diff.Changes, SeedNodeID); got == nil || !got.Equal(targetNodes[SeedNodeID]) {
		t.Fatalf("typed node diff = %#v", got)
	}
	if got := diffEdgeByID(diff.Changes, "edge-updated"); got == nil || !got.Equal(targetEdges["edge-updated"]) {
		t.Fatalf("typed edge diff = %#v", got)
	}
	if got := diffContextNodeByID(diff.Context, "node-2"); got == nil || !got.Equal(nodes["node-2"]) {
		t.Fatalf("typed node context = %#v", got)
	}
	if got := diffContextEdgeByID(diff.Context, "edge-context"); got == nil || !got.Equal(edges["edge-context"]) {
		t.Fatalf("typed edge context = %#v", got)
	}
	diffNodeByID(diff.Changes, SeedNodeID).Properties["metadata"] = StringPropertyValue("mutated")
	diffEdgeByID(diff.Changes, "edge-updated").Properties["weight"] = IntegerPropertyValue(99)
	diffContextNodeByID(diff.Context, "node-2").Labels[0] = "mutated"
	diffContextEdgeByID(diff.Context, "edge-context").Properties["weight"] = IntegerPropertyValue(99)
	again, err := repo.Diff(DiffRequest{
		Base: base, Target: target,
		IncludeOneHop: true, MaxRows: 10, MaxResponseBytes: 1 << 20,
	})
	if err != nil {
		t.Fatalf("Diff again: %v", err)
	}
	if got := diffNodeByID(again.Changes, SeedNodeID); got == nil || !got.Equal(targetNodes[SeedNodeID]) {
		t.Fatalf("diff node was mutated through result: %#v", got)
	}
	if got := diffContextEdgeByID(again.Context, "edge-context"); got == nil || !got.Equal(edges["edge-context"]) {
		t.Fatalf("diff context edge was mutated through result: %#v", got)
	}

	history, err := repo.History(HistoryRequest{Commit: target, EntityID: SeedNodeID})
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if got, want := history.Entries[0].ChangedFields, []string{"title", "labels", "properties"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changed fields = %#v, want %#v", got, want)
	}
	if len(history.Entries[0].EdgeAdditions) != 1 || !history.Entries[0].EdgeAdditions[0].Equal(targetEdges["edge-updated"]) ||
		len(history.Entries[0].EdgeRemovals) != 1 || !history.Entries[0].EdgeRemovals[0].Equal(edges["edge-updated"]) {
		t.Fatalf("typed edge history = %#v", history.Entries[0])
	}
	history.Entries[0].EdgeAdditions[0].Properties["weight"] = IntegerPropertyValue(99)
	history, err = repo.History(HistoryRequest{Commit: target, EntityID: SeedNodeID})
	if err != nil {
		t.Fatalf("History again: %v", err)
	}
	if !history.Entries[0].EdgeAdditions[0].Equal(targetEdges["edge-updated"]) {
		t.Fatalf("history edge was mutated through result: %#v", history.Entries[0].EdgeAdditions)
	}

	impact, err := repo.Impact(ImpactRequest{
		Commit:   base,
		Delta:    []MutationOperation{{Action: "update", Entity: "node", ID: SeedNodeID, Title: "Hypothetical"}},
		MaxDepth: 1, MaxVisited: 10,
	})
	if err != nil {
		t.Fatalf("Impact: %v", err)
	}
	wantImpactNode := nodes[SeedNodeID]
	wantImpactNode.Title = "Hypothetical"
	if len(impact.Impacts) == 0 || !impact.Impacts[0].Node.Equal(wantImpactNode) {
		t.Fatalf("typed impact node = %#v, want %#v", impact.Impacts, wantImpactNode)
	}

	clonedNodes, clonedEdges := cloneNodes(nodes), cloneEdges(edges)
	applyMutationOperations(clonedNodes, clonedEdges, []MutationOperation{
		{Action: "update", Entity: "edge", ID: "edge-context", Source: "node-2", Target: SeedNodeID},
	})
	wantImpactEdge := edges["edge-context"]
	wantImpactEdge.Source, wantImpactEdge.Target = "node-2", SeedNodeID
	if !clonedEdges["edge-context"].Equal(wantImpactEdge) {
		t.Fatalf("typed impact edge clone = %#v, want %#v", clonedEdges["edge-context"], wantImpactEdge)
	}
}

func TestPinnedReadAPIsDoNotMoveBranches(t *testing.T) {
	repo := NewSeedRepository()
	head, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}

	if _, err := repo.Diff(DiffRequest{
		Base: head, Target: head, MaxRows: 10, MaxResponseBytes: 1 << 20,
	}); err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if _, err := repo.History(HistoryRequest{Commit: head, EntityID: SeedNodeID}); err != nil {
		t.Fatalf("History: %v", err)
	}
	if _, err := repo.Impact(ImpactRequest{
		Commit:   head,
		Delta:    []MutationOperation{{Action: "update", Entity: "node", ID: SeedNodeID, Title: "Hypothetical"}},
		MaxDepth: 1, MaxVisited: 10,
	}); err != nil {
		t.Fatalf("Impact: %v", err)
	}

	current, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch after reads: %v", err)
	}
	if current != head {
		t.Fatalf("pinned read APIs moved main from %q to %q", head, current)
	}
}

func TestResolvePinnedRejectsUnknownCommitWithCommitNotFound(t *testing.T) {
	_, err := NewSeedRepository().ResolvePinned("missing", SeedNodeID)
	if !errors.Is(err, ErrCommitNotFound) {
		t.Fatalf("ResolvePinned error = %v, want ErrCommitNotFound", err)
	}
}

func commitReadSurfaceSnapshot(t *testing.T, repo *Repository, nodes map[string]Node, edges map[string]Edge, schemaRoot ObjectID) ObjectID {
	t.Helper()
	parent := repo.branches["main"]
	snapshot, err := repo.materializeSnapshotLocked(nodes, edges, schemaRoot)
	if err != nil {
		t.Fatalf("materialize snapshot: %v", err)
	}
	snapshotID := repo.store("graph-snapshot", snapshot)
	repo.snapshots[snapshotID] = snapshot
	repo.projections[snapshot.NodeRoot] = nodes
	repo.edgeProjections[snapshotID] = edges
	next := repo.newCommit(snapshotID, []ObjectID{parent}, "", "")
	commitID := repo.store("commit", next)
	repo.commits[commitID] = next
	repo.branches["main"] = commitID
	return commitID
}

func richReadNode(id, title string) Node {
	return Node{
		ID:     id,
		Title:  title,
		Labels: []string{"Requirement", "Decision"},
		Properties: map[string]PropertyValue{
			"metadata": MapPropertyValue(map[string]PropertyValue{
				"priority": IntegerPropertyValue(3),
				"tags":     ListPropertyValue([]PropertyValue{StringPropertyValue("read")}),
			}),
		},
	}
}

func richReadEdge(id, source, target, edgeType string) Edge {
	return Edge{
		ID: id, Source: source, Target: target, Type: edgeType,
		Properties: map[string]PropertyValue{"weight": IntegerPropertyValue(3)},
	}
}

func diffNodeByID(entries []DiffEntry, id string) *Node {
	for _, entry := range entries {
		if entry.ID == id {
			return entry.Node
		}
	}
	return nil
}

func diffEdgeByID(entries []DiffEntry, id string) *Edge {
	for _, entry := range entries {
		if entry.ID == id {
			return entry.Edge
		}
	}
	return nil
}

func diffContextNodeByID(context []DiffContext, id string) *Node {
	for _, entry := range context {
		if entry.ID == id {
			return entry.Node
		}
	}
	return nil
}

func diffContextEdgeByID(context []DiffContext, id string) *Edge {
	for _, entry := range context {
		if entry.ID == id {
			return entry.Edge
		}
	}
	return nil
}
