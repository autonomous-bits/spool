package graphcontract

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/fxamacker/cbor/v2"
	"github.com/pelletier/go-toml/v2"
)

var (
	// ErrInvalidSchemaTOML reports malformed TOML or TOML that does not match
	// the schema authoring format.
	ErrInvalidSchemaTOML = errors.New("invalid schema TOML")
	// ErrInvalidSchemaSnapshot reports a schema snapshot without a version.
	ErrInvalidSchemaSnapshot = errors.New("invalid schema snapshot")
	// ErrInvalidSchemaDefinition reports inconsistent or unsupported schema rules.
	ErrInvalidSchemaDefinition = errors.New("invalid schema definition")
	// ErrInvalidSchemaIdentifier reports an invalid schema identifier.
	ErrInvalidSchemaIdentifier = errors.New("invalid schema identifier")
	// ErrSchemaValidation reports graph contents that do not satisfy a schema.
	ErrSchemaValidation = errors.New("schema validation failed")
)

const (
	// BuiltinSchemaVersion is the version of the initial permissive schema.
	BuiltinSchemaVersion uint16 = 1
	// GlobalInvariantAcyclic requires the directed graph to contain no cycles.
	GlobalInvariantAcyclic GlobalInvariant = "acyclic"
	// GlobalInvariantNoSelfLoop disallows edges whose endpoints are identical.
	GlobalInvariantNoSelfLoop GlobalInvariant = "no-self-loop"
	// UniversalModifierLabel identifies the intrinsic cross-cutting modifier label.
	UniversalModifierLabel = "Ephemeral"
)

// SchemaSnapshot is the canonical schema object referenced by a graph snapshot.
// Version one is retained as the permissive built-in schema; later versions may
// declare node, edge, and graph-wide validation rules.
type SchemaSnapshot struct {
	Version          uint16            `json:"version" cbor:"0,keyasint"`
	Permissive       bool              `json:"permissive" cbor:"1,keyasint"`
	NodeRules        []NodeLabelRule   `json:"nodeRules,omitempty" cbor:"2,keyasint,omitempty"`
	EdgeRules        []EdgeTypeRule    `json:"edgeRules,omitempty" cbor:"3,keyasint,omitempty"`
	GlobalInvariants []GlobalInvariant `json:"globalInvariants,omitempty" cbor:"4,keyasint,omitempty"`
}

type schemaSnapshotCBOR SchemaSnapshot

// NodeLabelRule defines constraints for nodes carrying Label.
type NodeLabelRule struct {
	Label            string         `json:"label" cbor:"1,keyasint"`
	Properties       []PropertyRule `json:"properties,omitempty" cbor:"2,keyasint,omitempty"`
	NaturalKey       []string       `json:"naturalKey,omitempty" cbor:"3,keyasint,omitempty"`
	NaturalKeyUnique bool           `json:"naturalKeyUnique,omitempty" cbor:"4,keyasint,omitempty"`
}

// EdgeTypeRule defines constraints for edges of Type.
type EdgeTypeRule struct {
	Type         string         `json:"type" cbor:"1,keyasint"`
	Properties   []PropertyRule `json:"properties,omitempty" cbor:"2,keyasint,omitempty"`
	SourceLabels []string       `json:"sourceLabels,omitempty" cbor:"3,keyasint,omitempty"`
	TargetLabels []string       `json:"targetLabels,omitempty" cbor:"4,keyasint,omitempty"`
	Cardinality  Cardinality    `json:"cardinality" cbor:"5,keyasint"`
}

// PropertyRule defines whether a property is required, indexed, and its allowed value kinds.
type PropertyRule struct {
	Key      string         `json:"key" cbor:"1,keyasint"`
	Required bool           `json:"required" cbor:"2,keyasint"`
	Types    []PropertyKind `json:"types" cbor:"3,keyasint"`
	Indexed  bool           `json:"indexed,omitempty" cbor:"4,keyasint,omitempty"`
}

// Cardinality bounds incoming and outgoing edges of an edge type. A maximum of
// zero is unbounded.
type Cardinality struct {
	SourceMin uint32 `json:"sourceMin,omitempty" cbor:"1,keyasint,omitempty"`
	SourceMax uint32 `json:"sourceMax,omitempty" cbor:"2,keyasint,omitempty"`
	TargetMin uint32 `json:"targetMin,omitempty" cbor:"3,keyasint,omitempty"`
	TargetMax uint32 `json:"targetMax,omitempty" cbor:"4,keyasint,omitempty"`
}

// GlobalInvariant names a graph-wide invariant enforced by a schema validator.
type GlobalInvariant string

// SchemaViolationCode identifies the kind of failed schema constraint.
type SchemaViolationCode string

