package graphcontract_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/autonomous-bits/spool/graphcontract"
)

const schemaFixtureFormatVersion = 1

type schemaFixture struct {
	FormatVersion int                             `json:"format_version"`
	Candidate     string                          `json:"candidate"`
	Schema        string                          `json:"schema"`
	Nodes         map[string]graphcontract.Node   `json:"nodes"`
	Edges         map[string]graphcontract.Edge   `json:"edges"`
	Violations    []graphcontract.SchemaViolation `json:"violations"`
}

func TestSchemaInteroperabilityFixtures(t *testing.T) {
	fixtureRoot := filepath.Join("testdata", "schema", "v1")
	fixturePaths, err := filepath.Glob(filepath.Join(fixtureRoot, "*.json"))
	if err != nil {
		t.Fatalf("list fixtures: %v", err)
	}
	if len(fixturePaths) == 0 {
		t.Fatal("no schema interoperability fixtures")
	}
	for _, path := range fixturePaths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			var fixture schemaFixture
			if err := json.Unmarshal(data, &fixture); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			if fixture.FormatVersion != schemaFixtureFormatVersion {
				t.Fatalf("format version = %d, want %d", fixture.FormatVersion, schemaFixtureFormatVersion)
			}
			if fixture.Candidate != "graph" && fixture.Candidate != "migration" && fixture.Candidate != "merge" {
				t.Fatalf("candidate = %q, want graph, migration, or merge", fixture.Candidate)
			}
			schemaData, err := os.ReadFile(filepath.Join(fixtureRoot, fixture.Schema))
			if err != nil {
				t.Fatalf("read schema: %v", err)
			}
			schema, err := graphcontract.DecodeSchemaTOML(schemaData)
			if err != nil {
				t.Fatalf("decode schema: %v", err)
			}
			err = graphcontract.ValidateSchemaSnapshot(schema, fixture.Nodes, fixture.Edges)
			if len(fixture.Violations) == 0 {
				if err != nil {
					t.Fatalf("validate valid %s candidate: %v", fixture.Candidate, err)
				}
				return
			}
			if !errors.Is(err, graphcontract.ErrSchemaValidation) {
				t.Fatalf("validation error = %v, want ErrSchemaValidation", err)
			}
			var validationError *graphcontract.SchemaValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf("validation error = %T, want *SchemaValidationError", err)
			}
			if !reflect.DeepEqual(validationError.Violations, fixture.Violations) {
				t.Fatalf("violations = %#v, want %#v", validationError.Violations, fixture.Violations)
			}
		})
	}
}

func TestInvalidSchemaInteroperabilityFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "schema", "v1", "invalid-schema.toml"))
	if err != nil {
		t.Fatalf("read invalid schema fixture: %v", err)
	}
	if _, err := graphcontract.DecodeSchemaTOML(data); !errors.Is(err, graphcontract.ErrInvalidSchemaTOML) || !errors.Is(err, graphcontract.ErrInvalidSchemaDefinition) {
		t.Fatalf("decode invalid schema error = %v, want invalid TOML and schema definition", err)
	}
}

func TestSchemaSnapshotCanonicalCBORRoundTrip(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "schema", "v1", "schema.toml"))
	if err != nil {
		t.Fatalf("read schema fixture: %v", err)
	}
	schema, err := graphcontract.DecodeSchemaTOML(data)
	if err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	encoded, err := graphcontract.MarshalSchemaSnapshot(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	decoded, err := graphcontract.UnmarshalSchemaSnapshot(encoded)
	if err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	if !reflect.DeepEqual(decoded, schema) {
		t.Fatalf("decoded schema = %#v, want %#v", decoded, schema)
	}
}
