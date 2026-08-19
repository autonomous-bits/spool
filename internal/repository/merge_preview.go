package repository

import (
	"errors"
	"reflect"
	"sort"
)

var (
	// ErrMergePreviewNotClean reports an attempt to apply a preview with conflicts.
	ErrMergePreviewNotClean = errors.New("merge preview is not clean")
	// ErrMergePreviewMismatch reports an apply request that does not name the current preview.
	ErrMergePreviewMismatch = errors.New("merge preview identifier does not match")
)

// MergeConflict describes a deterministic three-way merge disagreement.
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

// MergeChange describes an entity changed from the target snapshot by a preview.
type MergeChange struct {
	Entity string `json:"entity"`
	ID     string `json:"id"`
	Change string `json:"change"`
}

// MergePreview is an immutable, deterministic prediction of merging SourceBranch into TargetBranch.
type MergePreview struct {
	ID           ObjectID            `json:"id"`
	Binding      MergePreviewBinding `json:"binding"`
	SourceBranch string              `json:"sourceBranch"`
	TargetBranch string              `json:"targetBranch"`
	Clean        bool                `json:"clean"`
	Changes      []MergeChange       `json:"changes"`
	Conflicts    []MergeConflict     `json:"conflicts"`
	Violations   []SchemaViolation   `json:"violations,omitempty"`
}

type mergeCandidate struct {
	nodes      map[string]Node
	edges      map[string]Edge
	schemaRoot ObjectID
	preview    MergePreview
}

// PreviewMerge computes a three-way graph merge without changing repository state.
func (r *Repository) PreviewMerge(sourceBranch, targetBranch string) (MergePreview, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.ensureOpenLocked(); err != nil {
		return MergePreview{}, err
	}
	candidate, err := r.previewMergeLocked(sourceBranch, targetBranch)
	if err != nil {
		return MergePreview{}, err
	}
	return candidate.preview, nil
}

func (r *Repository) previewMergeLocked(sourceBranch, targetBranch string) (mergeCandidate, error) {
	source, ok := r.branches[sourceBranch]
	if !ok {
		return mergeCandidate{}, ErrBranchNotFound
	}
	target, ok := r.branches[targetBranch]
	if !ok {
		return mergeCandidate{}, ErrBranchNotFound
	}
	base, ok := r.mergeBaseLocked(source, target)
	if !ok {
		return mergeCandidate{}, ErrCommitNotFound
	}
	baseSnapshot := r.snapshots[r.commits[base].Snapshot]
	sourceSnapshot := r.snapshots[r.commits[source].Snapshot]
	targetSnapshot := r.snapshots[r.commits[target].Snapshot]
	conflicts := make([]MergeConflict, 0)
	nodes := mergeNodeMaps(
		r.projections[baseSnapshot.NodeRoot],
		r.projections[sourceSnapshot.NodeRoot],
		r.projections[targetSnapshot.NodeRoot],
		&conflicts,
	)
	edges := mergeEdgeMaps(
		r.edgeProjections[r.commits[base].Snapshot],
		r.edgeProjections[r.commits[source].Snapshot],
		r.edgeProjections[r.commits[target].Snapshot],
		&conflicts,
	)
	schemaRoot := mergeSchemaRoot(baseSnapshot.SchemaRoot, sourceSnapshot.SchemaRoot, targetSnapshot.SchemaRoot, &conflicts)
	violations := []SchemaViolation(nil)
	if len(conflicts) == 0 {
		schema, err := r.schemaSnapshotLocked(schemaRoot)
		if err != nil {
			return mergeCandidate{}, err
		}
		if err := ValidateSchemaSnapshot(schema, nodes, edges); err != nil {
			var validation *SchemaValidationError
			if errors.As(err, &validation) {
				violations = validation.Violations
			} else {
				return mergeCandidate{}, err
			}
			for _, violation := range violations {
				conflicts = append(conflicts, MergeConflict{
					Category: "semantic", Entity: violation.Entity, ID: violation.EntityID,
					Field: violation.Field, Paths: schemaViolationPaths(violation),
				})
			}
		}
	}
	sortMergeConflicts(conflicts)
	for index := range conflicts {
		if conflicts[index].Paths == nil {
			conflicts[index].Paths = mergeConflictPaths(conflicts[index])
		}
		conflicts[index].ConflictID = mergeConflictID(conflicts[index])
	}
	preview := MergePreview{
		Binding:      MergePreviewBinding{MergeBase: base, SourceCommit: source, TargetCommit: target},
		SourceBranch: sourceBranch, TargetBranch: targetBranch,
		Clean: len(conflicts) == 0,
		Changes: mergeChanges(
			r.projections[targetSnapshot.NodeRoot], nodes,
			r.edgeProjections[r.commits[target].Snapshot], edges,
		),
		Conflicts: conflicts, Violations: violations,
	}
	preview.ID = mergePreviewID(preview)
	return mergeCandidate{nodes: nodes, edges: edges, schemaRoot: schemaRoot, preview: preview}, nil
}

func mergePreviewID(preview MergePreview) ObjectID {
	return persistedObjectID("merge-preview", struct {
		Binding      MergePreviewBinding
		SourceBranch string
		TargetBranch string
		Clean        bool
		Changes      []MergeChange
		Conflicts    []MergeConflict
		Violations   []SchemaViolation
	}{preview.Binding, preview.SourceBranch, preview.TargetBranch, preview.Clean, preview.Changes, preview.Conflicts, preview.Violations})
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
	result := target.clone()
	result.Title = mergeStringField("node", id, "title", base.Title, source.Title, target.Title, conflicts)
	result.Labels = mergeValueField("node", id, "labels", base.Labels, source.Labels, target.Labels, conflicts)
	result.Properties = mergeProperties("node", id, base.Properties, source.Properties, target.Properties, conflicts)
	return result, true
}

func mergeEdge(id string, base, source, target Edge, baseOK, sourceOK, targetOK bool, conflicts *[]MergeConflict) (Edge, bool) {
	if resolved, value, present := mergeExistence("edge", id, base, source, target, baseOK, sourceOK, targetOK, func(a, b Edge) bool { return a.Equal(b) }, conflicts); resolved {
		return value, present
	}
	result := target.clone()
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
				result[key] = targetValue.clone()
			}
			continue
		}
		if sourceOK == baseOK && (!sourceOK || sourceValue.Equal(baseValue)) {
			if targetOK {
				result[key] = targetValue.clone()
			}
			continue
		}
		if targetOK == baseOK && (!targetOK || targetValue.Equal(baseValue)) {
			if sourceOK {
				result[key] = sourceValue.clone()
			}
			continue
		}
		*conflicts = append(*conflicts, MergeConflict{Category: "structural", Entity: entity, ID: id, Field: "properties." + key})
		if targetOK {
			result[key] = targetValue.clone()
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
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

func sortMergeConflicts(conflicts []MergeConflict) {
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
		return conflicts[i].Field < conflicts[j].Field
	})
}

func mergeConflictID(conflict MergeConflict) string {
	return string(persistedObjectID("merge-conflict", struct {
		Category string
		Entity   string
		ID       string
		Field    string
		Paths    []string
	}{conflict.Category, conflict.Entity, conflict.ID, conflict.Field, conflict.Paths}))
}

func mergeConflictPaths(conflict MergeConflict) []string {
	if conflict.Entity == "schema" {
		return []string{"schema/" + conflict.Field}
	}
	path := conflict.Entity + "/" + conflict.ID
	if conflict.Field != "" {
		path += "/" + conflict.Field
	}
	return []string{path}
}

func schemaViolationPaths(violation SchemaViolation) []string {
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