const (
	SchemaViolationInvalidNode          SchemaViolationCode = "invalid-node"
	SchemaViolationInvalidEdge          SchemaViolationCode = "invalid-edge"
	SchemaViolationNodeID               SchemaViolationCode = "node-id"
	SchemaViolationEdgeID               SchemaViolationCode = "edge-id"
	SchemaViolationNodeLabel            SchemaViolationCode = "node-label"
	SchemaViolationEdgeType             SchemaViolationCode = "edge-type"
	SchemaViolationRequiredProperty     SchemaViolationCode = "required-property"
	SchemaViolationPropertyType         SchemaViolationCode = "property-type"
	SchemaViolationMissingSource        SchemaViolationCode = "missing-source"
	SchemaViolationMissingTarget        SchemaViolationCode = "missing-target"
	SchemaViolationSourceLabel          SchemaViolationCode = "source-label"
	SchemaViolationTargetLabel          SchemaViolationCode = "target-label"
	SchemaViolationSourceCardinalityMin SchemaViolationCode = "source-cardinality-min"
	SchemaViolationSourceCardinalityMax SchemaViolationCode = "source-cardinality-max"
	SchemaViolationTargetCardinalityMin SchemaViolationCode = "target-cardinality-min"
	SchemaViolationTargetCardinalityMax SchemaViolationCode = "target-cardinality-max"
	SchemaViolationNaturalKeyUnique     SchemaViolationCode = "natural-key-unique"
	SchemaViolationAcyclic              SchemaViolationCode = "acyclic"
	SchemaViolationNoSelfLoop           SchemaViolationCode = "no-self-loop"
)

// SchemaViolation is one stable, machine-readable failed graph constraint.
type SchemaViolation struct {
	Code     SchemaViolationCode `json:"code"`
	Entity   string              `json:"entity"`
	EntityID string              `json:"entityID"`
	Rule     string              `json:"rule,omitempty"`
	Field    string              `json:"field,omitempty"`
	Expected string              `json:"expected,omitempty"`
	Actual   string              `json:"actual,omitempty"`
}

// SchemaValidationError contains every violation found while validating a
// materialized graph. Violations are sorted lexically for stable previews.
type SchemaValidationError struct {
	Violations []SchemaViolation
}

func (e *SchemaValidationError) Error() string {
	return fmt.Sprintf("%s: %d violation(s)", ErrSchemaValidation, len(e.Violations))
}

func (e *SchemaValidationError) Unwrap() error { return ErrSchemaValidation }

// MarshalCBOR returns the normalized, canonical CBOR encoding of s.
func (s SchemaSnapshot) MarshalCBOR() ([]byte, error) {
	normalized, err := s.Normalize()
	if err != nil {
		return nil, err
	}
	return canonicalCBOR.Marshal(schemaSnapshotCBOR(normalized))
}

// MarshalSchemaSnapshot returns the normalized, canonical CBOR encoding of s.
func MarshalSchemaSnapshot(s SchemaSnapshot) ([]byte, error) { return s.MarshalCBOR() }

// UnmarshalSchemaSnapshot decodes and verifies canonical CBOR for a schema snapshot.
func UnmarshalSchemaSnapshot(data []byte) (SchemaSnapshot, error) {
	var schema schemaSnapshotCBOR
	if err := cbor.Unmarshal(data, &schema); err != nil {
		return SchemaSnapshot{}, fmt.Errorf("%w: decode schema snapshot: %v", ErrInvalidCanonicalCBOR, err)
	}
	normalized, err := SchemaSnapshot(schema).Normalize()
	if err != nil {
		return SchemaSnapshot{}, err
	}
	canonical, err := normalized.MarshalCBOR()
	if err != nil || !bytes.Equal(data, canonical) {
		return SchemaSnapshot{}, fmt.Errorf("%w: schema snapshot", ErrInvalidCanonicalCBOR)
	}
	return normalized, nil
}

// BuiltinSchemaSnapshot returns the built-in versioned permissive schema.
func BuiltinSchemaSnapshot() SchemaSnapshot {
	return SchemaSnapshot{Version: BuiltinSchemaVersion, Permissive: true}
}

