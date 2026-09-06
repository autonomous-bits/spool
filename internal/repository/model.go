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
	ErrInvalidSchemaSnapshot = graphcontract.ErrInvalidSchemaSnapshot
	// ErrInvalidSchemaDefinition reports inconsistent or unsupported schema rules.
	ErrInvalidSchemaDefinition = graphcontract.ErrInvalidSchemaDefinition
	// ErrInvalidSchemaIdentifier reports a label, type, or property key unsafe for storage.
	ErrInvalidSchemaIdentifier = graphcontract.ErrInvalidSchemaIdentifier
	// ErrPropertyValueLimit reports a property value exceeding an ingestion limit.
	ErrPropertyValueLimit = errors.New("property value exceeds limit")
)

const (
	MaxSchemaLabelLength       = 128
	MaxSchemaPropertyKeyLength = 128
	MaxPropertyStringLength    = 16 * 1024
	MaxPropertyEntries         = 256
	MaxPropertyAggregateBytes  = 64 * 1024
	MaxPropertyDepth           = 16
	BuiltinSchemaVersion       = graphcontract.BuiltinSchemaVersion
	GlobalInvariantAcyclic     = graphcontract.GlobalInvariantAcyclic
	GlobalInvariantNoSelfLoop  = graphcontract.GlobalInvariantNoSelfLoop
	UniversalModifierLabel     = graphcontract.UniversalModifierLabel
)

type (
	PropertyKind      = graphcontract.PropertyKind
	PropertyValueKind = graphcontract.PropertyValueKind
	PropertyValue     = graphcontract.PropertyValue
	Node              = graphcontract.Node
	Edge              = graphcontract.Edge
	SchemaSnapshot    = graphcontract.SchemaSnapshot
	NodeLabelRule     = graphcontract.NodeLabelRule
	EdgeTypeRule      = graphcontract.EdgeTypeRule
	PropertyRule      = graphcontract.PropertyRule
	Cardinality       = graphcontract.Cardinality
	GlobalInvariant   = graphcontract.GlobalInvariant
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

func NullPropertyValue() PropertyValue           { return graphcontract.NullPropertyValue() }
func BoolPropertyValue(value bool) PropertyValue { return graphcontract.BoolPropertyValue(value) }
func IntegerPropertyValue(value int64) PropertyValue {
	return graphcontract.IntegerPropertyValue(value)
}
func FloatPropertyValue(value float64) PropertyValue { return graphcontract.FloatPropertyValue(value) }
func StringPropertyValue(value string) PropertyValue { return graphcontract.StringPropertyValue(value) }
func ListPropertyValue(value []PropertyValue) PropertyValue {
	return graphcontract.ListPropertyValue(value)
}
func MapPropertyValue(value map[string]PropertyValue) PropertyValue {
	return graphcontract.MapPropertyValue(value)
}
func BuiltinSchemaSnapshot() SchemaSnapshot { return graphcontract.BuiltinSchemaSnapshot() }

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

func hasLabel(node Node, label string) bool {
	index := sort.SearchStrings(node.Labels, label)
	return index < len(node.Labels) && node.Labels[index] == label
}

func sortedEdgeIDs(edges map[string]Edge) []string {
	ids := make([]string, 0, len(edges))
	for id := range edges {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
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
	case graphcontract.Commit:
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
	case graphcontract.Commit:
		return graphcontract.MarshalCommit(value)
	case SchemaSnapshot:
		return graphcontract.MarshalSchemaSnapshot(value)
	}
	normalized, err := normalizeCanonicalObject(value)
	if err != nil {
		return nil, err
	}
	return canonicalCBOR.Marshal(normalized)
}
