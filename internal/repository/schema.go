package repository

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"
)

var (
	// ErrInvalidSchemaTOML reports malformed TOML or TOML that does not match
	// the schema authoring format.
	ErrInvalidSchemaTOML = errors.New("invalid schema TOML")
)

// DecodeSchemaTOML decodes a schema definition from TOML and returns its
// normalized canonical representation. Unknown keys are rejected so a typo
// cannot silently weaken validation.
func DecodeSchemaTOML(data []byte) (SchemaSnapshot, error) {
	return DecodeSchemaTOMLReader(bytes.NewReader(data))
}

// ParseSchemaTOML is an alias for DecodeSchemaTOML.
func ParseSchemaTOML(data []byte) (SchemaSnapshot, error) {
	return DecodeSchemaTOML(data)
}

// DecodeSchemaTOMLReader decodes a schema definition from a TOML stream.
func DecodeSchemaTOMLReader(reader io.Reader) (SchemaSnapshot, error) {
	var document tomlSchemaDocument
	decoder := toml.NewDecoder(reader).DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return SchemaSnapshot{}, fmt.Errorf("%w: %w", ErrInvalidSchemaTOML, err)
	}

	schema := SchemaSnapshot{
		Version:          document.Version,
		Permissive:       document.Permissive,
		GlobalInvariants: make([]GlobalInvariant, len(document.GlobalInvariants)),
		NodeRules:        make([]NodeLabelRule, len(document.Nodes)),
		EdgeRules:        make([]EdgeTypeRule, len(document.Edges)),
	}
	for i, invariant := range document.GlobalInvariants {
		schema.GlobalInvariants[i] = GlobalInvariant(invariant)
	}
	for i, node := range document.Nodes {
		schema.NodeRules[i] = NodeLabelRule{
			Label:            node.Label,
			Properties:       decodePropertyRules(node.Properties),
			NaturalKey:       node.NaturalKey,
			NaturalKeyUnique: node.NaturalKeyUnique,
		}
	}
	for i, edge := range document.Edges {
		schema.EdgeRules[i] = EdgeTypeRule{
			Type:         edge.Type,
			Properties:   decodePropertyRules(edge.Properties),
			SourceLabels: edge.SourceLabels,
			TargetLabels: edge.TargetLabels,
			Cardinality: Cardinality{
				SourceMin: edge.Cardinality.SourceMin,
				SourceMax: edge.Cardinality.SourceMax,
				TargetMin: edge.Cardinality.TargetMin,
				TargetMax: edge.Cardinality.TargetMax,
			},
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
}

func decodePropertyRules(rules []tomlPropertyRule) []PropertyRule {
	if len(rules) == 0 {
		return nil
	}
	decoded := make([]PropertyRule, len(rules))
	for i, rule := range rules {
		decoded[i] = PropertyRule{
			Key:      rule.Key,
			Required: rule.Required,
			Types:    propertyKinds(rule.Types),
		}
	}
	return decoded
}

func propertyKinds(types []string) []PropertyKind {
	if len(types) == 0 {
		return nil
	}
	kinds := make([]PropertyKind, len(types))
	for i, kind := range types {
		kinds[i] = PropertyKind(kind)
	}
	return kinds
}

func validateNodeIngestion(node Node) error {
	for _, label := range node.Labels {
		if err := validateSchemaIdentifier(label, MaxSchemaLabelLength, "label"); err != nil {
			return err
		}
	}
	return validatePropertiesIngestion(node.Properties)
}

func validateEdgeIngestion(edge Edge) error {
	if edge.Type != "" {
		if err := validateSchemaIdentifier(edge.Type, MaxSchemaLabelLength, "edge type"); err != nil {
			return err
		}
	}
	return validatePropertiesIngestion(edge.Properties)
}

func validatePropertiesIngestion(properties map[string]PropertyValue) error {
	budget := propertyBudget{}
	for key, value := range properties {
		if err := validatePropertyKey(key); err != nil {
			return err
		}
		budget.bytes += len(key)
		if budget.bytes > MaxPropertyAggregateBytes {
			return fmt.Errorf("%w: strings and keys exceed %d bytes", ErrPropertyValueLimit, MaxPropertyAggregateBytes)
		}
		if err := validatePropertyValueIngestion(value, 0, &budget); err != nil {
			return fmt.Errorf("validate property %q: %w", key, err)
		}
	}
	return nil
}

func validatePropertyValueIngestion(value PropertyValue, depth int, budget *propertyBudget) error {
	if depth > MaxPropertyDepth {
		return fmt.Errorf("%w: property nesting exceeds %d", ErrPropertyValueLimit, MaxPropertyDepth)
	}
	budget.entries++
	if budget.entries > MaxPropertyEntries {
		return fmt.Errorf("%w: more than %d property values", ErrPropertyValueLimit, MaxPropertyEntries)
	}
	switch value.Kind {
	case PropertyString:
		if !utf8.ValidString(value.String) {
			return fmt.Errorf("%w: string is not valid UTF-8", ErrInvalidPropertyValue)
		}
		if len(value.String) > MaxPropertyStringLength {
			return fmt.Errorf("%w: string exceeds %d bytes", ErrPropertyValueLimit, MaxPropertyStringLength)
		}
		budget.bytes += len(value.String)
		if budget.bytes > MaxPropertyAggregateBytes {
			return fmt.Errorf("%w: strings and keys exceed %d bytes", ErrPropertyValueLimit, MaxPropertyAggregateBytes)
		}
	case PropertyList:
		for i, item := range value.List {
			if err := validatePropertyValueIngestion(item, depth+1, budget); err != nil {
				return fmt.Errorf("list item %d: %w", i, err)
			}
		}
	case PropertyMap:
		for key, item := range value.Map {
			if err := validatePropertyKey(key); err != nil {
				return err
			}
			budget.bytes += len(key)
			if budget.bytes > MaxPropertyAggregateBytes {
				return fmt.Errorf("%w: strings and keys exceed %d bytes", ErrPropertyValueLimit, MaxPropertyAggregateBytes)
			}
			if err := validatePropertyValueIngestion(item, depth+1, budget); err != nil {
				return fmt.Errorf("map key %q: %w", key, err)
			}
		}
	}
	_, err := value.Normalize()
	return err
}
