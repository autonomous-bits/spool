package repository

import (
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/autonomous-bits/spool/graphcontract"
)

var ErrInvalidSchemaTOML = graphcontract.ErrInvalidSchemaTOML

type propertyBudget struct {
	entries int
	bytes   int
}

// DecodeSchemaTOML delegates schema parsing and canonical normalization to the
// public graph contract.
func DecodeSchemaTOML(data []byte) (SchemaSnapshot, error) {
	return graphcontract.DecodeSchemaTOML(data)
}

// ParseSchemaTOML is an alias for DecodeSchemaTOML.
func ParseSchemaTOML(data []byte) (SchemaSnapshot, error) {
	return DecodeSchemaTOML(data)
}

// DecodeSchemaTOMLReader decodes a schema definition from a TOML stream.
func DecodeSchemaTOMLReader(reader io.Reader) (SchemaSnapshot, error) {
	return graphcontract.DecodeSchemaTOMLReader(reader)
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
