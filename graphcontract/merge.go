package graphcontract

import (
	"fmt"
	"reflect"
	"sort"
)

// MergeConflict describes one deterministic three-way merge disagreement.
type MergeConflict struct {
	// ConflictID is the deterministic identifier used when selecting a resolution.
	ConflictID string `json:"conflictId"`
	// Category is "structural", "schema", or "semantic".
	Category string `json:"category"`
	// Entity is "node", "edge", or "schema".
	Entity string `json:"entity"`
	// ID identifies the affected graph entity when applicable.
	ID string `json:"id,omitempty"`
	// Field identifies the overlapping field or property key.
	Field string `json:"field,omitempty"`
	// Paths identifies the affected graph locations in deterministic order.
	Paths []string `json:"paths"`
}

// MergeChange describes an entity changed from the target snapshot by a merge.
type MergeChange struct {
	Entity string `json:"entity"`
	ID     string `json:"id"`
	Change string `json:"change"`
}

// MergeResult is the deterministic outcome of a three-way graph merge simulation.
type MergeResult struct {
	Nodes      map[string]Node
	Edges      map[string]Edge
	SchemaRoot ObjectID
	Clean      bool
	Changes    []MergeChange
	Conflicts  []MergeConflict
	Violations []SchemaViolation
}

// ThreeWayMerge computes a deterministic three-way merge of graph nodes,
// edges, and schema roots against a common base keyed by canonical object ID.
// It reports structural and schema-root conflicts only; callers that can
// resolve SchemaRoot to a schema snapshot may perform validation afterward and
// append semantic conflicts and violations deterministically.
func ThreeWayMerge(
	baseNodes, sourceNodes, targetNodes map[string]Node,
	baseEdges, sourceEdges, targetEdges map[string]Edge,
	baseSchemaRoot, sourceSchemaRoot, targetSchemaRoot ObjectID,
) (MergeResult, error) {
	conflicts := make([]MergeConflict, 0)
	nodes := mergeNodeMaps(baseNodes, sourceNodes, targetNodes, &conflicts)
	edges := mergeEdgeMaps(baseEdges, sourceEdges, targetEdges, &conflicts)
	schemaRoot := mergeSchemaRoot(baseSchemaRoot, sourceSchemaRoot, targetSchemaRoot, &conflicts)
	changes := mergeChanges(targetNodes, nodes, targetEdges, edges)

	SortMergeConflicts(conflicts)
	for i := range conflicts {
		if conflicts[i].Paths == nil {
			conflicts[i].Paths = MergeConflictPaths(conflicts[i])
		}
		conflictID, err := MergeConflictID(conflicts[i])
		if err != nil {
			return MergeResult{}, fmt.Errorf("calculate merge conflict ID: %w", err)
		}
		conflicts[i].ConflictID = conflictID
	}

	return MergeResult{
		Nodes:      nodes,
		Edges:      edges,
		SchemaRoot: schemaRoot,
		Clean:      len(conflicts) == 0,
		Changes:    changes,
		Conflicts:  conflicts,
		Violations: nil,
	}, nil
}

func mergeSchemaRoot(base, source, target ObjectID, conflicts *[]MergeConflict) ObjectID {
	if source == target || source == base {
		return target
	}
	if target == base {
		return source
	}
	*conflicts = append(*conflicts, MergeConflict{Category: "schema", Entity: "schema", Field: "root"})
	return target
}

func mergeNodeMaps(base, source, target map[string]Node, conflicts *[]MergeConflict) map[string]Node {
	ids := unionIDs(base, source, target)
	result := make(map[string]Node, len(ids))
	for _, id := range ids {
		merged, present := mergeNode(id, base[id], source[id], target[id], hasNode(base, id), hasNode(source, id), hasNode(target, id), conflicts)
		if present {
			result[id] = merged
		}
	}
	return result
}

func mergeEdgeMaps(base, source, target map[string]Edge, conflicts *[]MergeConflict) map[string]Edge {
	ids := unionIDs(base, source, target)
	result := make(map[string]Edge, len(ids))
	for _, id := range ids {
		merged, present := mergeEdge(id, base[id], source[id], target[id], hasEdge(base, id), hasEdge(source, id), hasEdge(target, id), conflicts)
		if present {
			result[id] = merged
		}
	}
	return result
}

