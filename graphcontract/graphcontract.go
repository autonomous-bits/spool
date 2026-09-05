// Package graphcontract defines Spool's canonical graph object contract.
package graphcontract

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"time"

	"github.com/fxamacker/cbor/v2"
)

var (
	// ErrInvalidPropertyValue reports a property value with an unknown kind or a
	// non-finite floating-point value.
	ErrInvalidPropertyValue = errors.New("invalid property value")
	// ErrInvalidCanonicalCBOR reports CBOR that does not encode a normalized
	// graph contract value in its canonical representation.
	ErrInvalidCanonicalCBOR = errors.New("invalid canonical CBOR")
	// ErrInvalidCommit reports a commit record without its required snapshot or
	// with an empty parent identifier.
	ErrInvalidCommit = errors.New("invalid commit")
)

var canonicalCBOR, _ = cbor.CanonicalEncOptions().EncMode()

// PropertyKind identifies the concrete value carried by a PropertyValue.
type PropertyKind string

// ObjectID is a content-derived identifier for an immutable graph object.
type ObjectID string

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

// propertyValueCBOR prevents PropertyValue.MarshalCBOR from recursively
// dispatching while retaining the exact field tags of PropertyValue.
type propertyValueCBOR PropertyValue

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

// MarshalCBOR returns the normalized, canonical CBOR encoding of v.
func (v PropertyValue) MarshalCBOR() ([]byte, error) {
	normalized, err := v.Normalize()
	if err != nil {
		return nil, err
	}
	return canonicalCBOR.Marshal(propertyValueCBOR(normalized))
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

// Clone returns a deep copy of v.
func (v PropertyValue) Clone() PropertyValue {
	cloned := v
	if v.List != nil {
		cloned.List = make([]PropertyValue, len(v.List))
		for i, item := range v.List {
			cloned.List[i] = item.Clone()
		}
	}
	if v.Map != nil {
		cloned.Map = make(map[string]PropertyValue, len(v.Map))
		for key, item := range v.Map {
			cloned.Map[key] = item.Clone()
		}
	}
	return cloned
}

// Node is the immutable node representation stored in a graph snapshot.
type Node struct {
	// ID uniquely identifies the node within a graph snapshot.
	ID string `json:"id" cbor:"1,keyasint"`
	// Title is the node's display value and compatibility field.
	Title string `json:"title" cbor:"2,keyasint"`
	// Labels identifies the node's sorted, unique type labels.
	Labels []string `json:"labels" cbor:"3,keyasint"`
	// Properties holds typed, recursively composable node properties.
	Properties map[string]PropertyValue `json:"properties" cbor:"4,keyasint"`
}

type nodeCBOR struct {
	ID         string                   `cbor:"1,keyasint"`
	Title      string                   `cbor:"2,keyasint"`
	Labels     []string                 `cbor:"3,keyasint"`
	Properties map[string]PropertyValue `cbor:"4,keyasint"`
}

// NewNode constructs a normalized node.
func NewNode(id, title string, labels []string, properties map[string]PropertyValue) (Node, error) {
	return Node{ID: id, Title: title, Labels: labels, Properties: properties}.Normalize()
}

// MarshalCBOR returns the normalized, canonical CBOR encoding of n. Omitted
// and explicitly empty collections have one encoding.
func (n Node) MarshalCBOR() ([]byte, error) {
	normalized, err := n.Normalize()
	if err != nil {
		return nil, err
	}
	labels := normalized.Labels
	if labels == nil {
		labels = []string{}
	}
	properties := normalized.Properties
	if properties == nil {
		properties = map[string]PropertyValue{}
	}
	return canonicalCBOR.Marshal(nodeCBOR{
		ID: normalized.ID, Title: normalized.Title, Labels: labels, Properties: properties,
	})
}

// Normalize returns a canonical node with sorted, deduplicated labels and
// normalized property values.
func (n Node) Normalize() (Node, error) {
	normalized := Node{ID: n.ID, Title: n.Title}
	if n.Labels != nil {
		labels := append([]string(nil), n.Labels...)
		sort.Strings(labels)
		normalized.Labels = make([]string, 0, len(labels))
		for _, label := range labels {
			if len(normalized.Labels) == 0 || normalized.Labels[len(normalized.Labels)-1] != label {
				normalized.Labels = append(normalized.Labels, label)
			}
		}
	}
	if n.Properties != nil {
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
	if err != nil {
		return false
	}
	return reflect.DeepEqual(canonicalNodeCollections(normalized), canonicalNodeCollections(otherNormalized))
}

func canonicalNodeCollections(node Node) Node {
	if node.Labels == nil {
		node.Labels = []string{}
	}
	if node.Properties == nil {
		node.Properties = map[string]PropertyValue{}
	}
	return node
}

// Clone returns a deep copy of n.
func (n Node) Clone() Node {
	cloned := Node{ID: n.ID, Title: n.Title}
	if n.Labels != nil {
		cloned.Labels = append([]string(nil), n.Labels...)
	}
	if n.Properties != nil {
		cloned.Properties = make(map[string]PropertyValue, len(n.Properties))
		for key, value := range n.Properties {
			cloned.Properties[key] = value.Clone()
		}
	}
	return cloned
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
	Properties map[string]PropertyValue `json:"properties" cbor:"5,keyasint"`
}

type edgeCBOR struct {
	ID         string                   `cbor:"1,keyasint"`
	Source     string                   `cbor:"2,keyasint"`
	Target     string                   `cbor:"3,keyasint"`
	Type       string                   `cbor:"4,keyasint,omitempty"`
	Properties map[string]PropertyValue `cbor:"5,keyasint"`
}

// NewEdge constructs a normalized edge.
func NewEdge(id, source, target, edgeType string, properties map[string]PropertyValue) (Edge, error) {
	return Edge{ID: id, Source: source, Target: target, Type: edgeType, Properties: properties}.Normalize()
}

// MarshalCBOR returns the normalized, canonical CBOR encoding of e. Omitted
// and explicitly empty properties have one encoding.
func (e Edge) MarshalCBOR() ([]byte, error) {
	normalized, err := e.Normalize()
	if err != nil {
		return nil, err
	}
	properties := normalized.Properties
	if properties == nil {
		properties = map[string]PropertyValue{}
	}
	return canonicalCBOR.Marshal(edgeCBOR{
		ID: normalized.ID, Source: normalized.Source, Target: normalized.Target, Type: normalized.Type, Properties: properties,
	})
}

// Normalize returns a canonical edge with normalized property values.
func (e Edge) Normalize() (Edge, error) {
	normalized := Edge{ID: e.ID, Source: e.Source, Target: e.Target, Type: e.Type}
	if e.Properties != nil {
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
	if err != nil {
		return false
	}
	return reflect.DeepEqual(canonicalEdgeProperties(normalized), canonicalEdgeProperties(otherNormalized))
}

func canonicalEdgeProperties(edge Edge) Edge {
	if edge.Properties == nil {
		edge.Properties = map[string]PropertyValue{}
	}
	return edge
}

// Clone returns a deep copy of e.
func (e Edge) Clone() Edge {
	cloned := Edge{ID: e.ID, Source: e.Source, Target: e.Target, Type: e.Type}
	if e.Properties != nil {
		cloned.Properties = make(map[string]PropertyValue, len(e.Properties))
		for key, value := range e.Properties {
			cloned.Properties[key] = value.Clone()
		}
	}
	return cloned
}

// Commit is an immutable record that assigns a graph snapshot to a point in
// the ordered commit DAG. Parents are identity-bearing: their order and
// repetition are preserved exactly.
type Commit struct {
	Snapshot ObjectID   `json:"snapshot" cbor:"1,keyasint"`
	Parents  []ObjectID `json:"parents" cbor:"2,keyasint"`
	Message  string     `json:"message" cbor:"3,keyasint"`
	Author   string     `json:"author" cbor:"4,keyasint"`
	Time     time.Time  `json:"time" cbor:"5,keyasint"`
}

// commitCBOR prevents Commit.MarshalCBOR from recursively dispatching while
// retaining the exact field tags of Commit.
type commitCBOR Commit

// NewCommit constructs a normalized commit record.
func NewCommit(snapshot ObjectID, parents []ObjectID, author, message string, timestamp time.Time) (Commit, error) {
	return Commit{
		Snapshot: snapshot,
		Parents:  parents,
		Author:   author,
		Message:  message,
		Time:     timestamp,
	}.Normalize()
}

// Normalize returns the canonical commit representation. It normalizes time
// to UTC whole seconds, the precision represented by canonical CBOR, and
// defensively copies parents without changing their order.
func (c Commit) Normalize() (Commit, error) {
	if c.Snapshot == "" {
		return Commit{}, fmt.Errorf("%w: snapshot is required", ErrInvalidCommit)
	}
	normalized := Commit{
		Snapshot: c.Snapshot,
		Message:  c.Message,
		Author:   c.Author,
		Time:     c.Time.UTC().Truncate(time.Second),
	}
	if len(c.Parents) == 0 {
		return normalized, nil
	}
	normalized.Parents = make([]ObjectID, len(c.Parents))
	for i, parent := range c.Parents {
		if parent == "" {
			return Commit{}, fmt.Errorf("%w: parent %d is required", ErrInvalidCommit, i)
		}
		normalized.Parents[i] = parent
	}
	return normalized, nil
}

// MarshalCBOR returns the normalized, canonical CBOR encoding of c. Omitted
// and explicitly empty parent collections have one encoding.
func (c Commit) MarshalCBOR() ([]byte, error) {
	normalized, err := c.Normalize()
	if err != nil {
		return nil, err
	}
	parents := normalized.Parents
	if parents == nil {
		parents = []ObjectID{}
	}
	return canonicalCBOR.Marshal(commitCBOR{
		Snapshot: normalized.Snapshot,
		Parents:  parents,
		Message:  normalized.Message,
		Author:   normalized.Author,
		Time:     normalized.Time,
	})
}

// UnmarshalCBOR decodes and verifies canonical CBOR for c.
func (c *Commit) UnmarshalCBOR(data []byte) error {
	decoded, err := UnmarshalCommit(data)
	if err != nil {
		return err
	}
	*c = decoded
	return nil
}

// Equal reports semantic equality after canonical normalization.
func (c Commit) Equal(other Commit) bool {
	normalized, err := c.Normalize()
	if err != nil {
		return false
	}
	otherNormalized, err := other.Normalize()
	return err == nil && reflect.DeepEqual(canonicalCommitParents(normalized), canonicalCommitParents(otherNormalized))
}

func canonicalCommitParents(commit Commit) Commit {
	if commit.Parents == nil {
		commit.Parents = []ObjectID{}
	}
	return commit
}

// Clone returns a deep copy of c.
func (c Commit) Clone() Commit {
	cloned := c
	if c.Parents != nil {
		cloned.Parents = append([]ObjectID(nil), c.Parents...)
	}
	return cloned
}

// MarshalPropertyValue returns the normalized, canonical CBOR encoding of v.
func MarshalPropertyValue(v PropertyValue) ([]byte, error) {
	return v.MarshalCBOR()
}

// MarshalNode returns the normalized, canonical CBOR encoding of n.
func MarshalNode(n Node) ([]byte, error) {
	return n.MarshalCBOR()
}

// MarshalEdge returns the normalized, canonical CBOR encoding of e.
func MarshalEdge(e Edge) ([]byte, error) {
	return e.MarshalCBOR()
}

// MarshalCommit returns the normalized, canonical CBOR encoding of c.
func MarshalCommit(c Commit) ([]byte, error) {
	return c.MarshalCBOR()
}

// UnmarshalPropertyValue decodes and verifies canonical CBOR for a property value.
func UnmarshalPropertyValue(data []byte) (PropertyValue, error) {
	var value PropertyValue
	if err := cbor.Unmarshal(data, &value); err != nil {
		return PropertyValue{}, fmt.Errorf("%w: decode property value: %v", ErrInvalidCanonicalCBOR, err)
	}
	normalized, err := value.Normalize()
	if err != nil {
		return PropertyValue{}, err
	}
	canonical, err := canonicalCBOR.Marshal(normalized)
	if err != nil || !bytes.Equal(data, canonical) {
		return PropertyValue{}, fmt.Errorf("%w: property value", ErrInvalidCanonicalCBOR)
	}
	return normalized, nil
}

// UnmarshalNode decodes and verifies canonical CBOR for a node.
func UnmarshalNode(data []byte) (Node, error) {
	var node Node
	if err := cbor.Unmarshal(data, &node); err != nil {
		return Node{}, fmt.Errorf("%w: decode node: %v", ErrInvalidCanonicalCBOR, err)
	}
	normalized, err := node.Normalize()
	if err != nil {
		return Node{}, err
	}
	canonical, err := canonicalCBOR.Marshal(normalized)
	if err != nil || !bytes.Equal(data, canonical) {
		return Node{}, fmt.Errorf("%w: node", ErrInvalidCanonicalCBOR)
	}
	return normalized, nil
}

// UnmarshalEdge decodes and verifies canonical CBOR for an edge.
func UnmarshalEdge(data []byte) (Edge, error) {
	var edge Edge
	if err := cbor.Unmarshal(data, &edge); err != nil {
		return Edge{}, fmt.Errorf("%w: decode edge: %v", ErrInvalidCanonicalCBOR, err)
	}
	normalized, err := edge.Normalize()
	if err != nil {
		return Edge{}, err
	}
	canonical, err := canonicalCBOR.Marshal(normalized)
	if err != nil || !bytes.Equal(data, canonical) {
		return Edge{}, fmt.Errorf("%w: edge", ErrInvalidCanonicalCBOR)
	}
	return normalized, nil
}

// UnmarshalCommit decodes and verifies canonical CBOR for a commit record.
func UnmarshalCommit(data []byte) (Commit, error) {
	var commit commitCBOR
	if err := cbor.Unmarshal(data, &commit); err != nil {
		return Commit{}, fmt.Errorf("%w: decode commit: %v", ErrInvalidCanonicalCBOR, err)
	}
	normalized, err := Commit(commit).Normalize()
	if err != nil {
		return Commit{}, err
	}
	canonical, err := normalized.MarshalCBOR()
	if err != nil || !bytes.Equal(data, canonical) {
		return Commit{}, fmt.Errorf("%w: commit", ErrInvalidCanonicalCBOR)
	}
	return normalized, nil
}
