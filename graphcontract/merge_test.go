package graphcontract_test

import (
	"reflect"
	"testing"

	"github.com/autonomous-bits/spool/graphcontract"
)

type mergeFixture struct {
	baseNodes        map[string]graphcontract.Node
	sourceNodes      map[string]graphcontract.Node
	targetNodes      map[string]graphcontract.Node
	baseEdges        map[string]graphcontract.Edge
	sourceEdges      map[string]graphcontract.Edge
	targetEdges      map[string]graphcontract.Edge
	baseSchemaRoot   graphcontract.ObjectID
	sourceSchemaRoot graphcontract.ObjectID
	targetSchemaRoot graphcontract.ObjectID
	wantClean        bool
	wantNodes        map[string]graphcontract.Node
	wantEdges        map[string]graphcontract.Edge
	wantSchemaRoot   graphcontract.ObjectID
	wantChanges      []graphcontract.MergeChange
	wantConflicts    []graphcontract.MergeConflict
}

func TestThreeWayMerge(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "clean merge combines independent changes",
			run: func(t *testing.T) {
				baseShared := mustNode(t, "node-shared", "Shared", []string{"Thing"}, map[string]graphcontract.PropertyValue{
					"priority": graphcontract.IntegerPropertyValue(1),
				})
				sourceShared := baseShared.Clone()
				sourceShared.Properties["owner"] = graphcontract.StringPropertyValue("source")
				targetShared := baseShared.Clone()
				targetShared.Properties["status"] = graphcontract.StringPropertyValue("target")

				baseStable := mustNode(t, "node-stable", "Stable", []string{"Thing"}, nil)
				baseRemoved := mustNode(t, "node-removed", "Removed", []string{"Thing"}, nil)
				sourceAdded := mustNode(t, "node-added", "Added", []string{"Feature"}, map[string]graphcontract.PropertyValue{
					"size": graphcontract.IntegerPropertyValue(2),
				})

				baseSharedEdge := mustEdge(t, "edge-shared", "node-shared", "node-stable", "RELATED", map[string]graphcontract.PropertyValue{
					"weight": graphcontract.IntegerPropertyValue(1),
				})
				sourceSharedEdge := baseSharedEdge.Clone()
				sourceSharedEdge.Properties["note"] = graphcontract.StringPropertyValue("source")
				targetSharedEdge := baseSharedEdge.Clone()
				targetSharedEdge.Properties["cost"] = graphcontract.IntegerPropertyValue(4)

				baseRemovedEdge := mustEdge(t, "edge-removed", "node-stable", "node-shared", "LINKS", nil)
				sourceAddedEdge := mustEdge(t, "edge-added", "node-added", "node-stable", "REFERENCES", map[string]graphcontract.PropertyValue{
					"rank": graphcontract.IntegerPropertyValue(3),
				})

				wantShared := targetShared.Clone()
				wantShared.Properties["owner"] = graphcontract.StringPropertyValue("source")
				wantSharedEdge := targetSharedEdge.Clone()
				wantSharedEdge.Properties["note"] = graphcontract.StringPropertyValue("source")

				result := runMergeFixture(t, mergeFixture{
					baseNodes: map[string]graphcontract.Node{
						"node-removed": baseRemoved,
						"node-shared":  baseShared,
						"node-stable":  baseStable,
					},
					sourceNodes: map[string]graphcontract.Node{
						"node-added":  sourceAdded,
						"node-shared": sourceShared,
						"node-stable": baseStable,
					},
					targetNodes: map[string]graphcontract.Node{
						"node-removed": baseRemoved,
						"node-shared":  targetShared,
						"node-stable":  baseStable,
					},
					baseEdges: map[string]graphcontract.Edge{
						"edge-removed": baseRemovedEdge,
						"edge-shared":  baseSharedEdge,
					},
					sourceEdges: map[string]graphcontract.Edge{
						"edge-added":  sourceAddedEdge,
						"edge-shared": sourceSharedEdge,
					},
					targetEdges: map[string]graphcontract.Edge{
						"edge-removed": baseRemovedEdge,
						"edge-shared":  targetSharedEdge,
					},
					wantClean: true,
					wantNodes: map[string]graphcontract.Node{
						"node-added":  sourceAdded,
						"node-shared": wantShared,
						"node-stable": baseStable,
					},
					wantEdges: map[string]graphcontract.Edge{
						"edge-added":  sourceAddedEdge,
						"edge-shared": wantSharedEdge,
					},
					wantChanges: []graphcontract.MergeChange{
						{Entity: "node", ID: "node-added", Change: "added"},
						{Entity: "node", ID: "node-removed", Change: "removed"},
						{Entity: "node", ID: "node-shared", Change: "modified"},
						{Entity: "edge", ID: "edge-added", Change: "added"},
						{Entity: "edge", ID: "edge-removed", Change: "removed"},
						{Entity: "edge", ID: "edge-shared", Change: "modified"},
					},
				})

				if got := result.Nodes["node-shared"].Properties["owner"]; !got.Equal(graphcontract.StringPropertyValue("source")) {
					t.Fatalf("merged node owner = %#v, want source owner", got)
				}
				if got := result.Nodes["node-shared"].Properties["status"]; !got.Equal(graphcontract.StringPropertyValue("target")) {
					t.Fatalf("merged node status = %#v, want target status", got)
				}
				if got := result.Edges["edge-shared"].Properties["note"]; !got.Equal(graphcontract.StringPropertyValue("source")) {
					t.Fatalf("merged edge note = %#v, want source note", got)
				}
				if got := result.Edges["edge-shared"].Properties["cost"]; !got.Equal(graphcontract.IntegerPropertyValue(4)) {
					t.Fatalf("merged edge cost = %#v, want target cost", got)
				}
			},
		},
		{
			name: "structural field conflict reports one node title conflict",
			run: func(t *testing.T) {
				base := mustNode(t, "node-1", "Base", []string{"Thing"}, nil)
				source := base.Clone()
				source.Title = "Source"
				target := base.Clone()
				target.Title = "Target"

				runMergeFixture(t, mergeFixture{
					baseNodes:   map[string]graphcontract.Node{"node-1": base},
					sourceNodes: map[string]graphcontract.Node{"node-1": source},
					targetNodes: map[string]graphcontract.Node{"node-1": target},
					wantClean:   false,
					wantNodes:   map[string]graphcontract.Node{"node-1": target},
					wantConflicts: []graphcontract.MergeConflict{
						{
							Category: "structural",
							Entity:   "node",
							ID:       "node-1",
							Field:    "title",
							Paths:    []string{"node/node-1/title"},
						},
					},
				})
			},
		},
		{
			name: "property conflict reports properties key path",
			run: func(t *testing.T) {
				base := mustNode(t, "node-1", "Base", []string{"Thing"}, map[string]graphcontract.PropertyValue{
					"priority": graphcontract.IntegerPropertyValue(1),
				})
				source := base.Clone()
				source.Properties["priority"] = graphcontract.IntegerPropertyValue(2)
				target := base.Clone()
				target.Properties["priority"] = graphcontract.IntegerPropertyValue(3)

				runMergeFixture(t, mergeFixture{
					baseNodes:   map[string]graphcontract.Node{"node-1": base},
					sourceNodes: map[string]graphcontract.Node{"node-1": source},
					targetNodes: map[string]graphcontract.Node{"node-1": target},
					wantClean:   false,
					wantNodes:   map[string]graphcontract.Node{"node-1": target},
					wantConflicts: []graphcontract.MergeConflict{
						{
							Category: "structural",
							Entity:   "node",
							ID:       "node-1",
							Field:    "properties.priority",
							Paths:    []string{"node/node-1/properties.priority"},
						},
					},
				})
			},
		},
		{
			name: "existence conflict reports delete modify race",
			run: func(t *testing.T) {
				baseNodeA := mustNode(t, "node-a", "A", []string{"Thing"}, nil)
				baseNodeB := mustNode(t, "node-b", "B", []string{"Thing"}, nil)
				baseEdge := mustEdge(t, "edge-1", "node-a", "node-b", "RELATED", nil)
				targetEdge := baseEdge.Clone()
				targetEdge.Type = "REFERENCES"

				runMergeFixture(t, mergeFixture{
					baseNodes: map[string]graphcontract.Node{
						"node-a": baseNodeA,
						"node-b": baseNodeB,
					},
					sourceNodes: map[string]graphcontract.Node{
						"node-a": baseNodeA,
						"node-b": baseNodeB,
					},
					targetNodes: map[string]graphcontract.Node{
						"node-a": baseNodeA,
						"node-b": baseNodeB,
					},
					baseEdges:   map[string]graphcontract.Edge{"edge-1": baseEdge},
					targetEdges: map[string]graphcontract.Edge{"edge-1": targetEdge},
					wantClean:   false,
					wantNodes: map[string]graphcontract.Node{
						"node-a": baseNodeA,
						"node-b": baseNodeB,
					},
					wantEdges: map[string]graphcontract.Edge{"edge-1": targetEdge},
					wantConflicts: []graphcontract.MergeConflict{
						{
							Category: "structural",
							Entity:   "edge",
							ID:       "edge-1",
							Field:    "existence",
							Paths:    []string{"edge/edge-1/existence"},
						},
					},
				})
			},
		},
		{
			name: "one sided changes merge without false positives",
			run: func(t *testing.T) {
				baseNode := mustNode(t, "node-1", "Base", []string{"Thing"}, map[string]graphcontract.PropertyValue{
					"priority": graphcontract.IntegerPropertyValue(1),
				})
				sourceNode := baseNode.Clone()
				sourceNode.Properties["priority"] = graphcontract.IntegerPropertyValue(2)
				targetNode := baseNode.Clone()

				baseEdge := mustEdge(t, "edge-1", "node-1", "node-2", "DEPENDS_ON", nil)
				sourceEdge := baseEdge.Clone()
				targetEdge := baseEdge.Clone()
				targetEdge.Type = "RELATES_TO"

				runMergeFixture(t, mergeFixture{
					baseNodes: map[string]graphcontract.Node{
						"node-1": baseNode,
					},
					sourceNodes: map[string]graphcontract.Node{
						"node-1": sourceNode,
					},
					targetNodes: map[string]graphcontract.Node{
						"node-1": targetNode,
					},
					baseEdges: map[string]graphcontract.Edge{
						"edge-1": baseEdge,
					},
					sourceEdges: map[string]graphcontract.Edge{
						"edge-1": sourceEdge,
					},
					targetEdges: map[string]graphcontract.Edge{
						"edge-1": targetEdge,
					},
					wantClean: true,
					wantNodes: map[string]graphcontract.Node{
						"node-1": sourceNode,
					},
					wantEdges: map[string]graphcontract.Edge{
						"edge-1": targetEdge,
					},
					wantChanges: []graphcontract.MergeChange{
						{Entity: "node", ID: "node-1", Change: "modified"},
					},
				})
			},
		},
		{
			name: "nil versus empty labels do not report a false conflict",
			run: func(t *testing.T) {
				baseNode := mustNode(t, "node-1", "Base", nil, nil)
				sourceNode := baseNode.Clone()
				sourceNode.Labels = []string{}
				targetNode := baseNode.Clone()

				runMergeFixture(t, mergeFixture{
					baseNodes: map[string]graphcontract.Node{
						"node-1": baseNode,
					},
					sourceNodes: map[string]graphcontract.Node{
						"node-1": sourceNode,
					},
					targetNodes: map[string]graphcontract.Node{
						"node-1": targetNode,
					},
					wantClean: true,
					wantNodes: map[string]graphcontract.Node{
						"node-1": targetNode,
					},
				})
			},
		},
		{
			name: "empty conflict paths default the same as nil paths",
			run: func(t *testing.T) {
				baseNode := mustNode(t, "node-1", "Base", []string{"Thing"}, nil)
				sourceNode := baseNode.Clone()
				sourceNode.Title = "Source"
				targetNode := baseNode.Clone()
				targetNode.Title = "Target"

				result := runMergeFixture(t, mergeFixture{
					baseNodes: map[string]graphcontract.Node{
						"node-1": baseNode,
					},
					sourceNodes: map[string]graphcontract.Node{
						"node-1": sourceNode,
					},
					targetNodes: map[string]graphcontract.Node{
						"node-1": targetNode,
					},
					wantClean: false,
					wantNodes: map[string]graphcontract.Node{
						"node-1": targetNode,
					},
					wantConflicts: []graphcontract.MergeConflict{
						{Category: "structural", Entity: "node", ID: "node-1", Field: "title", Paths: []string{"node/node-1/title"}},
					},
				})
				if len(result.Conflicts) != 1 || len(result.Conflicts[0].Paths) == 0 {
					t.Fatalf("conflicts = %#v, want a single conflict with a defaulted, non-empty Paths", result.Conflicts)
				}
			},
		},
		{
			name: "schema root merge and conflict",
			run: func(t *testing.T) {
				tests := []struct {
					name    string
					fixture mergeFixture
				}{
					{
						name: "fast forward",
						fixture: mergeFixture{
							baseSchemaRoot:   "schema-base",
							sourceSchemaRoot: "schema-next",
							targetSchemaRoot: "schema-base",
							wantClean:        true,
							wantSchemaRoot:   "schema-next",
						},
					},
					{
						name: "conflict",
						fixture: mergeFixture{
							baseSchemaRoot:   "schema-base",
							sourceSchemaRoot: "schema-source",
							targetSchemaRoot: "schema-target",
							wantClean:        false,
							wantSchemaRoot:   "schema-target",
							wantConflicts: []graphcontract.MergeConflict{
								{
									Category: "schema",
									Entity:   "schema",
									Field:    "root",
									Paths:    []string{"schema/root"},
								},
							},
						},
					},
				}

				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						runMergeFixture(t, tt.fixture)
					})
				}
			},
		},
		{
			name: "conflict ordering and IDs are deterministic",
			run: func(t *testing.T) {
				baseNodeA := mustNode(t, "node-a", "Base A", []string{"Thing"}, map[string]graphcontract.PropertyValue{
					"priority": graphcontract.IntegerPropertyValue(1),
				})
				sourceNodeA := baseNodeA.Clone()
				sourceNodeA.Properties["priority"] = graphcontract.IntegerPropertyValue(2)
				targetNodeA := baseNodeA.Clone()
				targetNodeA.Properties["priority"] = graphcontract.IntegerPropertyValue(3)

				baseNodeB := mustNode(t, "node-b", "Base B", []string{"Thing"}, nil)
				sourceNodeB := baseNodeB.Clone()
				sourceNodeB.Title = "Source B"
				targetNodeB := baseNodeB.Clone()
				targetNodeB.Title = "Target B"

				baseNodeStable := mustNode(t, "node-stable", "Stable", []string{"Thing"}, nil)

				baseEdge := mustEdge(t, "edge-z", "node-a", "node-b", "RELATED", nil)
				targetEdge := baseEdge.Clone()
				targetEdge.Type = "REFERENCES"

				fixture := mergeFixture{
					baseNodes: map[string]graphcontract.Node{
						"node-a":      baseNodeA,
						"node-b":      baseNodeB,
						"node-stable": baseNodeStable,
					},
					sourceNodes: map[string]graphcontract.Node{
						"node-a":      sourceNodeA,
						"node-b":      sourceNodeB,
						"node-stable": baseNodeStable,
					},
					targetNodes: map[string]graphcontract.Node{
						"node-a":      targetNodeA,
						"node-b":      targetNodeB,
						"node-stable": baseNodeStable,
					},
					baseEdges: map[string]graphcontract.Edge{
						"edge-z": baseEdge,
					},
					targetEdges: map[string]graphcontract.Edge{
						"edge-z": targetEdge,
					},
					baseSchemaRoot:   "schema-base",
					sourceSchemaRoot: "schema-source",
					targetSchemaRoot: "schema-target",
					wantClean:        false,
					wantNodes: map[string]graphcontract.Node{
						"node-a":      targetNodeA,
						"node-b":      targetNodeB,
						"node-stable": baseNodeStable,
					},
					wantEdges: map[string]graphcontract.Edge{
						"edge-z": targetEdge,
					},
					wantConflicts: []graphcontract.MergeConflict{
						{Category: "schema", Entity: "schema", Field: "root", Paths: []string{"schema/root"}},
						{Category: "structural", Entity: "edge", ID: "edge-z", Field: "existence", Paths: []string{"edge/edge-z/existence"}},
						{Category: "structural", Entity: "node", ID: "node-a", Field: "properties.priority", Paths: []string{"node/node-a/properties.priority"}},
						{Category: "structural", Entity: "node", ID: "node-b", Field: "title", Paths: []string{"node/node-b/title"}},
					},
					wantSchemaRoot: "schema-target",
				}

				first := runMergeFixture(t, fixture)
				second := runMergeFixture(t, fixture)

				if !reflect.DeepEqual(first.Conflicts, second.Conflicts) {
					t.Fatalf("conflicts differ between runs:\nfirst:  %#v\nsecond: %#v", first.Conflicts, second.Conflicts)
				}
				for index, conflict := range first.Conflicts {
					if conflict.ConflictID == "" {
						t.Fatalf("conflicts[%d] missing deterministic conflict ID", index)
					}
					if conflict.ConflictID != second.Conflicts[index].ConflictID {
						t.Fatalf("conflict ID %d = %q, want %q", index, conflict.ConflictID, second.Conflicts[index].ConflictID)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func runMergeFixture(t *testing.T, fixture mergeFixture) graphcontract.MergeResult {
	t.Helper()

	result, err := graphcontract.ThreeWayMerge(
		fixture.baseNodes,
		fixture.sourceNodes,
		fixture.targetNodes,
		fixture.baseEdges,
		fixture.sourceEdges,
		fixture.targetEdges,
		fixture.baseSchemaRoot,
		fixture.sourceSchemaRoot,
		fixture.targetSchemaRoot,
	)
	if err != nil {
		t.Fatalf("ThreeWayMerge: %v", err)
	}
	if result.Clean != fixture.wantClean {
		t.Fatalf("clean = %v, want %v", result.Clean, fixture.wantClean)
	}
	if result.SchemaRoot != fixture.wantSchemaRoot {
		t.Fatalf("schema root = %q, want %q", result.SchemaRoot, fixture.wantSchemaRoot)
	}
	assertNodeMapsEqual(t, result.Nodes, fixture.wantNodes)
	assertEdgeMapsEqual(t, result.Edges, fixture.wantEdges)
	if !sameMergeChanges(result.Changes, fixture.wantChanges) {
		t.Fatalf("changes = %#v, want %#v", result.Changes, fixture.wantChanges)
	}
	assertConflictsEqual(t, result.Conflicts, fixture.wantConflicts)
	if len(result.Violations) != 0 {
		t.Fatalf("violations = %#v, want none", result.Violations)
	}
	return result
}

func assertNodeMapsEqual(t *testing.T, got, want map[string]graphcontract.Node) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("node count = %d, want %d", len(got), len(want))
	}
	for id, wantNode := range want {
		gotNode, ok := got[id]
		if !ok {
			t.Fatalf("node %q missing from merged result", id)
		}
		if !gotNode.Equal(wantNode) {
			t.Fatalf("node %q = %#v, want %#v", id, gotNode, wantNode)
		}
	}
}

func assertEdgeMapsEqual(t *testing.T, got, want map[string]graphcontract.Edge) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("edge count = %d, want %d", len(got), len(want))
	}
	for id, wantEdge := range want {
		gotEdge, ok := got[id]
		if !ok {
			t.Fatalf("edge %q missing from merged result", id)
		}
		if !gotEdge.Equal(wantEdge) {
			t.Fatalf("edge %q = %#v, want %#v", id, gotEdge, wantEdge)
		}
	}
}

