package repository

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
)

var (
	// ErrInvalidPropertyValue reports a property value with an unknown kind or a
	// non-finite floating-point value.
	ErrInvalidPropertyValue = errors.New("invalid property value")
	// ErrInvalidSchemaSnapshot reports a schema snapshot without a version.
	ErrInvalidSchemaSnapshot = errors.New("invalid schema snapshot")
)

// PropertyKind identifies the concrete value carried by a PropertyValue.
type PropertyKind string

const (
	PropertyNull    PropertyKind = "null"
	PropertyBool    PropertyKind = "bool"
	PropertyInteger PropertyKind = "integer"
	PropertyFloat   PropertyKind = "float"
	PropertyString  PropertyKind = "string"
	PropertyList    PropertyKind = "list"
	PropertyMap     PropertyKind = "map"
)

// PropertyValueKind is an alias retained for callers that prefer the explicit
// type name.
type PropertyValueKind = PropertyKind

// PropertyValue is a tagged, recursively composable graph property value.
// Only the field associated with Kind is significant.
type PropertyValue struct {
	Kind    PropertyKind             `json:"kind" cbor:"1,keyasint"`
	Bool    bool                     `json:"bool,omitempty" cbor:"2,keyasint,omitempty"`
	Integer int64                    `json:"integer,omitempty" cbor:"3,keyasint,omitempty"`
	Float   float64                  `json:"float,omitempty" cbor:"4,keyasint,omitempty"`
	String  string                   `json:"string,omitempty" cbor:"5,keyasint,omitempty"`
	List    []PropertyValue          `json:"list,omitempty" cbor:"6,keyasint,omitempty"`
	Map     map[string]PropertyValue `json:"map,omitempty" cbor:"7,keyasint,omitempty"`
}

// NullPropertyValue returns the canonical null property value.
func NullPropertyValue() PropertyValue { return PropertyValue{Kind: PropertyNull} }

// BoolPropertyValue returns a boolean property value.
func BoolPropertyValue(value bool) PropertyValue {
	return PropertyValue{Kind: PropertyBool, Bool: value}
}

// IntegerPropertyValue returns an integer property value.
func IntegerPropertyValue(value int64) PropertyValue {
	return PropertyValue{Kind: PropertyInteger, Integer: value}
}

// FloatPropertyValue returns a floating-point property value.
func FloatPropertyValue(value float64) PropertyValue {
	return PropertyValue{Kind: PropertyFloat, Float: value}
}

// StringPropertyValue returns a string property value.
func StringPropertyValue(value string) PropertyValue {
	return PropertyValue{Kind: PropertyString, String: value}
}

// ListPropertyValue returns a list property value.
func ListPropertyValue(value []PropertyValue) PropertyValue {
	return PropertyValue{Kind: PropertyList, List: value}
}

// MapPropertyValue returns a string-keyed map property value.
func MapPropertyValue(value map[string]PropertyValue) PropertyValue {
	return PropertyValue{Kind: PropertyMap, Map: value}
}

// Normalize returns the canonical representation of v. It clears fields that
// do not belong to v.Kind, recursively normalizes values, and normalizes
// negative zero to zero. CBOR canonical encoding deterministically orders the
// resulting string-keyed maps.
func (v PropertyValue) Normalize() (PropertyValue, error) {
	normalized := PropertyValue{Kind: v.Kind}
	switch v.Kind {
	case PropertyNull:
		return normalized, nil
	case PropertyBool:
		normalized.Bool = v.Bool
	case PropertyInteger:
		normalized.Integer = v.Integer
	case PropertyFloat:
		if math.IsNaN(v.Float) || math.IsInf(v.Float, 0) {
			return PropertyValue{}, fmt.Errorf("%w: float must be finite", ErrInvalidPropertyValue)
		}
		if v.Float == 0 {
			normalized.Float = 0
		} else {
			normalized.Float = v.Float
		}
	case PropertyString:
		normalized.String = v.String
	case PropertyList:
		if len(v.List) == 0 {
			return normalized, nil
		}
		normalized.List = make([]PropertyValue, len(v.List))
		for i, item := range v.List {
			item, err := item.Normalize()
			if err != nil {
				return PropertyValue{}, fmt.Errorf("%w: list item %d: %w", ErrInvalidPropertyValue, i, err)
			}
			normalized.List[i] = item
		}
	case PropertyMap:
		if len(v.Map) == 0 {
			return normalized, nil
		}
		normalized.Map = make(map[string]PropertyValue, len(v.Map))
		for key, item := range v.Map {
			item, err := item.Normalize()
			if err != nil {
				return PropertyValue{}, fmt.Errorf("%w: map key %q: %w", ErrInvalidPropertyValue, key, err)
			}
			normalized.Map[key] = item
		}
	default:
		return PropertyValue{}, fmt.Errorf("%w: kind %q", ErrInvalidPropertyValue, v.Kind)
	}
	return normalized, nil
}

// Equal reports semantic equality after canonical normalization.
func (v PropertyValue) Equal(other PropertyValue) bool {
	normalized, err := v.Normalize()
	if err != nil {
		return false
	}
	otherNormalized, err := other.Normalize()
	return err == nil && reflect.DeepEqual(normalized, otherNormalized)
}

