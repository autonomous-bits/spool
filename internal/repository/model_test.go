package repository

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

func TestPropertyGraphModelNormalizesRecursivelyAndDeterministically(t *testing.T) {
	node := Node{
		ID:     "node-1",
		Title:  "Compatibility title",
		Labels: []string{"Requirement", "Decision", "Requirement"},
		Properties: map[string]PropertyValue{
			"metadata": {
				Kind: PropertyMap,
				Map: map[string]PropertyValue{
					"z": {Kind: PropertyFloat, Float: math.Copysign(0, -1)},
					"a": {
						Kind: PropertyList,
						List: []PropertyValue{
							{Kind: PropertyString, String: "first", Integer: 99},
							NullPropertyValue(),
						},
					},
				},
			},
		},
	}
	equivalent := Node{
		ID:     "node-1",
		Title:  "Compatibility title",
		Labels: []string{"Decision", "Requirement"},
		Properties: map[string]PropertyValue{
			"metadata": {
				Kind: PropertyMap,
				Map: map[string]PropertyValue{
					"a": {Kind: PropertyList, List: []PropertyValue{{Kind: PropertyString, String: "first"}, NullPropertyValue()}},
					"z": {Kind: PropertyFloat},
				},
			},
		},
	}

	normalized, err := node.Normalize()
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got, want := normalized.Labels, []string{"Decision", "Requirement"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("labels = %#v, want %#v", got, want)
	}
	if got := normalized.Properties["metadata"].Map["a"].List[0]; got.Kind != PropertyString || got.String != "first" || got.Integer != 0 {
		t.Fatalf("normalized list item = %#v", got)
	}
	if math.Signbit(normalized.Properties["metadata"].Map["z"].Float) {
		t.Fatal("normalization retained negative zero")
	}
	if !node.Equal(equivalent) {
		t.Fatal("equivalent normalized nodes are not equal")
	}

	first, err := canonicalObjectEncoding(node)
	if err != nil {
		t.Fatalf("encode node: %v", err)
	}
	second, err := canonicalObjectEncoding(equivalent)
	if err != nil {
		t.Fatalf("encode equivalent node: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("equivalent labels or property-map insertion order changed canonical CBOR")
	}

	repo := NewSeedRepository()
	if firstID, secondID := repo.store("node", node), repo.store("node", equivalent); firstID != secondID {
		t.Fatalf("equivalent nodes have distinct object IDs: %q != %q", firstID, secondID)
	}

	jsonValue, err := json.Marshal(normalized.Properties["metadata"])
	if err != nil {
		t.Fatalf("marshal property JSON: %v", err)
	}
	if !bytes.Contains(jsonValue, []byte(`"kind":"map"`)) {
		t.Fatalf("property JSON lacks tag: %s", jsonValue)
	}
}

func TestPropertyGraphModelRejectsInvalidFloatsAndStoresBuiltinSchema(t *testing.T) {
	if _, err := FloatPropertyValue(math.NaN()).Normalize(); !errors.Is(err, ErrInvalidPropertyValue) {
		t.Fatalf("NaN Normalize error = %v, want ErrInvalidPropertyValue", err)
	}

	if _, err := (SchemaSnapshot{}).Normalize(); !errors.Is(err, ErrInvalidSchemaSnapshot) {
		t.Fatalf("zero schema Normalize error = %v, want ErrInvalidSchemaSnapshot", err)
	}

	repo := NewSeedRepository()
	snapshot := repo.snapshots[repo.commits[repo.branches["main"]].Snapshot]
	var schema SchemaSnapshot
	if err := cbor.Unmarshal(repo.objects[snapshot.SchemaRoot], &schema); err != nil {
		t.Fatalf("decode built-in schema: %v", err)
	}
	if got, want := schema, BuiltinSchemaSnapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("seed schema = %#v, want %#v", got, want)
	}

	edge := Edge{
		ID:         "edge-1",
		Source:     "node-1",
		Target:     "node-2",
		Type:       "DEPENDS_ON",
		Properties: map[string]PropertyValue{"weight": IntegerPropertyValue(3)},
	}
	normalized, err := edge.Normalize()
	if err != nil {
		t.Fatalf("Normalize edge: %v", err)
	}
	if normalized.Type != "DEPENDS_ON" || !normalized.Properties["weight"].Equal(IntegerPropertyValue(3)) {
		t.Fatalf("normalized edge = %#v", normalized)
	}
}

func TestNodeAndEdgeCanonicalizationCollapsesAbsentCollections(t *testing.T) {
	nodeWithoutCollections := Node{ID: "node-1", Title: "Node"}
	nodeWithEmptyCollections := Node{
		ID: "node-1", Title: "Node", Labels: []string{}, Properties: map[string]PropertyValue{},
	}
	if !nodeWithoutCollections.Equal(nodeWithEmptyCollections) {
		t.Fatal("nodes with absent and empty collections are not equivalent")
	}
	nodeWithoutBytes, err := canonicalObjectEncoding(nodeWithoutCollections)
	if err != nil {
		t.Fatalf("encode node without collections: %v", err)
	}
	nodeWithBytes, err := canonicalObjectEncoding(nodeWithEmptyCollections)
	if err != nil {
		t.Fatalf("encode node with empty collections: %v", err)
	}
	if !bytes.Equal(nodeWithoutBytes, nodeWithBytes) {
		t.Fatal("equivalent nodes have distinct canonical encodings")
	}

	edgeWithoutProperties := Edge{ID: "edge-1", Source: "node-1", Target: "node-2"}
	edgeWithEmptyProperties := Edge{
		ID: "edge-1", Source: "node-1", Target: "node-2", Properties: map[string]PropertyValue{},
	}
	if !edgeWithoutProperties.Equal(edgeWithEmptyProperties) {
		t.Fatal("edges with absent and empty properties are not equivalent")
	}
	edgeWithoutBytes, err := canonicalObjectEncoding(edgeWithoutProperties)
	if err != nil {
		t.Fatalf("encode edge without properties: %v", err)
	}
	edgeWithBytes, err := canonicalObjectEncoding(edgeWithEmptyProperties)
	if err != nil {
		t.Fatalf("encode edge with empty properties: %v", err)
	}
	if !bytes.Equal(edgeWithoutBytes, edgeWithBytes) {
		t.Fatal("equivalent edges have distinct canonical encodings")
	}
}