func assertConflictsEqual(t *testing.T, got, want []graphcontract.MergeConflict) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("conflict count = %d, want %d (%#v)", len(got), len(want), got)
	}
	for index := range want {
		if got[index].ConflictID == "" {
			t.Fatalf("conflict %d missing conflict ID", index)
		}
		if got[index].Category != want[index].Category ||
			got[index].Entity != want[index].Entity ||
			got[index].ID != want[index].ID ||
			got[index].Field != want[index].Field ||
			!reflect.DeepEqual(got[index].Paths, want[index].Paths) {
			t.Fatalf("conflict %d = %#v, want %#v", index, got[index], want[index])
		}
	}
}

func sameMergeChanges(got, want []graphcontract.MergeChange) bool {
	if len(got) == 0 && len(want) == 0 {
		return true
	}
	return reflect.DeepEqual(got, want)
}

func mustNode(t *testing.T, id, title string, labels []string, properties map[string]graphcontract.PropertyValue) graphcontract.Node {
	t.Helper()
	node, err := graphcontract.NewNode(id, title, labels, properties)
	if err != nil {
		t.Fatalf("NewNode(%q): %v", id, err)
	}
	return node
}

func mustEdge(t *testing.T, id, source, target, edgeType string, properties map[string]graphcontract.PropertyValue) graphcontract.Edge {
	t.Helper()
	edge, err := graphcontract.NewEdge(id, source, target, edgeType, properties)
	if err != nil {
		t.Fatalf("NewEdge(%q): %v", id, err)
	}
	return edge
}
