package repository

import (
	"bytes"
	"errors"
	"math"
	"reflect"
	"testing"
)

func TestDecodeSchemaTOMLNormalizesRulesAndObjectIdentity(t *testing.T) {
	first := []byte(`
version = 2
global_invariants = ["acyclic", "no-self-loop"]

[[node]]
label = "Requirement"
natural_key = ["external_id", "project"]
natural_key_unique = true
[[node.property]]
key = "project"
required = true
types = ["string"]
[[node.property]]
key = "external_id"
required = true
types = ["integer", "string"]

[[edge]]
type = "DEPENDS_ON"
source_labels = ["Requirement", "Decision"]
target_labels = ["Requirement"]
[edge.cardinality]
source_min = 1
source_max = 3
target_max = 2
[[edge.property]]
key = "reason"
required = true
types = ["string"]
`)
	second := []byte(`
global_invariants=["no-self-loop", "acyclic"]
version=2

[[edge]]
target_labels=["Requirement"]
type="DEPENDS_ON"
source_labels=["Decision", "Requirement"]
[[edge.property]]
types=["string"]
required=true
key="reason"
[edge.cardinality]
target_max=2
source_max=3
source_min=1

[[node]]
natural_key_unique=true
label="Requirement"
natural_key=["project", "external_id"]
[[node.property]]
types=["string", "integer"]
key="external_id"
required=true
[[node.property]]
types=["string"]
required=true
key="project"
`)

	left, err := DecodeSchemaTOML(first)
	if err != nil {
		t.Fatalf("DecodeSchemaTOML first: %v", err)
	}
	right, err := ParseSchemaTOML(second)
	if err != nil {
		t.Fatalf("ParseSchemaTOML second: %v", err)
	}
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("normalized schemas differ:\nleft: %#v\nright: %#v", left, right)
	}
	if got, want := left.NodeRules[0].NaturalKey, []string{"external_id", "project"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("natural key = %#v, want %#v", got, want)
	}
	if got, want := left.NodeRules[0].Properties[0].Types, []PropertyKind{PropertyInteger, PropertyString}; !reflect.DeepEqual(got, want) {
		t.Fatalf("property types = %#v, want %#v", got, want)
	}

	leftEncoding, err := canonicalObjectEncoding(left)
	if err != nil {
		t.Fatalf("encode first schema: %v", err)
	}
	rightEncoding, err := canonicalObjectEncoding(right)
	if err != nil {
		t.Fatalf("encode second schema: %v", err)
	}
	if !bytes.Equal(leftEncoding, rightEncoding) {
		t.Fatal("equivalent schema TOML produced different canonical objects")
	}
	repo := newTestSeedRepository(t)
	if leftID, rightID := repo.store("schema-root", left), repo.store("schema-root", right); leftID != rightID {
		t.Fatalf("equivalent schemas have distinct object IDs: %q != %q", leftID, rightID)
	}
}

func TestDecodeSchemaTOMLRejectsMalformedAndInvalidDefinitions(t *testing.T) {
	tests := []struct {
		name string
		toml string
		want error
	}{
		{
			name: "unknown TOML field",
			toml: "version = 2\npermissve = false\n",
			want: ErrInvalidSchemaTOML,
		},
		{
			name: "unknown property type",
			toml: "version = 2\n[[node]]\nlabel = \"Node\"\n[[node.property]]\nkey = \"priority\"\ntypes = [\"decimal\"]\n",
			want: ErrInvalidSchemaDefinition,
		},
		{
			name: "natural key without opt in",
			toml: "version = 2\n[[node]]\nlabel = \"Node\"\nnatural_key = [\"name\"]\n[[node.property]]\nkey = \"name\"\nrequired = true\ntypes = [\"string\"]\n",
			want: ErrInvalidSchemaDefinition,
		},
		{
			name: "invalid identifier",
			toml: "version = 2\n[[node]]\nlabel = \"../Node\"\n",
			want: ErrInvalidSchemaIdentifier,
		},
		{
			name: "invalid cardinality",
			toml: "version = 2\n[[edge]]\ntype = \"OWNS\"\n[edge.cardinality]\nsource_min = 2\nsource_max = 1\n",
			want: ErrInvalidSchemaDefinition,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeSchemaTOML([]byte(test.toml))
			if !errors.Is(err, test.want) {
				t.Fatalf("DecodeSchemaTOML error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestSchemaAndPropertyIngestionLimits(t *testing.T) {
	if _, err := (MutationOperation{Entity: "node", Labels: []string{"valid\x00label"}}).Normalize(); !errors.Is(err, ErrInvalidSchemaIdentifier) {
		t.Fatalf("NUL label error = %v, want ErrInvalidSchemaIdentifier", err)
	}
	if _, err := (MutationOperation{Entity: "node", Properties: map[string]PropertyValue{"bad/key": StringPropertyValue("value")}}).Normalize(); !errors.Is(err, ErrInvalidSchemaIdentifier) {
		t.Fatalf("property key error = %v, want ErrInvalidSchemaIdentifier", err)
	}
	oversized := StringPropertyValue(string(bytes.Repeat([]byte("x"), MaxPropertyStringLength+1)))
	if _, err := (MutationOperation{
		Entity:     "node",
		Properties: map[string]PropertyValue{"value": oversized},
	}).Normalize(); !errors.Is(err, ErrPropertyValueLimit) {
		t.Fatalf("large string error = %v, want ErrPropertyValueLimit", err)
	}
	deep := NullPropertyValue()
	for i := 0; i <= MaxPropertyDepth; i++ {
		deep = ListPropertyValue([]PropertyValue{deep})
	}
	if _, err := (MutationOperation{Entity: "node", Properties: map[string]PropertyValue{"value": deep}}).Normalize(); !errors.Is(err, ErrPropertyValueLimit) {
		t.Fatalf("deep property error = %v, want ErrPropertyValueLimit", err)
	}
	if _, err := FloatPropertyValue(math.Inf(1)).Normalize(); !errors.Is(err, ErrInvalidPropertyValue) {
		t.Fatalf("infinite float error = %v, want ErrInvalidPropertyValue", err)
	}
}

func TestBuiltinAndLegacySchemaCompatibility(t *testing.T) {
	builtin := BuiltinSchemaSnapshot()
	normalized, err := builtin.Normalize()
	if err != nil {
		t.Fatalf("normalize builtin schema: %v", err)
	}
	if !reflect.DeepEqual(normalized, builtin) {
		t.Fatalf("normalized builtin = %#v, want %#v", normalized, builtin)
	}

	repo := newTestSeedRepository(t)
	legacy := map[string]string{"version": "v1"}
	legacyRoot := repo.store("schema-root", legacy)
	if got, err := repo.schemaSnapshotLocked(legacyRoot); err != nil || !reflect.DeepEqual(got, builtin) {
		t.Fatalf("legacy schema = %#v, %v; want %#v, nil", got, err, builtin)
	}
}
