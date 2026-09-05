package graphcontract

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
)

const (
	goldenNodeCBOR        = "a401666e6f64652d310273436f6d7061746962696c697479207469746c650382684465636973696f6e6b526571756972656d656e7404a1686d65746164617461a201636d617007a26161a201646c6973740682a20166737472696e6705656669727374a101646e756c6c617aa10165666c6f6174"
	goldenEdgeCBOR        = "a50166656467652d3102666e6f64652d3103666e6f64652d32046a444550454e44535f4f4e05a166776569676874a20167696e74656765720303"
	goldenMergeCommitCBOR = "a5016a736e617073686f742d3102826d7461726765742d706172656e746d736f757263652d706172656e74036d6d6572676520666561747572650465616c696365051a6a9c7bc8"
)

func TestCanonicalCBORGoldenVectors(t *testing.T) {
	node := Node{
		ID:     "node-1",
		Title:  "Compatibility title",
		Labels: []string{"Requirement", "Decision", "Requirement"},
		Properties: map[string]PropertyValue{
			"metadata": MapPropertyValue(map[string]PropertyValue{
				"z": FloatPropertyValue(math.Copysign(0, -1)),
				"a": ListPropertyValue([]PropertyValue{
					{Kind: PropertyString, String: "first", Integer: 99},
					NullPropertyValue(),
				}),
			}),
		},
	}
	edge := Edge{
		ID:     "edge-1",
		Source: "node-1",
		Target: "node-2",
		Type:   "DEPENDS_ON",
		Properties: map[string]PropertyValue{
			"weight": IntegerPropertyValue(3),
		},
	}

	nodeData, err := MarshalNode(node)
	if err != nil {
		t.Fatalf("MarshalNode: %v", err)
	}
	edgeData, err := MarshalEdge(edge)
	if err != nil {
		t.Fatalf("MarshalEdge: %v", err)
	}
	if got := hex.EncodeToString(nodeData); got != goldenNodeCBOR {
		t.Fatalf("node CBOR = %s, want %s", got, goldenNodeCBOR)
	}
	if got := hex.EncodeToString(edgeData); got != goldenEdgeCBOR {
		t.Fatalf("edge CBOR = %s, want %s", got, goldenEdgeCBOR)
	}

	decodedNode, err := UnmarshalNode(nodeData)
	if err != nil {
		t.Fatalf("UnmarshalNode: %v", err)
	}
	if !decodedNode.Equal(node) {
		t.Fatalf("decoded node = %#v, want semantic equality with %#v", decodedNode, node)
	}
	decodedEdge, err := UnmarshalEdge(edgeData)
	if err != nil {
		t.Fatalf("UnmarshalEdge: %v", err)
	}
	if !decodedEdge.Equal(edge) {
		t.Fatalf("decoded edge = %#v, want semantic equality with %#v", decodedEdge, edge)
	}

	nonCanonical, err := canonicalCBOR.Marshal(nodeCBOR{ID: "node-1", Labels: []string{"Requirement", "Decision"}})
	if err != nil {
		t.Fatalf("marshal non-canonical node: %v", err)
	}
	if _, err := UnmarshalNode(nonCanonical); !errors.Is(err, ErrInvalidCanonicalCBOR) {
		t.Fatalf("UnmarshalNode non-canonical error = %v, want ErrInvalidCanonicalCBOR", err)
	}
}

func TestCanonicalCBORCollapsesAbsentCollections(t *testing.T) {
	withoutCollections, err := MarshalNode(Node{ID: "node-1", Title: "Node"})
	if err != nil {
		t.Fatalf("MarshalNode without collections: %v", err)
	}

	withEmptyCollections, err := MarshalNode(Node{
		ID: "node-1", Title: "Node", Labels: []string{}, Properties: map[string]PropertyValue{},
	})
	if err != nil {
		t.Fatalf("MarshalNode with empty collections: %v", err)
	}
	if string(withoutCollections) != string(withEmptyCollections) {
		t.Fatal("nodes with absent and empty collections have distinct canonical CBOR")
	}

	withoutProperties, err := MarshalEdge(Edge{ID: "edge-1", Source: "node-1", Target: "node-2"})
	if err != nil {
		t.Fatalf("MarshalEdge without properties: %v", err)
	}
	withEmptyProperties, err := MarshalEdge(Edge{
		ID: "edge-1", Source: "node-1", Target: "node-2", Properties: map[string]PropertyValue{},
	})
	if err != nil {
		t.Fatalf("MarshalEdge with empty properties: %v", err)
	}
	if string(withoutProperties) != string(withEmptyProperties) {
		t.Fatal("edges with absent and empty properties have distinct canonical CBOR")
	}
}