// Normalize validates and canonicalizes a schema snapshot.
func (s SchemaSnapshot) Normalize() (SchemaSnapshot, error) {
	if s.Version == 0 {
		return SchemaSnapshot{}, ErrInvalidSchemaSnapshot
	}
	if s.Permissive && (len(s.NodeRules) != 0 || len(s.EdgeRules) != 0 || len(s.GlobalInvariants) != 0) {
		return SchemaSnapshot{}, fmt.Errorf("%w: permissive schemas cannot declare rules", ErrInvalidSchemaDefinition)
	}
	normalized := SchemaSnapshot{Version: s.Version, Permissive: s.Permissive}
	if len(s.NodeRules) != 0 {
		normalized.NodeRules = make([]NodeLabelRule, len(s.NodeRules))
		for i, rule := range s.NodeRules {
			var err error
			if normalized.NodeRules[i], err = rule.normalize(); err != nil {
				return SchemaSnapshot{}, fmt.Errorf("%w: node rule %d: %w", ErrInvalidSchemaDefinition, i, err)
			}
		}
		sort.Slice(normalized.NodeRules, func(i, j int) bool { return normalized.NodeRules[i].Label < normalized.NodeRules[j].Label })
		for i := 1; i < len(normalized.NodeRules); i++ {
			if normalized.NodeRules[i-1].Label == normalized.NodeRules[i].Label {
				return SchemaSnapshot{}, fmt.Errorf("%w: duplicate node label %q", ErrInvalidSchemaDefinition, normalized.NodeRules[i].Label)
			}
		}
	}
	if len(s.EdgeRules) != 0 {
		normalized.EdgeRules = make([]EdgeTypeRule, len(s.EdgeRules))
		for i, rule := range s.EdgeRules {
			var err error
			if normalized.EdgeRules[i], err = rule.normalize(); err != nil {
				return SchemaSnapshot{}, fmt.Errorf("%w: edge rule %d: %w", ErrInvalidSchemaDefinition, i, err)
			}
		}
		sort.Slice(normalized.EdgeRules, func(i, j int) bool { return normalized.EdgeRules[i].Type < normalized.EdgeRules[j].Type })
		for i := 1; i < len(normalized.EdgeRules); i++ {
			if normalized.EdgeRules[i-1].Type == normalized.EdgeRules[i].Type {
				return SchemaSnapshot{}, fmt.Errorf("%w: duplicate edge type %q", ErrInvalidSchemaDefinition, normalized.EdgeRules[i].Type)
			}
		}
	}
	if len(s.GlobalInvariants) != 0 {
		normalized.GlobalInvariants = append([]GlobalInvariant(nil), s.GlobalInvariants...)
		sort.Slice(normalized.GlobalInvariants, func(i, j int) bool { return normalized.GlobalInvariants[i] < normalized.GlobalInvariants[j] })
		for i, invariant := range normalized.GlobalInvariants {
			if err := validateIdentifier(string(invariant), 128, "global invariant"); err != nil {
				return SchemaSnapshot{}, err
			}
			if !supportedGlobalInvariant(invariant) {
				return SchemaSnapshot{}, fmt.Errorf("%w: unsupported global invariant %q", ErrInvalidSchemaDefinition, invariant)
			}
			if i > 0 && normalized.GlobalInvariants[i-1] == invariant {
				return SchemaSnapshot{}, fmt.Errorf("%w: duplicate global invariant %q", ErrInvalidSchemaDefinition, invariant)
			}
		}
	}
	return normalized, nil
}

func (r NodeLabelRule) normalize() (NodeLabelRule, error) {
	if err := validateIdentifier(r.Label, 128, "node label"); err != nil {
		return NodeLabelRule{}, err
	}
	properties, err := normalizePropertyRules(r.Properties)
	if err != nil {
		return NodeLabelRule{}, err
	}
	normalized := NodeLabelRule{Label: r.Label, Properties: properties, NaturalKeyUnique: r.NaturalKeyUnique}
	if len(r.NaturalKey) == 0 {
		if r.NaturalKeyUnique {
			return NodeLabelRule{}, fmt.Errorf("%w: natural-key uniqueness requires a natural key", ErrInvalidSchemaDefinition)
		}
		return normalized, nil
	}
	if !r.NaturalKeyUnique {
		return NodeLabelRule{}, fmt.Errorf("%w: natural key must explicitly opt into uniqueness", ErrInvalidSchemaDefinition)
	}
	normalized.NaturalKey = normalizeIdentifiers(r.NaturalKey)
	for i, key := range normalized.NaturalKey {
		if err := validateIdentifier(key, 128, "property key"); err != nil {
			return NodeLabelRule{}, err
		}
		if i > 0 && normalized.NaturalKey[i-1] == key {
			return NodeLabelRule{}, fmt.Errorf("%w: duplicate natural-key property %q", ErrInvalidSchemaDefinition, key)
		}
		found := false
		for _, property := range properties {
			if property.Key == key && property.Required {
				found = true
				break
			}
		}
		if !found {
			return NodeLabelRule{}, fmt.Errorf("%w: natural-key property %q must be required", ErrInvalidSchemaDefinition, key)
		}
	}
	return normalized, nil
}