func mergeNode(id string, base, source, target Node, baseOK, sourceOK, targetOK bool, conflicts *[]MergeConflict) (Node, bool) {
	if resolved, value, present := mergeExistence("node", id, base, source, target, baseOK, sourceOK, targetOK, func(a, b Node) bool { return a.Equal(b) }, conflicts); resolved {
		return value, present
	}
	result := target.Clone()
	result.Title = mergeStringField("node", id, "title", base.Title, source.Title, target.Title, conflicts)
	result.Labels = mergeValueField("node", id, "labels", base.Labels, source.Labels, target.Labels, conflicts)
	result.Properties = mergeProperties("node", id, base.Properties, source.Properties, target.Properties, conflicts)
	return result, true
}

func mergeEdge(id string, base, source, target Edge, baseOK, sourceOK, targetOK bool, conflicts *[]MergeConflict) (Edge, bool) {
	if resolved, value, present := mergeExistence("edge", id, base, source, target, baseOK, sourceOK, targetOK, func(a, b Edge) bool { return a.Equal(b) }, conflicts); resolved {
		return value, present
	}
	result := target.Clone()
	result.Source = mergeStringField("edge", id, "source", base.Source, source.Source, target.Source, conflicts)
	result.Target = mergeStringField("edge", id, "target", base.Target, source.Target, target.Target, conflicts)
	result.Type = mergeStringField("edge", id, "type", base.Type, source.Type, target.Type, conflicts)
	result.Properties = mergeProperties("edge", id, base.Properties, source.Properties, target.Properties, conflicts)
	return result, true
}

func mergeExistence[T any](entity, id string, base, source, target T, baseOK, sourceOK, targetOK bool, equal func(T, T) bool, conflicts *[]MergeConflict) (bool, T, bool) {
	if baseOK && sourceOK && targetOK {
		return false, target, true
	}
	if sourceOK == targetOK && (!sourceOK || equal(source, target)) {
		return true, target, targetOK
	}
	if sourceOK == baseOK && (!sourceOK || equal(source, base)) {
		return true, target, targetOK
	}
	if targetOK == baseOK && (!targetOK || equal(target, base)) {
		return true, source, sourceOK
	}
	*conflicts = append(*conflicts, MergeConflict{Category: "structural", Entity: entity, ID: id, Field: "existence"})
	return true, target, targetOK
}

func mergeStringField(entity, id, field, base, source, target string, conflicts *[]MergeConflict) string {
	return mergeValueField(entity, id, field, base, source, target, conflicts)
}

func mergeValueField[T any](entity, id, field string, base, source, target T, conflicts *[]MergeConflict) T {
	if reflect.DeepEqual(source, target) || reflect.DeepEqual(source, base) {
		return target
	}
	if reflect.DeepEqual(target, base) {
		return source
	}
	*conflicts = append(*conflicts, MergeConflict{Category: "structural", Entity: entity, ID: id, Field: field})
	return target
}