func TestStandardCBORMarshalMatchesCanonicalContract(t *testing.T) {
	property := PropertyValue{Kind: PropertyString, String: "first", Integer: 99}
	genericProperty, err := cbor.Marshal(property)
	if err != nil {
		t.Fatalf("marshal property with cbor: %v", err)
	}
	canonicalProperty, err := MarshalPropertyValue(property)
	if err != nil {
		t.Fatalf("MarshalPropertyValue: %v", err)
	}
	if !bytes.Equal(genericProperty, canonicalProperty) {
		t.Fatal("cbor.Marshal property bytes differ from MarshalPropertyValue")
	}
	if _, err := UnmarshalPropertyValue(genericProperty); err != nil {
		t.Fatalf("UnmarshalPropertyValue generic bytes: %v", err)
	}

	node := Node{
		ID: "node-1", Labels: []string{"Requirement", "Decision", "Requirement"},
		Properties: map[string]PropertyValue{"priority": property},
	}
	genericNode, err := cbor.Marshal(node)
	if err != nil {
		t.Fatalf("marshal node with cbor: %v", err)
	}
	canonicalNode, err := MarshalNode(node)
	if err != nil {
		t.Fatalf("MarshalNode: %v", err)
	}
	if !bytes.Equal(genericNode, canonicalNode) {
		t.Fatal("cbor.Marshal node bytes differ from MarshalNode")
	}
	if _, err := UnmarshalNode(genericNode); err != nil {
		t.Fatalf("UnmarshalNode generic bytes: %v", err)
	}

	edge := Edge{
		ID: "edge-1", Source: "node-1", Target: "node-2", Type: "DEPENDS_ON",
		Properties: map[string]PropertyValue{"weight": FloatPropertyValue(math.Copysign(0, -1))},
	}
	genericEdge, err := cbor.Marshal(edge)
	if err != nil {
		t.Fatalf("marshal edge with cbor: %v", err)
	}
	canonicalEdge, err := MarshalEdge(edge)
	if err != nil {
		t.Fatalf("MarshalEdge: %v", err)
	}
	if !bytes.Equal(genericEdge, canonicalEdge) {
		t.Fatal("cbor.Marshal edge bytes differ from MarshalEdge")
	}
	if _, err := UnmarshalEdge(genericEdge); err != nil {
		t.Fatalf("UnmarshalEdge generic bytes: %v", err)
	}
}

func TestConstructorsNormalizeGraphObjects(t *testing.T) {
	node, err := NewNode("node-1", "Node", []string{"Requirement", "Decision", "Requirement"}, map[string]PropertyValue{
		"priority": {Kind: PropertyInteger, Integer: 3, String: "ignored"},
	})
	if err != nil {
		t.Fatalf("NewNode: %v", err)
	}
	if got, want := node.Labels, []string{"Decision", "Requirement"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("node labels = %#v, want %#v", got, want)
	}
	if value := node.Properties["priority"]; value.String != "" || value.Integer != 3 {
		t.Fatalf("node property = %#v, want normalized integer", value)
	}

	edge, err := NewEdge("edge-1", "node-1", "node-2", "DEPENDS_ON", map[string]PropertyValue{
		"weight": FloatPropertyValue(math.Copysign(0, -1)),
	})
	if err != nil {
		t.Fatalf("NewEdge: %v", err)
	}
	if math.Signbit(edge.Properties["weight"].Float) {
		t.Fatal("NewEdge retained negative zero")
	}
}

