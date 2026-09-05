package graphcontract

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

const (
	goldenNodeCBOR = "a401666e6f64652d310273436f6d7061746962696c697479207469746c650382684465636973696f6e6b526571756972656d656e7404a1686d65746164617461a201636d617007a26161a201646c6973740682a20166737472696e6705656669727374a101646e756c6c617aa10165666c6f6174"
	goldenEdgeCBOR = "a50166656467652d3102666e6f64652d3103666e6f64652d32046a444550454e44535f4f4e05a166776569676874a20167696e74656765720303"
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
