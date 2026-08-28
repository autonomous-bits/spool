package repository

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

var (
	// ErrSchemaValidation reports graph contents that do not satisfy a schema.
	ErrSchemaValidation = errors.New("schema validation failed")
)

const (
	// GlobalInvariantAcyclic requires the directed graph to contain no cycles.
	GlobalInvariantAcyclic GlobalInvariant = "acyclic"
	// GlobalInvariantNoSelfLoop disallows edges whose endpoints are identical.
	GlobalInvariantNoSelfLoop GlobalInvariant = "no-self-loop"
)

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
// Entity and EntityID identify the affected graph value. Rule identifies the
// schema label, edge type, or global invariant, while Field narrows that rule
// to a property or endpoint when applicable.
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

// Error implements error.
func (e *SchemaValidationError) Error() string {
	return fmt.Sprintf("%s: %d violation(s)", ErrSchemaValidation, len(e.Violations))
}

// Unwrap makes SchemaValidationError match ErrSchemaValidation.
func (e *SchemaValidationError) Unwrap() error {
	return ErrSchemaValidation
}

// ValidateSchemaSnapshot checks fully materialized graph entities against
// schema. It never mutates nodes or edges. Invalid schemas are returned as
// their normalization errors; graph violations are returned as a
// *SchemaValidationError.
func ValidateSchemaSnapshot(schema SchemaSnapshot, nodes map[string]Node, edges map[string]Edge) error {
	normalizedSchema, err := schema.Normalize()
	if err != nil {
		return err
	}

	validator := schemaValidator{
		schema: normalizedSchema,
		nodes:  nodes,
		edges:  edges,
	}
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

// UniversalModifierLabel identifies the intrinsic cross-cutting modifier label.
const UniversalModifierLabel = "Ephemeral"

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
			v.add(SchemaViolation{
				Code: SchemaViolationSourceLabel, Entity: "edge", EntityID: id, Rule: rule.Type, Field: "source",
				Expected: strings.Join(rule.SourceLabels, ","), Actual: strings.Join(source.Labels, ","),
			})
		}
		if target, exists := v.nodesByID[edge.Target]; exists && !hasAnyLabel(target, rule.TargetLabels) {
			v.add(SchemaViolation{
				Code: SchemaViolationTargetLabel, Entity: "edge", EntityID: id, Rule: rule.Type, Field: "target",
				Expected: strings.Join(rule.TargetLabels, ","), Actual: strings.Join(target.Labels, ","),
			})
		}
	}
	v.validateCardinality()
}

func (v *schemaValidator) validateProperties(entity, id, rule string, properties map[string]PropertyValue, rules []PropertyRule) {
	for _, propertyRule := range rules {
		value, exists := properties[propertyRule.Key]
		if !exists {
			if propertyRule.Required {
				v.add(SchemaViolation{
					Code: SchemaViolationRequiredProperty, Entity: entity, EntityID: id, Rule: rule, Field: propertyRule.Key,
					Expected: propertyKindsText(propertyRule.Types),
				})
			}
			continue
		}
		if !containsPropertyKind(propertyRule.Types, value.Kind) {
			v.add(SchemaViolation{
				Code: SchemaViolationPropertyType, Entity: entity, EntityID: id, Rule: rule, Field: propertyRule.Key,
				Expected: propertyKindsText(propertyRule.Types), Actual: string(value.Kind),
			})
		}
	}
}

func (v *schemaValidator) validateCardinality() {
	for _, rule := range v.schema.EdgeRules {
		sourceCounts := make(map[string]uint32)
		targetCounts := make(map[string]uint32)
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
		v.add(SchemaViolation{
			Code: code, Entity: "node", EntityID: id, Rule: rule, Field: endpoint,
			Expected: "at least " + strconv.FormatUint(uint64(minimum), 10), Actual: strconv.FormatUint(uint64(count), 10),
		})
	}
	if maximum != 0 && count > maximum {
		code := SchemaViolationSourceCardinalityMax
		if endpoint == "target" {
			code = SchemaViolationTargetCardinalityMax
		}
		v.add(SchemaViolation{
			Code: code, Entity: "node", EntityID: id, Rule: rule, Field: endpoint,
			Expected: "at most " + strconv.FormatUint(uint64(maximum), 10), Actual: strconv.FormatUint(uint64(count), 10),
		})
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
				v.add(SchemaViolation{
					Code: SchemaViolationNaturalKeyUnique, Entity: "node", EntityID: id, Rule: rule.Label,
					Field:    strings.Join(rule.NaturalKey, ","),
					Expected: "unique natural key",
				})
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
					v.add(SchemaViolation{
						Code: SchemaViolationNoSelfLoop, Entity: "edge", EntityID: id, Rule: string(invariant),
						Expected: "distinct source and target",
					})
				}
			}
		case GlobalInvariantAcyclic:
			for _, id := range v.cycleEdgeIDs() {
				v.add(SchemaViolation{
					Code: SchemaViolationAcyclic, Entity: "edge", EntityID: id, Rule: string(invariant),
					Expected: "acyclic graph",
				})
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
	cycleEdges := make([]string, 0)
	for _, id := range sortedEdgeIDs(v.edgesByID) {
		edge := v.edgesByID[id]
		sourceComponent, sourceExists := components[edge.Source]
		targetComponent, targetExists := components[edge.Target]
		if !sourceExists || !targetExists || sourceComponent != targetComponent {
			continue
		}
		if edge.Source == edge.Target || componentSizes[sourceComponent] > 1 {
			cycleEdges = append(cycleEdges, id)
		}
	}
	return cycleEdges
}

func stronglyConnectedComponents(adjacency map[string][]string) (map[string]int, map[int]int) {
	index, component := 0, 0
	indices := make(map[string]int, len(adjacency))
	lowlinks := make(map[string]int, len(adjacency))
	onStack := make(map[string]bool, len(adjacency))
	stack := make([]string, 0, len(adjacency))
	components := make(map[string]int, len(adjacency))
	componentSizes := make(map[int]int)

	var visit func(string)
	visit = func(node string) {
		index++
		indices[node], lowlinks[node] = index, index
		stack = append(stack, node)
		onStack[node] = true
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
			onStack[member] = false
			components[member] = component
			size++
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
	switch invariant {
	case GlobalInvariantAcyclic, GlobalInvariantNoSelfLoop:
		return true
	default:
		return false
	}
}