func (r EdgeTypeRule) normalize() (EdgeTypeRule, error) {
	if err := validateIdentifier(r.Type, 128, "edge type"); err != nil {
		return EdgeTypeRule{}, err
	}
	properties, err := normalizePropertyRules(r.Properties)
	if err != nil {
		return EdgeTypeRule{}, err
	}
	if (r.Cardinality.SourceMax != 0 && r.Cardinality.SourceMin > r.Cardinality.SourceMax) ||
		(r.Cardinality.TargetMax != 0 && r.Cardinality.TargetMin > r.Cardinality.TargetMax) {
		return EdgeTypeRule{}, fmt.Errorf("%w: cardinality minimum exceeds maximum", ErrInvalidSchemaDefinition)
	}
	normalized := EdgeTypeRule{Type: r.Type, Properties: properties, Cardinality: r.Cardinality}
	normalized.SourceLabels = normalizeIdentifiers(r.SourceLabels)
	normalized.TargetLabels = normalizeIdentifiers(r.TargetLabels)
	for _, labels := range [][]string{normalized.SourceLabels, normalized.TargetLabels} {
		for i, label := range labels {
			if err := validateIdentifier(label, 128, "label"); err != nil {
				return EdgeTypeRule{}, err
			}
			if i > 0 && labels[i-1] == label {
				return EdgeTypeRule{}, fmt.Errorf("%w: duplicate edge endpoint label %q", ErrInvalidSchemaDefinition, label)
			}
		}
	}
	return normalized, nil
}

func normalizePropertyRules(rules []PropertyRule) ([]PropertyRule, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	normalized := make([]PropertyRule, len(rules))
	for i, rule := range rules {
		if err := validateIdentifier(rule.Key, 128, "property key"); err != nil {
			return nil, err
		}
		if len(rule.Types) == 0 {
			return nil, fmt.Errorf("%w: property %q has no allowed types", ErrInvalidSchemaDefinition, rule.Key)
		}
		types := append([]PropertyKind(nil), rule.Types...)
		sort.Slice(types, func(i, j int) bool { return types[i] < types[j] })
		for j, kind := range types {
			if !validPropertyKind(kind) {
				return nil, fmt.Errorf("%w: property %q type %q", ErrInvalidSchemaDefinition, rule.Key, kind)
			}
			if j > 0 && types[j-1] == kind {
				return nil, fmt.Errorf("%w: property %q repeats type %q", ErrInvalidSchemaDefinition, rule.Key, kind)
			}
		}
		if rule.Indexed {
			for _, kind := range types {
				if kind != PropertyString && kind != PropertyInteger && kind != PropertyFloat {
					return nil, fmt.Errorf("%w: indexed property %q must have only scalar string or number types", ErrInvalidSchemaDefinition, rule.Key)
				}
			}
		}
		normalized[i] = PropertyRule{Key: rule.Key, Required: rule.Required, Types: types, Indexed: rule.Indexed}
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].Key < normalized[j].Key })
	for i := 1; i < len(normalized); i++ {
		if normalized[i-1].Key == normalized[i].Key {
			return nil, fmt.Errorf("%w: duplicate property rule %q", ErrInvalidSchemaDefinition, normalized[i].Key)
		}
	}
	return normalized, nil
}

func validPropertyKind(kind PropertyKind) bool {
	switch kind {
	case PropertyNull, PropertyBool, PropertyInteger, PropertyFloat, PropertyString, PropertyList, PropertyMap:
		return true
	default:
		return false
	}
}

func normalizeIdentifiers(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := append([]string(nil), values...)
	sort.Strings(normalized)
	return normalized
}