// Node is the immutable node representation stored in a graph snapshot.
type Node struct {
	// ID uniquely identifies the node within a graph snapshot.
	ID string `json:"id" cbor:"1,keyasint"`
	// Title is the node's display value and compatibility field.
	Title string `json:"title" cbor:"2,keyasint"`
	// Labels identifies the node's sorted, unique type labels.
	Labels []string `json:"labels,omitempty" cbor:"3,keyasint,omitempty"`
	// Properties holds typed, recursively composable node properties.
	Properties map[string]PropertyValue `json:"properties,omitempty" cbor:"4,keyasint,omitempty"`
}

// Normalize returns a canonical node with sorted, deduplicated labels and
// normalized property values.
func (n Node) Normalize() (Node, error) {
	normalized := Node{ID: n.ID, Title: n.Title}
	if len(n.Labels) > 0 {
		labels := append([]string(nil), n.Labels...)
		sort.Strings(labels)
		normalized.Labels = labels[:0]
		for _, label := range labels {
			if len(normalized.Labels) == 0 || normalized.Labels[len(normalized.Labels)-1] != label {
				normalized.Labels = append(normalized.Labels, label)
			}
		}
	}
	if len(n.Properties) > 0 {
		normalized.Properties = make(map[string]PropertyValue, len(n.Properties))
		for key, value := range n.Properties {
			value, err := value.Normalize()
			if err != nil {
				return Node{}, fmt.Errorf("normalize node property %q: %w", key, err)
			}
			normalized.Properties[key] = value
		}
	}
	return normalized, nil
}

// Equal reports semantic equality after canonical normalization.
func (n Node) Equal(other Node) bool {
	normalized, err := n.Normalize()
	if err != nil {
		return false
	}
	otherNormalized, err := other.Normalize()
	return err == nil && reflect.DeepEqual(normalized, otherNormalized)
}

func (n Node) clone() Node {
	normalized, err := n.Normalize()
	if err != nil {
		panic(fmt.Sprintf("clone node %q: %v", n.ID, err))
	}
	return normalized
}

// Edge is the immutable edge representation stored in a graph snapshot.
type Edge struct {
	// ID uniquely identifies the edge within a graph snapshot.
	ID string `json:"id" cbor:"1,keyasint"`
	// Source identifies the edge's originating node.
	Source string `json:"source" cbor:"2,keyasint"`
	// Target identifies the edge's destination node.
	Target string `json:"target" cbor:"3,keyasint"`
	// Type identifies the edge's relationship type.
	Type string `json:"type,omitempty" cbor:"4,keyasint,omitempty"`
	// Properties holds typed, recursively composable edge properties.
	Properties map[string]PropertyValue `json:"properties,omitempty" cbor:"5,keyasint,omitempty"`
}

// Normalize returns a canonical edge with normalized property values.
func (e Edge) Normalize() (Edge, error) {
	normalized := Edge{ID: e.ID, Source: e.Source, Target: e.Target, Type: e.Type}
	if len(e.Properties) > 0 {
		normalized.Properties = make(map[string]PropertyValue, len(e.Properties))
		for key, value := range e.Properties {
			value, err := value.Normalize()
			if err != nil {
				return Edge{}, fmt.Errorf("normalize edge property %q: %w", key, err)
			}
			normalized.Properties[key] = value
		}
	}
	return normalized, nil
}

// Equal reports semantic equality after canonical normalization.
func (e Edge) Equal(other Edge) bool {
	normalized, err := e.Normalize()
	if err != nil {
		return false
	}
	otherNormalized, err := other.Normalize()
	return err == nil && reflect.DeepEqual(normalized, otherNormalized)
}

func (e Edge) clone() Edge {
	normalized, err := e.Normalize()
	if err != nil {
		panic(fmt.Sprintf("clone edge %q: %v", e.ID, err))
	}
	return normalized
}

// SchemaSnapshot is the canonical schema object referenced by a graph snapshot.
// The initial schema is deliberately permissive; validation policies are added
// by later schema versions.
type SchemaSnapshot struct {
	Version    uint16 `json:"version" cbor:"0,keyasint"`
	Permissive bool   `json:"permissive" cbor:"1,keyasint"`
}

// BuiltinSchemaVersion is the version of the initial permissive schema.
const BuiltinSchemaVersion uint16 = 1

// BuiltinSchemaSnapshot returns the built-in versioned permissive schema.
func BuiltinSchemaSnapshot() SchemaSnapshot {
	return SchemaSnapshot{Version: BuiltinSchemaVersion, Permissive: true}
}

// Normalize validates a schema snapshot's required version.
func (s SchemaSnapshot) Normalize() (SchemaSnapshot, error) {
	if s.Version == 0 {
		return SchemaSnapshot{}, ErrInvalidSchemaSnapshot
	}
	return s, nil
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
	normalized, err := normalizeCanonicalObject(value)
	if err != nil {
		return nil, err
	}
	return canonicalCBOR.Marshal(normalized)
}