func mergeProperties(entity, id string, base, source, target map[string]PropertyValue, conflicts *[]MergeConflict) map[string]PropertyValue {
	keys := unionIDs(base, source, target)
	result := make(map[string]PropertyValue, len(keys))
	for _, key := range keys {
		baseValue, baseOK := base[key]
		sourceValue, sourceOK := source[key]
		targetValue, targetOK := target[key]
		if sourceOK == targetOK && (!sourceOK || sourceValue.Equal(targetValue)) {
			if targetOK {
				result[key] = targetValue.Clone()
			}
			continue
		}
		if sourceOK == baseOK && (!sourceOK || sourceValue.Equal(baseValue)) {
			if targetOK {
				result[key] = targetValue.Clone()
			}
			continue
		}
		if targetOK == baseOK && (!targetOK || targetValue.Equal(baseValue)) {
			if sourceOK {
				result[key] = sourceValue.Clone()
			}
			continue
		}
		*conflicts = append(*conflicts, MergeConflict{Category: "structural", Entity: entity, ID: id, Field: "properties." + key})
		if targetOK {
			result[key] = targetValue.Clone()
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// SortMergeConflicts orders merge conflicts deterministically by category,
// entity, entity ID, field, and then path list contents.
func SortMergeConflicts(conflicts []MergeConflict) {
	sort.Slice(conflicts, func(i, j int) bool {
		if conflicts[i].Category != conflicts[j].Category {
			return conflicts[i].Category < conflicts[j].Category
		}
		if conflicts[i].Entity != conflicts[j].Entity {
			return conflicts[i].Entity < conflicts[j].Entity
		}
		if conflicts[i].ID != conflicts[j].ID {
			return conflicts[i].ID < conflicts[j].ID
		}
		if conflicts[i].Field != conflicts[j].Field {
			return conflicts[i].Field < conflicts[j].Field
		}
		return compareMergeConflictPaths(conflicts[i].Paths, conflicts[j].Paths) < 0
	})
}

// MergeConflictID returns the deterministic content-derived identifier for one
// merge conflict using the canonical merge-conflict object encoding.
func MergeConflictID(conflict MergeConflict) (string, error) {
	encoded, err := canonicalCBOR.Marshal(struct {
		Category string
		Entity   string
		ID       string
		Field    string
		Paths    []string
	}{conflict.Category, conflict.Entity, conflict.ID, conflict.Field, conflict.Paths})
	if err != nil {
		return "", err
	}
	return string(ObjectIDForEncoded("merge-conflict", encoded)), nil
}

// MergeConflictPaths returns the deterministic graph path list for one merge
// conflict when the conflict did not already provide explicit paths.
func MergeConflictPaths(conflict MergeConflict) []string {
	if conflict.Entity == "schema" {
		return []string{"schema/" + conflict.Field}
	}
	path := conflict.Entity + "/" + conflict.ID
	if conflict.Field != "" {
		path += "/" + conflict.Field
	}
	return []string{path}
}

// SchemaViolationPaths returns the deterministic graph path list that
// identifies a schema validation failure.
func SchemaViolationPaths(violation SchemaViolation) []string {
	path := violation.Entity + "/" + violation.EntityID
	if violation.Field != "" {
		path += "/" + violation.Field
	}
	if violation.Rule != "" {
		path += "/rule/" + violation.Rule
	}
	return []string{path}
}

func mergeChanges(targetNodes, nodes map[string]Node, targetEdges, edges map[string]Edge) []MergeChange {
	changes := make([]MergeChange, 0)
	for _, id := range unionIDs(targetNodes, nodes, nil) {
		_, targetOK, mergedOK := targetNodes[id], hasNode(targetNodes, id), hasNode(nodes, id)
		change := ""
		switch {
		case !targetOK && mergedOK:
			change = "added"
		case targetOK && !mergedOK:
			change = "removed"
		case targetOK && mergedOK && !targetNodes[id].Equal(nodes[id]):
			change = "modified"
		}
		if change != "" {
			changes = append(changes, MergeChange{Entity: "node", ID: id, Change: change})
		}
	}
	for _, id := range unionIDs(targetEdges, edges, nil) {
		_, targetOK, mergedOK := targetEdges[id], hasEdge(targetEdges, id), hasEdge(edges, id)
		change := ""
		switch {
		case !targetOK && mergedOK:
			change = "added"
		case targetOK && !mergedOK:
			change = "removed"
		case targetOK && mergedOK && !targetEdges[id].Equal(edges[id]):
			change = "modified"
		}
		if change != "" {
			changes = append(changes, MergeChange{Entity: "edge", ID: id, Change: change})
		}
	}
	return changes
}

func compareMergeConflictPaths(left, right []string) int {
	for index := 0; index < len(left) && index < len(right); index++ {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	switch {
	case len(left) < len(right):
		return -1
	case len(left) > len(right):
		return 1
	default:
		return 0
	}
}

func unionIDs[T any](first, second, third map[string]T) []string {
	ids := make(map[string]struct{}, len(first)+len(second)+len(third))
	for id := range first {
		ids[id] = struct{}{}
	}
	for id := range second {
		ids[id] = struct{}{}
	}
	for id := range third {
		ids[id] = struct{}{}
	}
	result := make([]string, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func hasNode(nodes map[string]Node, id string) bool { _, ok := nodes[id]; return ok }
func hasEdge(edges map[string]Edge, id string) bool { _, ok := edges[id]; return ok }