func TestCanonicalMergeCommitGoldenVector(t *testing.T) {
	merge := Commit{
		Snapshot: "snapshot-1",
		Parents:  []ObjectID{"target-parent", "source-parent"},
		Message:  "merge feature",
		Author:   "alice",
		Time:     time.Date(2026, time.September, 5, 21, 30, 0, 123456000, time.FixedZone("UTC+1", 3600)),
	}

	data, err := MarshalCommit(merge)
	if err != nil {
		t.Fatalf("MarshalCommit: %v", err)
	}
	if got := hex.EncodeToString(data); got != goldenMergeCommitCBOR {
		t.Fatalf("merge commit CBOR = %s, want %s", got, goldenMergeCommitCBOR)
	}
	decoded, err := UnmarshalCommit(data)
	if err != nil {
		t.Fatalf("UnmarshalCommit: %v", err)
	}
	if !decoded.Equal(merge) {
		t.Fatalf("decoded merge = %#v, want semantic equality with %#v", decoded, merge)
	}
	if want := []ObjectID{"target-parent", "source-parent"}; !slices.Equal(decoded.Parents, want) {
		t.Fatalf("decoded parent order = %#v, want %#v", decoded.Parents, want)
	}

	reordered := merge.Clone()
	reordered.Parents[0], reordered.Parents[1] = reordered.Parents[1], reordered.Parents[0]
	reorderedData, err := MarshalCommit(reordered)
	if err != nil {
		t.Fatalf("MarshalCommit reordered: %v", err)
	}
	if bytes.Equal(data, reorderedData) {
		t.Fatal("reordered merge parents have identical canonical CBOR")
	}

	cloned := merge.Clone()
	cloned.Parents[0] = "changed-parent"
	if merge.Parents[0] == cloned.Parents[0] {
		t.Fatal("Clone shares the parent slice")
	}

	generic, err := cbor.Marshal(merge)
	if err != nil {
		t.Fatalf("cbor.Marshal merge: %v", err)
	}
	if !bytes.Equal(generic, data) {
		t.Fatal("cbor.Marshal merge bytes differ from MarshalCommit")
	}

	nonCanonical, err := canonicalCBOR.Marshal(commitCBOR{
		Snapshot: merge.Snapshot, Message: merge.Message, Author: merge.Author, Time: merge.Time.UTC(),
	})
	if err != nil {
		t.Fatalf("marshal non-canonical merge: %v", err)
	}
	if _, err := UnmarshalCommit(nonCanonical); !errors.Is(err, ErrInvalidCanonicalCBOR) {
		t.Fatalf("UnmarshalCommit non-canonical error = %v, want ErrInvalidCanonicalCBOR", err)
	}
	if _, err := NewCommit("", nil, "", "", time.Time{}); !errors.Is(err, ErrInvalidCommit) {
		t.Fatalf("NewCommit missing snapshot error = %v, want ErrInvalidCommit", err)
	}
	duplicateParents, err := NewCommit("snapshot-1", []ObjectID{"same-parent", "same-parent"}, "", "", time.Time{})
	if err != nil {
		t.Fatalf("NewCommit duplicate parents: %v", err)
	}
	if want := []ObjectID{"same-parent", "same-parent"}; !slices.Equal(duplicateParents.Parents, want) {
		t.Fatalf("duplicate parents = %#v, want %#v", duplicateParents.Parents, want)
	}

	withoutParents, err := MarshalCommit(Commit{Snapshot: "snapshot-1"})
	if err != nil {
		t.Fatalf("MarshalCommit without parents: %v", err)
	}
	withEmptyParents, err := MarshalCommit(Commit{Snapshot: "snapshot-1", Parents: []ObjectID{}})
	if err != nil {
		t.Fatalf("MarshalCommit with empty parents: %v", err)
	}
	if !bytes.Equal(withoutParents, withEmptyParents) {
		t.Fatal("commits with absent and empty parents have distinct canonical CBOR")
	}
}
