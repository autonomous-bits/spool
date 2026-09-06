package graphcontract_test

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/autonomous-bits/spool/graphcontract"
)

const objectFixtureFormatVersion = 1

// objectFixture is the versioned, Rack-consumable conformance format for
// canonical Node and Edge encoding. A fixture with a non-empty "error"
// field must be rejected; otherwise its value must marshal to exactly
// CanonicalCBORHex and hash to exactly ObjectID.
type objectFixture struct {
	FormatVersion    int             `json:"format_version"`
	ObjectType       string          `json:"object_type"`
	Value            json.RawMessage `json:"value"`
	CanonicalCBORHex string          `json:"canonical_cbor_hex"`
	ObjectID         string          `json:"object_id"`
	Error            string          `json:"error"`
}

type nodeValue struct {
	ID         string                                 `json:"id"`
	Title      string                                 `json:"title"`
	Labels     []string                               `json:"labels"`
	Properties map[string]graphcontract.PropertyValue `json:"properties"`
}

type edgeValue struct {
	ID         string                                 `json:"id"`
	Source     string                                 `json:"source"`
	Target     string                                 `json:"target"`
	Type       string                                 `json:"type"`
	Properties map[string]graphcontract.PropertyValue `json:"properties"`
}

func loadObjectFixtures(t *testing.T) []objectFixture {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "objects", "v1", "*.json"))
	if err != nil {
		t.Fatalf("list object fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no object conformance fixtures")
	}
	fixtures := make([]objectFixture, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read object fixture %s: %v", path, err)
		}
		var fixture objectFixture
		if err := json.Unmarshal(data, &fixture); err != nil {
			t.Fatalf("decode object fixture %s: %v", path, err)
		}
		if fixture.FormatVersion != objectFixtureFormatVersion {
			t.Fatalf("%s: format version = %d, want %d", path, fixture.FormatVersion, objectFixtureFormatVersion)
		}
		fixtures = append(fixtures, fixture)
	}
	return fixtures
}

// TestObjectConformanceFixtures proves canonical Node and Edge encoding,
// decoding, and content-derived ObjectID computation are stable, covering
// every PropertyValue kind, minimal (collection-absent) values, invalid
// property kinds, and non-canonical CBOR rejection.
func TestObjectConformanceFixtures(t *testing.T) {
	for _, fixture := range loadObjectFixtures(t) {
		fixture := fixture
		t.Run(fixture.ObjectType+"/"+fixture.ObjectID+fixture.Error, func(t *testing.T) {
			switch {
			case fixture.CanonicalCBORHex != "" && fixture.Error == "":
				assertValidObjectFixture(t, fixture)
			case fixture.Value != nil && fixture.Error != "":
				assertInvalidPropertyFixture(t, fixture)
			case fixture.CanonicalCBORHex != "" && fixture.Error != "":
				assertNonCanonicalObjectFixture(t, fixture)
			default:
				t.Fatalf("fixture has neither a valid value nor a recognized error case")
			}
		})
	}
}

func assertValidObjectFixture(t *testing.T, fixture objectFixture) {
	t.Helper()
	wantHex := fixture.CanonicalCBORHex
	wantID := graphcontract.ObjectID(fixture.ObjectID)
	switch fixture.ObjectType {
	case "node":
		var value nodeValue
		if err := json.Unmarshal(fixture.Value, &value); err != nil {
			t.Fatalf("decode node value: %v", err)
		}
		node, err := graphcontract.NewNode(value.ID, value.Title, value.Labels, value.Properties)
		if err != nil {
			t.Fatalf("NewNode: %v", err)
		}
		encoded, err := graphcontract.MarshalNode(node)
		if err != nil {
			t.Fatalf("MarshalNode: %v", err)
		}
		if got := hex.EncodeToString(encoded); got != wantHex {
			t.Fatalf("node canonical CBOR = %s, want %s", got, wantHex)
		}
		if got := graphcontract.ObjectIDForEncoded("node", encoded); got != wantID {
			t.Fatalf("node object id = %s, want %s", got, wantID)
		}
		decoded, err := graphcontract.UnmarshalNode(encoded)
		if err != nil {
			t.Fatalf("UnmarshalNode: %v", err)
		}
		if !decoded.Equal(node) {
			t.Fatalf("decoded node = %#v, want semantic equality with %#v", decoded, node)
		}
	case "edge":
		var value edgeValue
		if err := json.Unmarshal(fixture.Value, &value); err != nil {
			t.Fatalf("decode edge value: %v", err)
		}
		edge, err := graphcontract.NewEdge(value.ID, value.Source, value.Target, value.Type, value.Properties)
		if err != nil {
			t.Fatalf("NewEdge: %v", err)
		}
		encoded, err := graphcontract.MarshalEdge(edge)
		if err != nil {
			t.Fatalf("MarshalEdge: %v", err)
		}
		if got := hex.EncodeToString(encoded); got != wantHex {
			t.Fatalf("edge canonical CBOR = %s, want %s", got, wantHex)
		}
		if got := graphcontract.ObjectIDForEncoded("edge", encoded); got != wantID {
			t.Fatalf("edge object id = %s, want %s", got, wantID)
		}
		decoded, err := graphcontract.UnmarshalEdge(encoded)
		if err != nil {
			t.Fatalf("UnmarshalEdge: %v", err)
		}
		if !decoded.Equal(edge) {
			t.Fatalf("decoded edge = %#v, want semantic equality with %#v", decoded, edge)
		}
	default:
		t.Fatalf("unknown object_type %q", fixture.ObjectType)
	}
}

func assertInvalidPropertyFixture(t *testing.T, fixture objectFixture) {
	t.Helper()
	if fixture.ObjectType != "node" {
		t.Fatalf("unsupported invalid object_type %q", fixture.ObjectType)
	}
	var value nodeValue
	if err := json.Unmarshal(fixture.Value, &value); err != nil {
		t.Fatalf("decode node value: %v", err)
	}
	if _, err := graphcontract.NewNode(value.ID, value.Title, value.Labels, value.Properties); !errors.Is(err, graphcontract.ErrInvalidPropertyValue) {
		t.Fatalf("NewNode error = %v, want ErrInvalidPropertyValue", err)
	}
}

func assertNonCanonicalObjectFixture(t *testing.T, fixture objectFixture) {
	t.Helper()
	data, err := hex.DecodeString(fixture.CanonicalCBORHex)
	if err != nil {
		t.Fatalf("decode fixture hex: %v", err)
	}
	switch fixture.ObjectType {
	case "node":
		if _, err := graphcontract.UnmarshalNode(data); !errors.Is(err, graphcontract.ErrInvalidCanonicalCBOR) {
			t.Fatalf("UnmarshalNode error = %v, want ErrInvalidCanonicalCBOR", err)
		}
	case "edge":
		if _, err := graphcontract.UnmarshalEdge(data); !errors.Is(err, graphcontract.ErrInvalidCanonicalCBOR) {
			t.Fatalf("UnmarshalEdge error = %v, want ErrInvalidCanonicalCBOR", err)
		}
	default:
		t.Fatalf("unknown object_type %q", fixture.ObjectType)
	}
}
