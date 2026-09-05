package repository

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/autonomous-bits/spool/graphcontract"
)

var (
	// ErrInvalidPropertyValue reports a property value rejected by graphcontract.
	ErrInvalidPropertyValue = graphcontract.ErrInvalidPropertyValue
	// ErrInvalidSchemaSnapshot reports a schema snapshot without a version.
	ErrInvalidSchemaSnapshot = errors.New("invalid schema snapshot")
	// ErrInvalidSchemaDefinition reports inconsistent or unsupported schema rules.
	ErrInvalidSchemaDefinition = errors.New("invalid schema definition")
	// ErrInvalidSchemaIdentifier reports a label, type, or property key unsafe for storage.
	ErrInvalidSchemaIdentifier = errors.New("invalid schema identifier")
	// ErrPropertyValueLimit reports a property value exceeding an ingestion limit.
	ErrPropertyValueLimit = errors.New("property value exceeds limit")
)

const (
	// MaxSchemaLabelLength bounds schema labels and edge types in bytes.
	MaxSchemaLabelLength = 128
	// MaxSchemaPropertyKeyLength bounds schema and graph property keys in bytes.
	MaxSchemaPropertyKeyLength = 128
	// MaxPropertyStringLength bounds a string property value in bytes.
	MaxPropertyStringLength = 16 * 1024
	// MaxPropertyEntries bounds the aggregate number of values and map entries in one property set.
	MaxPropertyEntries = 256
	// MaxPropertyAggregateBytes bounds strings and keys in one property set.
	MaxPropertyAggregateBytes = 64 * 1024
	// MaxPropertyDepth bounds nesting in list and map property values.
	MaxPropertyDepth = 16
)

type (
	PropertyKind      = graphcontract.PropertyKind
	PropertyValueKind = graphcontract.PropertyValueKind
	PropertyValue     = graphcontract.PropertyValue
	Node              = graphcontract.Node
	Edge              = graphcontract.Edge
)

const (
	PropertyNull    = graphcontract.PropertyNull
	PropertyBool    = graphcontract.PropertyBool
	PropertyInteger = graphcontract.PropertyInteger
	PropertyFloat   = graphcontract.PropertyFloat
	PropertyString  = graphcontract.PropertyString
	PropertyList    = graphcontract.PropertyList
	PropertyMap     = graphcontract.PropertyMap
)

var (
	NullPropertyValue    = graphcontract.NullPropertyValue
	BoolPropertyValue    = graphcontract.BoolPropertyValue
	IntegerPropertyValue = graphcontract.IntegerPropertyValue
	FloatPropertyValue   = graphcontract.FloatPropertyValue
	StringPropertyValue  = graphcontract.StringPropertyValue
	ListPropertyValue    = graphcontract.ListPropertyValue
	MapPropertyValue     = graphcontract.MapPropertyValue
)

func canonicalNodeCollections(node Node) Node {
	if node.Labels == nil {
		node.Labels = []string{}
	}
	if node.Properties == nil {
		node.Properties = map[string]PropertyValue{}
	}
	return node
}

func canonicalEdgeProperties(edge Edge) Edge {
	if edge.Properties == nil {
		edge.Properties = map[string]PropertyValue{}
	}
	return edge
}

// SchemaSnapshot is the canonical schema object referenced by a graph snapshot.
// Version one is retained as the permissive built-in schema; later versions may
// declare node, edge, and repository-wide validation rules.
type SchemaSnapshot struct {
	Version          uint16            `json:"version" cbor:"0,keyasint"`
	Permissive       bool              `json:"permissive" cbor:"1,keyasint"`
	NodeRules        []NodeLabelRule   `json:"nodeRules,omitempty" cbor:"2,keyasint,omitempty"`
	EdgeRules        []EdgeTypeRule    `json:"edgeRules,omitempty" cbor:"3,keyasint,omitempty"`
	GlobalInvariants []GlobalInvariant `json:"globalInvariants,omitempty" cbor:"4,keyasint,omitempty"`
}

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

// GlobalInvariant names a repository-wide invariant enforced by a schema validator.
type GlobalInvariant string

// BuiltinSchemaVersion is the version of the initial permissive schema.
const BuiltinSchemaVersion uint16 = 1

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
			if err := validateSchemaIdentifier(string(invariant), MaxSchemaLabelLength, "global invariant"); err != nil {
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
	if err := validateSchemaIdentifier(r.Label, MaxSchemaLabelLength, "node label"); err != nil {
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
		if err := validatePropertyKey(key); err != nil {
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
	if err := validateSchemaIdentifier(r.Type, MaxSchemaLabelLength, "edge type"); err != nil {
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
			if err := validateSchemaIdentifier(label, MaxSchemaLabelLength, "label"); err != nil {
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
		if err := validatePropertyKey(rule.Key); err != nil {
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

func validatePropertyKey(key string) error {
	return validateSchemaIdentifier(key, MaxSchemaPropertyKeyLength, "property key")
}

func validateSchemaIdentifier(value string, maximum int, kind string) error {
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

func normalizeCanonicalObject(value any) (any, error) {
	switch value := value.(type) {
	case Node:
		return value.Normalize()
	case Edge:
		return value.Normalize()
	case SchemaSnapshot:
		return value.Normalize()
	default:
		return value, nil
	}
}

func canonicalObjectEncoding(value any) ([]byte, error) {
	switch value := value.(type) {
	case Node:
		return graphcontract.MarshalNode(value)
	case Edge:
		return graphcontract.MarshalEdge(value)
	}
	normalized, err := normalizeCanonicalObject(value)
	if err != nil {
		return nil, err
	}
	return canonicalCBOR.Marshal(normalized)
}