func validateIdentifier(value string, maximum int, kind string) error {
	if value == "" || !utf8.ValidString(value) || len(value) > maximum {
		return fmt.Errorf("%w: %s is empty, invalid UTF-8, or exceeds %d bytes", ErrInvalidSchemaIdentifier, kind, maximum)
	}
	if strings.ContainsAny(value, `/\`) {
		return fmt.Errorf("%w: %s contains a path separator", ErrInvalidSchemaIdentifier, kind)
	}
	if strings.IndexByte(value, 0) >= 0 {
		return fmt.Errorf("%w: %s contains NUL", ErrInvalidSchemaIdentifier, kind)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("%w: %s contains a control character", ErrInvalidSchemaIdentifier, kind)
		}
	}
	return nil
}

// DecodeSchemaTOML decodes a schema definition from TOML and returns its
// normalized canonical representation. Unknown keys are rejected.
func DecodeSchemaTOML(data []byte) (SchemaSnapshot, error) {
	return DecodeSchemaTOMLReader(bytes.NewReader(data))
}

// ParseSchemaTOML is an alias for DecodeSchemaTOML.
func ParseSchemaTOML(data []byte) (SchemaSnapshot, error) { return DecodeSchemaTOML(data) }

// DecodeSchemaTOMLReader decodes a schema definition from a TOML stream.
func DecodeSchemaTOMLReader(reader io.Reader) (SchemaSnapshot, error) {
	var document tomlSchemaDocument
	decoder := toml.NewDecoder(reader).DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return SchemaSnapshot{}, fmt.Errorf("%w: %w", ErrInvalidSchemaTOML, err)
	}
	schema := SchemaSnapshot{
		Version: document.Version, Permissive: document.Permissive,
		GlobalInvariants: make([]GlobalInvariant, len(document.GlobalInvariants)),
		NodeRules:        make([]NodeLabelRule, len(document.Nodes)), EdgeRules: make([]EdgeTypeRule, len(document.Edges)),
	}
	for i, invariant := range document.GlobalInvariants {
		schema.GlobalInvariants[i] = GlobalInvariant(invariant)
	}
	for i, node := range document.Nodes {
		schema.NodeRules[i] = NodeLabelRule{Label: node.Label, Properties: decodePropertyRules(node.Properties), NaturalKey: node.NaturalKey, NaturalKeyUnique: node.NaturalKeyUnique}
	}
	for i, edge := range document.Edges {
		schema.EdgeRules[i] = EdgeTypeRule{
			Type: edge.Type, Properties: decodePropertyRules(edge.Properties), SourceLabels: edge.SourceLabels, TargetLabels: edge.TargetLabels,
			Cardinality: Cardinality{SourceMin: edge.Cardinality.SourceMin, SourceMax: edge.Cardinality.SourceMax, TargetMin: edge.Cardinality.TargetMin, TargetMax: edge.Cardinality.TargetMax},
		}
	}
	normalized, err := schema.Normalize()
	if err != nil {
		return SchemaSnapshot{}, fmt.Errorf("%w: %w", ErrInvalidSchemaTOML, err)
	}
	return normalized, nil
}

type tomlSchemaDocument struct {
	Version          uint16         `toml:"version"`
	Permissive       bool           `toml:"permissive"`
	GlobalInvariants []string       `toml:"global_invariants"`
	Nodes            []tomlNodeRule `toml:"node"`
	Edges            []tomlEdgeRule `toml:"edge"`
}

type tomlNodeRule struct {
	Label            string             `toml:"label"`
	NaturalKey       []string           `toml:"natural_key"`
	NaturalKeyUnique bool               `toml:"natural_key_unique"`
	Properties       []tomlPropertyRule `toml:"property"`
}

type tomlEdgeRule struct {
	Type         string             `toml:"type"`
	SourceLabels []string           `toml:"source_labels"`
	TargetLabels []string           `toml:"target_labels"`
	Cardinality  tomlCardinality    `toml:"cardinality"`
	Properties   []tomlPropertyRule `toml:"property"`
}

type tomlCardinality struct {
	SourceMin uint32 `toml:"source_min"`
	SourceMax uint32 `toml:"source_max"`
	TargetMin uint32 `toml:"target_min"`
	TargetMax uint32 `toml:"target_max"`
}

type tomlPropertyRule struct {
	Key      string   `toml:"key"`
	Required bool     `toml:"required"`
	Types    []string `toml:"types"`
	Indexed  bool     `toml:"indexed"`
}

func decodePropertyRules(rules []tomlPropertyRule) []PropertyRule {
	if len(rules) == 0 {
		return nil
	}
	decoded := make([]PropertyRule, len(rules))
	for i, rule := range rules {
		kinds := make([]PropertyKind, len(rule.Types))
		for j, kind := range rule.Types {
			kinds[j] = PropertyKind(kind)
		}
		decoded[i] = PropertyRule{Key: rule.Key, Required: rule.Required, Types: kinds, Indexed: rule.Indexed}
	}
	return decoded
}

// ValidateSchemaSnapshot checks fully materialized graph entities against
// schema. It never mutates nodes or edges.
func ValidateSchemaSnapshot(schema SchemaSnapshot, nodes map[string]Node, edges map[string]Edge) error {
	normalizedSchema, err := schema.Normalize()
	if err != nil {
		return err
	}
	validator := schemaValidator{schema: normalizedSchema, nodes: nodes, edges: edges}
	validator.validate()
	if len(validator.violations) == 0 {
		return nil
	}
	sortSchemaViolations(validator.violations)
	return &SchemaValidationError{Violations: validator.violations}
}

type schemaValidator struct {
	schema     SchemaSnapshot
	nodes      map[string]Node
	edges      map[string]Edge
	nodeRules  map[string]NodeLabelRule
	edgeRules  map[string]EdgeTypeRule
	nodesByID  map[string]Node
	edgesByID  map[string]Edge
	violations []SchemaViolation
}

func (v *schemaValidator) validate() {
	v.nodeRules = make(map[string]NodeLabelRule, len(v.schema.NodeRules))
	for _, rule := range v.schema.NodeRules {
		v.nodeRules[rule.Label] = rule
	}
	v.edgeRules = make(map[string]EdgeTypeRule, len(v.schema.EdgeRules))
	for _, rule := range v.schema.EdgeRules {
		v.edgeRules[rule.Type] = rule
	}
	v.normalizeEntities()
	v.validateEndpoints()
	if !v.schema.Permissive {
		v.validateNodeRules()
		v.validateEdgeRules()
		v.validateNaturalKeys()
	}
	v.validateGlobalInvariants()
}

func (v *schemaValidator) normalizeEntities() {
	v.nodesByID = make(map[string]Node, len(v.nodes))
	for _, id := range sortedNodeIDs(v.nodes) {
		node := v.nodes[id]
		if id == "" || node.ID != id {
			v.add(SchemaViolation{Code: SchemaViolationNodeID, Entity: "node", EntityID: id, Field: "id", Expected: id, Actual: node.ID})
		}
		normalized, err := node.Normalize()
		if err != nil {
			v.add(SchemaViolation{Code: SchemaViolationInvalidNode, Entity: "node", EntityID: id, Expected: "normalized node"})
			continue
		}
		v.nodesByID[id] = normalized
	}
	v.edgesByID = make(map[string]Edge, len(v.edges))
	for _, id := range sortedEdgeIDs(v.edges) {
		edge := v.edges[id]
		if id == "" || edge.ID != id {
			v.add(SchemaViolation{Code: SchemaViolationEdgeID, Entity: "edge", EntityID: id, Field: "id", Expected: id, Actual: edge.ID})
		}
		normalized, err := edge.Normalize()
		if err != nil {
			v.add(SchemaViolation{Code: SchemaViolationInvalidEdge, Entity: "edge", EntityID: id, Expected: "normalized edge"})
			continue
		}
		v.edgesByID[id] = normalized
	}
}

func (v *schemaValidator) validateEndpoints() {
	for _, id := range sortedEdgeIDs(v.edges) {
		edge := v.edges[id]
		if _, exists := v.nodes[edge.Source]; !exists {
			v.add(SchemaViolation{Code: SchemaViolationMissingSource, Entity: "edge", EntityID: id, Field: "source", Expected: "existing node", Actual: edge.Source})
		}
		if _, exists := v.nodes[edge.Target]; !exists {
			v.add(SchemaViolation{Code: SchemaViolationMissingTarget, Entity: "edge", EntityID: id, Field: "target", Expected: "existing node", Actual: edge.Target})
		}
	}
}

func (v *schemaValidator) validateNodeRules() {
	for _, id := range sortedNodeIDs(v.nodesByID) {
		node := v.nodesByID[id]
		for _, label := range node.Labels {
			rule, exists := v.nodeRules[label]
			if !exists {
				if label == UniversalModifierLabel {
					continue
				}
				v.add(SchemaViolation{Code: SchemaViolationNodeLabel, Entity: "node", EntityID: id, Rule: label, Expected: "declared node label", Actual: label})
				continue
			}
			v.validateProperties("node", id, rule.Label, node.Properties, rule.Properties)
		}
	}
}

func (v *schemaValidator) validateEdgeRules() {
	for _, id := range sortedEdgeIDs(v.edgesByID) {
		edge := v.edgesByID[id]
		rule, exists := v.edgeRules[edge.Type]
		if !exists {
			v.add(SchemaViolation{Code: SchemaViolationEdgeType, Entity: "edge", EntityID: id, Rule: edge.Type, Expected: "declared edge type", Actual: edge.Type})
			continue
		}
		v.validateProperties("edge", id, rule.Type, edge.Properties, rule.Properties)
		if source, exists := v.nodesByID[edge.Source]; exists && !hasAnyLabel(source, rule.SourceLabels) {
			v.add(SchemaViolation{Code: SchemaViolationSourceLabel, Entity: "edge", EntityID: id, Rule: rule.Type, Field: "source", Expected: strings.Join(rule.SourceLabels, ","), Actual: strings.Join(source.Labels, ",")})
		}
		if target, exists := v.nodesByID[edge.Target]; exists && !hasAnyLabel(target, rule.TargetLabels) {
			v.add(SchemaViolation{Code: SchemaViolationTargetLabel, Entity: "edge", EntityID: id, Rule: rule.Type, Field: "target", Expected: strings.Join(rule.TargetLabels, ","), Actual: strings.Join(target.Labels, ",")})
		}
	}
	v.validateCardinality()
}

func (v *schemaValidator) validateProperties(entity, id, rule string, properties map[string]PropertyValue, rules []PropertyRule) {
	for _, propertyRule := range rules {
		value, exists := properties[propertyRule.Key]
		if !exists {
			if propertyRule.Required {
				v.add(SchemaViolation{Code: SchemaViolationRequiredProperty, Entity: entity, EntityID: id, Rule: rule, Field: propertyRule.Key, Expected: propertyKindsText(propertyRule.Types)})
			}
			continue
		}
		if !containsPropertyKind(propertyRule.Types, value.Kind) {
			v.add(SchemaViolation{Code: SchemaViolationPropertyType, Entity: entity, EntityID: id, Rule: rule, Field: propertyRule.Key, Expected: propertyKindsText(propertyRule.Types), Actual: string(value.Kind)})
		}
	}
}

func (v *schemaValidator) validateCardinality() {
	for _, rule := range v.schema.EdgeRules {
		sourceCounts, targetCounts := make(map[string]uint32), make(map[string]uint32)
		for _, edge := range v.edgesByID {
			if edge.Type != rule.Type {
				continue
			}
			if _, exists := v.nodesByID[edge.Source]; exists {
				sourceCounts[edge.Source]++
			}
			if _, exists := v.nodesByID[edge.Target]; exists {
				targetCounts[edge.Target]++
			}
		}
		for _, id := range sortedNodeIDs(v.nodesByID) {
			node := v.nodesByID[id]
			if hasAnyLabel(node, rule.SourceLabels) {
				v.validateCardinalityValue(id, rule.Type, "source", sourceCounts[id], rule.Cardinality.SourceMin, rule.Cardinality.SourceMax)
			}
			if hasAnyLabel(node, rule.TargetLabels) {
				v.validateCardinalityValue(id, rule.Type, "target", targetCounts[id], rule.Cardinality.TargetMin, rule.Cardinality.TargetMax)
			}
		}
	}
}

func (v *schemaValidator) validateCardinalityValue(id, rule, endpoint string, count, minimum, maximum uint32) {
	if minimum != 0 && count < minimum {
		code := SchemaViolationSourceCardinalityMin
		if endpoint == "target" {
			code = SchemaViolationTargetCardinalityMin
		}
		v.add(SchemaViolation{Code: code, Entity: "node", EntityID: id, Rule: rule, Field: endpoint, Expected: "at least " + strconv.FormatUint(uint64(minimum), 10), Actual: strconv.FormatUint(uint64(count), 10)})
	}
	if maximum != 0 && count > maximum {
		code := SchemaViolationSourceCardinalityMax
		if endpoint == "target" {
			code = SchemaViolationTargetCardinalityMax
		}
		v.add(SchemaViolation{Code: code, Entity: "node", EntityID: id, Rule: rule, Field: endpoint, Expected: "at most " + strconv.FormatUint(uint64(maximum), 10), Actual: strconv.FormatUint(uint64(count), 10)})
	}
}

func (v *schemaValidator) validateNaturalKeys() {
	for _, rule := range v.schema.NodeRules {
		if !rule.NaturalKeyUnique {
			continue
		}
		groups := make(map[string][]string)
		for _, id := range sortedNodeIDs(v.nodesByID) {
			node := v.nodesByID[id]
			if !hasLabel(node, rule.Label) {
				continue
			}
			key, complete := naturalKeyEncoding(node, rule.NaturalKey)
			if complete {
				groups[key] = append(groups[key], id)
			}
		}
		groupKeys := make([]string, 0, len(groups))
		for key := range groups {
			groupKeys = append(groupKeys, key)
		}
		sort.Strings(groupKeys)
		for _, key := range groupKeys {
			ids := groups[key]
			if len(ids) < 2 {
				continue
			}
			sort.Strings(ids)
			for _, id := range ids {
				v.add(SchemaViolation{Code: SchemaViolationNaturalKeyUnique, Entity: "node", EntityID: id, Rule: rule.Label, Field: strings.Join(rule.NaturalKey, ","), Expected: "unique natural key"})
			}
		}
	}
}

func (v *schemaValidator) validateGlobalInvariants() {
	for _, invariant := range v.schema.GlobalInvariants {
		switch invariant {
		case GlobalInvariantNoSelfLoop:
			for _, id := range sortedEdgeIDs(v.edgesByID) {
				edge := v.edgesByID[id]
				if edge.Source == edge.Target {
					v.add(SchemaViolation{Code: SchemaViolationNoSelfLoop, Entity: "edge", EntityID: id, Rule: string(invariant), Expected: "distinct source and target"})
				}
			}
		case GlobalInvariantAcyclic:
			for _, id := range v.cycleEdgeIDs() {
				v.add(SchemaViolation{Code: SchemaViolationAcyclic, Entity: "edge", EntityID: id, Rule: string(invariant), Expected: "acyclic graph"})
			}
		}
	}
}

func (v *schemaValidator) cycleEdgeIDs() []string {
	adjacency := make(map[string][]string, len(v.nodesByID))
	for id := range v.nodesByID {
		adjacency[id] = nil
	}
	for _, edge := range v.edgesByID {
		if _, sourceExists := v.nodesByID[edge.Source]; !sourceExists {
			continue
		}
		if _, targetExists := v.nodesByID[edge.Target]; !targetExists {
			continue
		}
		adjacency[edge.Source] = append(adjacency[edge.Source], edge.Target)
	}
	for id := range adjacency {
		sort.Strings(adjacency[id])
	}
	components, componentSizes := stronglyConnectedComponents(adjacency)
	var cycleEdges []string
	for _, id := range sortedEdgeIDs(v.edgesByID) {
		edge := v.edgesByID[id]
		sourceComponent, sourceExists := components[edge.Source]
		targetComponent, targetExists := components[edge.Target]
		if sourceExists && targetExists && sourceComponent == targetComponent && (edge.Source == edge.Target || componentSizes[sourceComponent] > 1) {
			cycleEdges = append(cycleEdges, id)
		}
	}
	return cycleEdges
}

func stronglyConnectedComponents(adjacency map[string][]string) (map[string]int, map[int]int) {
	index, component := 0, 0
	indices, lowlinks := make(map[string]int, len(adjacency)), make(map[string]int, len(adjacency))
	onStack, components, componentSizes := make(map[string]bool, len(adjacency)), make(map[string]int, len(adjacency)), make(map[int]int)
	stack := make([]string, 0, len(adjacency))
	var visit func(string)
	visit = func(node string) {
		index++
		indices[node], lowlinks[node] = index, index
		stack, onStack[node] = append(stack, node), true
		for _, next := range adjacency[node] {
			if indices[next] == 0 {
				visit(next)
				if lowlinks[next] < lowlinks[node] {
					lowlinks[node] = lowlinks[next]
				}
			} else if onStack[next] && indices[next] < lowlinks[node] {
				lowlinks[node] = indices[next]
			}
		}
		if lowlinks[node] != indices[node] {
			return
		}
		size := 0
		for {
			last := len(stack) - 1
			member := stack[last]
			stack = stack[:last]
			onStack[member], components[member], size = false, component, size+1
			if member == node {
				break
			}
		}
		componentSizes[component] = size
		component++
	}
	nodes := make([]string, 0, len(adjacency))
	for node := range adjacency {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)
	for _, node := range nodes {
		if indices[node] == 0 {
			visit(node)
		}
	}
	return components, componentSizes
}

func naturalKeyEncoding(node Node, keys []string) (string, bool) {
	values := make([]PropertyValue, len(keys))
	for i, key := range keys {
		value, exists := node.Properties[key]
		if !exists {
			return "", false
		}
		values[i] = value
	}
	encoded, err := canonicalCBOR.Marshal(values)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

func hasLabel(node Node, label string) bool {
	index := sort.SearchStrings(node.Labels, label)
	return index < len(node.Labels) && node.Labels[index] == label
}

func hasAnyLabel(node Node, labels []string) bool {
	if len(labels) == 0 {
		return true
	}
	for _, label := range labels {
		if hasLabel(node, label) {
			return true
		}
	}
	return false
}

func containsPropertyKind(kinds []PropertyKind, actual PropertyKind) bool {
	index := sort.Search(len(kinds), func(i int) bool { return kinds[i] >= actual })
	return index < len(kinds) && kinds[index] == actual
}

func propertyKindsText(kinds []PropertyKind) string {
	values := make([]string, len(kinds))
	for i, kind := range kinds {
		values[i] = string(kind)
	}
	return strings.Join(values, ",")
}

func (v *schemaValidator) add(violation SchemaViolation) {
	v.violations = append(v.violations, violation)
}

func sortedNodeIDs(nodes map[string]Node) []string {
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func sortedEdgeIDs(edges map[string]Edge) []string {
	ids := make([]string, 0, len(edges))
	for id := range edges {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func sortSchemaViolations(violations []SchemaViolation) {
	sort.Slice(violations, func(i, j int) bool {
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
	})
}

func supportedGlobalInvariant(invariant GlobalInvariant) bool {
	return invariant == GlobalInvariantAcyclic || invariant == GlobalInvariantNoSelfLoop
}
