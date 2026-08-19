package repository

import (
	"errors"
	"reflect"
	"sort"
	"testing"
)

func TestValidateSchemaSnapshotNodeAndEdgeRules(t *testing.T) {
	tests := []struct {
		name   string
		schema SchemaSnapshot
		nodes  map[string]Node
		edges  map[string]Edge
		want   []violationIdentity
	}{
		{
			name:   "node label is declared",
			schema: strictSchema(NodeLabelRule{Label: "User"}),
			nodes: map[string]Node{
				"node": {ID: "node", Labels: []string{"Unknown"}},
			},
			want: []violationIdentity{{SchemaViolationNodeLabel, "node", "node", "Unknown", ""}},
		},
		{
			name: "required node property",
			schema: strictSchema(NodeLabelRule{
				Label: "User", Properties: []PropertyRule{{Key: "email", Required: true, Types: []PropertyKind{PropertyString}}},
			}),
			nodes: map[string]Node{
				"node": {ID: "node", Labels: []string{"User"}},
			},
			want: []violationIdentity{{SchemaViolationRequiredProperty, "node", "node", "User", "email"}},
		},
		{
			name: "typed node property",
			schema: strictSchema(NodeLabelRule{
				Label: "User", Properties: []PropertyRule{{Key: "email", Required: true, Types: []PropertyKind{PropertyString}}},
			}),
			nodes: map[string]Node{
				"node": {ID: "node", Labels: []string{"User"}, Properties: map[string]PropertyValue{"email": IntegerPropertyValue(7)}},
			},
			want: []violationIdentity{{SchemaViolationPropertyType, "node", "node", "User", "email"}},
		},
		{
			name: "edge endpoint labels",
			schema: strictSchema(
				NodeLabelRule{Label: "Source"},
				NodeLabelRule{Label: "Target"},
				NodeLabelRule{Label: "Other"},
				EdgeTypeRule{Type: "LINK", SourceLabels: []string{"Source"}, TargetLabels: []string{"Target"}},
			),
			nodes: map[string]Node{
				"source": {ID: "source", Labels: []string{"Other"}},
				"target": {ID: "target", Labels: []string{"Other"}},
			},
			edges: map[string]Edge{
				"edge": {ID: "edge", Source: "source", Target: "target", Type: "LINK"},
			},
			want: []violationIdentity{
				{SchemaViolationSourceLabel, "edge", "edge", "LINK", "source"},
				{SchemaViolationTargetLabel, "edge", "edge", "LINK", "target"},
			},
		},
		{
			name: "edge type and property rules",
			schema: strictSchema(
				NodeLabelRule{Label: "User"},
				EdgeTypeRule{Type: "LINK", Properties: []PropertyRule{
					{Key: "reason", Required: true, Types: []PropertyKind{PropertyString}},
					{Key: "weight", Types: []PropertyKind{PropertyInteger}},
				}},
			),
			nodes: map[string]Node{
				"source": {ID: "source", Labels: []string{"User"}},
				"target": {ID: "target", Labels: []string{"User"}},
			},
			edges: map[string]Edge{
				"bad-property": {ID: "bad-property", Source: "source", Target: "target", Type: "LINK", Properties: map[string]PropertyValue{"weight": StringPropertyValue("heavy")}},
				"bad-type":     {ID: "bad-type", Source: "source", Target: "target", Type: "UNKNOWN"},
			},
			want: []violationIdentity{
				{SchemaViolationRequiredProperty, "edge", "bad-property", "LINK", "reason"},
				{SchemaViolationPropertyType, "edge", "bad-property", "LINK", "weight"},
				{SchemaViolationEdgeType, "edge", "bad-type", "UNKNOWN", ""},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			violations := validateSchemaViolations(t, test.schema, test.nodes, test.edges)
			if got := violationIdentities(violations); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("violations = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestValidateSchemaSnapshotCardinalityAndNaturalKeys(t *testing.T) {
	tests := []struct {
		name   string
		schema SchemaSnapshot
		nodes  map[string]Node
		edges  map[string]Edge
		want   []violationIdentity
	}{
		{
			name: "source minimum",
			schema: strictSchema(
				NodeLabelRule{Label: "Source"},
				NodeLabelRule{Label: "Target"},
				EdgeTypeRule{Type: "LINK", SourceLabels: []string{"Source"}, TargetLabels: []string{"Target"}, Cardinality: Cardinality{SourceMin: 1}},
			),
			nodes: map[string]Node{
				"source": {ID: "source", Labels: []string{"Source"}},
				"target": {ID: "target", Labels: []string{"Target"}},
			},
			want: []violationIdentity{{SchemaViolationSourceCardinalityMin, "node", "source", "LINK", "source"}},
		},
		{
			name: "source maximum",
			schema: strictSchema(
				NodeLabelRule{Label: "Source"},
				NodeLabelRule{Label: "Target"},
				EdgeTypeRule{Type: "LINK", SourceLabels: []string{"Source"}, TargetLabels: []string{"Target"}, Cardinality: Cardinality{SourceMax: 1}},
			),
			nodes: map[string]Node{
				"source": {ID: "source", Labels: []string{"Source"}},
				"one":    {ID: "one", Labels: []string{"Target"}},
				"two":    {ID: "two", Labels: []string{"Target"}},
			},
			edges: map[string]Edge{
				"one": {ID: "one", Source: "source", Target: "one", Type: "LINK"},
				"two": {ID: "two", Source: "source", Target: "two", Type: "LINK"},
			},
			want: []violationIdentity{{SchemaViolationSourceCardinalityMax, "node", "source", "LINK", "source"}},
		},
		{
			name: "target minimum",
			schema: strictSchema(
				NodeLabelRule{Label: "Source"},
				NodeLabelRule{Label: "Target"},
				EdgeTypeRule{Type: "LINK", SourceLabels: []string{"Source"}, TargetLabels: []string{"Target"}, Cardinality: Cardinality{TargetMin: 1}},
			),
			nodes: map[string]Node{
				"source": {ID: "source", Labels: []string{"Source"}},
				"target": {ID: "target", Labels: []string{"Target"}},
			},
			want: []violationIdentity{{SchemaViolationTargetCardinalityMin, "node", "target", "LINK", "target"}},
		},
		{
			name: "target maximum",
			schema: strictSchema(
				NodeLabelRule{Label: "Source"},
				NodeLabelRule{Label: "Target"},
				EdgeTypeRule{Type: "LINK", SourceLabels: []string{"Source"}, TargetLabels: []string{"Target"}, Cardinality: Cardinality{TargetMax: 1}},
			),
			nodes: map[string]Node{
				"one":    {ID: "one", Labels: []string{"Source"}},
				"two":    {ID: "two", Labels: []string{"Source"}},
				"target": {ID: "target", Labels: []string{"Target"}},
			},
			edges: map[string]Edge{
				"one": {ID: "one", Source: "one", Target: "target", Type: "LINK"},
				"two": {ID: "two", Source: "two", Target: "target", Type: "LINK"},
			},
			want: []violationIdentity{{SchemaViolationTargetCardinalityMax, "node", "target", "LINK", "target"}},
		},
		{
			name: "natural key uniqueness",
			schema: strictSchema(NodeLabelRule{
				Label:            "User",
				NaturalKey:       []string{"email"},
				NaturalKeyUnique: true,
				Properties:       []PropertyRule{{Key: "email", Required: true, Types: []PropertyKind{PropertyString}}},
			}),
			nodes: map[string]Node{
				"first":  {ID: "first", Labels: []string{"User"}, Properties: map[string]PropertyValue{"email": StringPropertyValue("same@example.com")}},
				"second": {ID: "second", Labels: []string{"User"}, Properties: map[string]PropertyValue{"email": StringPropertyValue("same@example.com")}},
			},
			want: []violationIdentity{
				{SchemaViolationNaturalKeyUnique, "node", "first", "User", "email"},
				{SchemaViolationNaturalKeyUnique, "node", "second", "User", "email"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			violations := validateSchemaViolations(t, test.schema, test.nodes, test.edges)
			if got := violationIdentities(violations); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("violations = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestValidateSchemaSnapshotEndpointIntegrityAndGlobalInvariants(t *testing.T) {
	tests := []struct {
		name   string
		schema SchemaSnapshot
		nodes  map[string]Node
		edges  map[string]Edge
		want   []violationIdentity
	}{
		{
			name:   "missing edge endpoints remain invalid for permissive schema",
			schema: BuiltinSchemaSnapshot(),
			edges: map[string]Edge{
				"edge": {ID: "edge", Source: "missing-source", Target: "missing-target", Type: "anything"},
			},
			want: []violationIdentity{
				{SchemaViolationMissingSource, "edge", "edge", "", "source"},
				{SchemaViolationMissingTarget, "edge", "edge", "", "target"},
			},
		},
		{
			name:   "invalid edge still reports missing endpoints",
			schema: BuiltinSchemaSnapshot(),
			edges: map[string]Edge{
				"edge": {
					ID: "edge", Source: "missing-source", Target: "missing-target",
					Properties: map[string]PropertyValue{"bad": {Kind: "unknown"}},
				},
			},
			want: []violationIdentity{
				{SchemaViolationInvalidEdge, "edge", "edge", "", ""},
				{SchemaViolationMissingSource, "edge", "edge", "", "source"},
				{SchemaViolationMissingTarget, "edge", "edge", "", "target"},
			},
		},
		{
			name: "acyclic and no self loop",
			schema: SchemaSnapshot{
				Version:          2,
				GlobalInvariants: []GlobalInvariant{GlobalInvariantAcyclic, GlobalInvariantNoSelfLoop},
				EdgeRules:        []EdgeTypeRule{{Type: "LINK"}},
			},
			nodes: map[string]Node{
				"a": {ID: "a"},
				"b": {ID: "b"},
			},
			edges: map[string]Edge{
				"first":  {ID: "first", Source: "a", Target: "b", Type: "LINK"},
				"second": {ID: "second", Source: "b", Target: "a", Type: "LINK"},
				"self":   {ID: "self", Source: "a", Target: "a", Type: "LINK"},
			},
			want: []violationIdentity{
				{SchemaViolationAcyclic, "edge", "first", "acyclic", ""},
				{SchemaViolationAcyclic, "edge", "second", "acyclic", ""},
				{SchemaViolationAcyclic, "edge", "self", "acyclic", ""},
				{SchemaViolationNoSelfLoop, "edge", "self", "no-self-loop", ""},
			},
		},
		{
			name:   "invalid entity normalization and identity",
			schema: BuiltinSchemaSnapshot(),
			nodes: map[string]Node{
				"node": {ID: "different", Labels: []string{"bad\x00label"}},
			},
			edges: map[string]Edge{
				"edge": {ID: "different", Source: "node", Target: "node", Type: "valid"},
			},
			want: []violationIdentity{
				{SchemaViolationEdgeID, "edge", "edge", "", "id"},
				{SchemaViolationNodeID, "node", "node", "", "id"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			violations := validateSchemaViolations(t, test.schema, test.nodes, test.edges)
			if got := violationIdentities(violations); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("violations = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestValidateSchemaSnapshotIsDeterministicAndPermissive(t *testing.T) {
	permissiveNodes := map[string]Node{
		"first":  {ID: "first", Labels: []string{"Undeclared"}, Properties: map[string]PropertyValue{"value": ListPropertyValue([]PropertyValue{BoolPropertyValue(true)})}},
		"second": {ID: "second"},
	}
	permissiveEdges := map[string]Edge{
		"edge": {ID: "edge", Source: "first", Target: "second", Type: "UNDECLARED", Properties: map[string]PropertyValue{"value": MapPropertyValue(map[string]PropertyValue{"key": StringPropertyValue("value")})}},
	}
	if err := ValidateSchemaSnapshot(BuiltinSchemaSnapshot(), permissiveNodes, permissiveEdges); err != nil {
		t.Fatalf("ValidateSchemaSnapshot permissive schema: %v", err)
	}

	schema := strictSchema(NodeLabelRule{
		Label:            "User",
		NaturalKey:       []string{"email"},
		NaturalKeyUnique: true,
		Properties:       []PropertyRule{{Key: "email", Required: true, Types: []PropertyKind{PropertyString}}},
	})
	nodes := map[string]Node{
		"zulu":  {ID: "zulu", Labels: []string{"User"}},
		"alpha": {ID: "alpha", Labels: []string{"Unknown"}},
		"bravo": {ID: "bravo", Labels: []string{"User"}},
	}

	var want []SchemaViolation
	for attempt := 0; attempt < 20; attempt++ {
		got := validateSchemaViolations(t, schema, nodes, nil)
		if !sort.IsSorted(schemaViolationSlice(got)) {
			t.Fatalf("violations are not lexically sorted: %#v", got)
		}
		if attempt == 0 {
			want = got
		} else if !reflect.DeepEqual(got, want) {
			t.Fatalf("attempt %d violations = %#v, want %#v", attempt, got, want)
		}
	}
}

func TestSchemaNormalizationRejectsUnknownGlobalInvariant(t *testing.T) {
	_, err := DecodeSchemaTOML([]byte("version = 2\nglobal_invariants = [\"unknown\"]\n"))
	if !errors.Is(err, ErrInvalidSchemaDefinition) {
		t.Fatalf("DecodeSchemaTOML error = %v, want ErrInvalidSchemaDefinition", err)
	}
}

type violationIdentity struct {
	Code     SchemaViolationCode
	Entity   string
	EntityID string
	Rule     string
	Field    string
}

func violationIdentities(violations []SchemaViolation) []violationIdentity {
	result := make([]violationIdentity, len(violations))
	for i, violation := range violations {
		result[i] = violationIdentity{
			Code: violation.Code, Entity: violation.Entity, EntityID: violation.EntityID,
			Rule: violation.Rule, Field: violation.Field,
		}
	}
	return result
}

func validateSchemaViolations(t *testing.T, schema SchemaSnapshot, nodes map[string]Node, edges map[string]Edge) []SchemaViolation {
	t.Helper()
	err := ValidateSchemaSnapshot(schema, nodes, edges)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrSchemaValidation) {
		t.Fatalf("ValidateSchemaSnapshot error = %v, want ErrSchemaValidation", err)
	}
	var validationError *SchemaValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf("ValidateSchemaSnapshot error = %T, want *SchemaValidationError", err)
	}
	return validationError.Violations
}

func strictSchema(parts ...any) SchemaSnapshot {
	schema := SchemaSnapshot{Version: 2}
	for _, part := range parts {
		switch part := part.(type) {
		case NodeLabelRule:
			schema.NodeRules = append(schema.NodeRules, part)
		case EdgeTypeRule:
			schema.EdgeRules = append(schema.EdgeRules, part)
		default:
			panic("unsupported schema part")
		}
	}
	return schema
}

type schemaViolationSlice []SchemaViolation

func (violations schemaViolationSlice) Len() int { return len(violations) }
func (violations schemaViolationSlice) Swap(i, j int) {
	violations[i], violations[j] = violations[j], violations[i]
}
func (violations schemaViolationSlice) Less(i, j int) bool {
	left, right := violations[i], violations[j]
	switch {
	case left.Entity != right.Entity:
		return left.Entity < right.Entity
	case left.EntityID != right.EntityID:
		return left.EntityID < right.EntityID
	case left.Rule != right.Rule:
		return left.Rule < right.Rule
	case left.Field != right.Field:
		return left.Field < right.Field
	case left.Code != right.Code:
		return left.Code < right.Code
	case left.Expected != right.Expected:
		return left.Expected < right.Expected
	default:
		return left.Actual < right.Actual
	}
}
