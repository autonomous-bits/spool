package repository

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
)

var (
	// ErrInvalidDiffSelector reports a selector that names both or neither branch and commit.
	ErrInvalidDiffSelector = errors.New("diff selector must identify exactly one branch or commit")
	// ErrInvalidContinuation reports a continuation token from another request or with invalid encoding.
	ErrInvalidContinuation = errors.New("diff continuation token does not match request")
	// ErrInvalidDiffBudget reports a non-positive row budget.
	ErrInvalidDiffBudget = errors.New("diff max rows must be positive")
	// ErrResponseBudgetTooSmall reports a byte budget unable to represent the response metadata.
	ErrResponseBudgetTooSmall = errors.New("diff response budget cannot represent result metadata")
)

// DiffSelector identifies a snapshot by exactly one branch or commit.
type DiffSelector struct {
	// Branch selects the branch's current head.
	Branch string `json:"branch,omitempty"`
	// Commit selects an explicit existing commit.
	Commit string `json:"commit,omitempty"`
}

// DiffFilter restricts diff changes by identifiers or node title substring.
type DiffFilter struct {
	// NodeIDs restricts returned node changes when non-empty.
	NodeIDs []string `json:"nodeIds,omitempty"`
	// EdgeIDs restricts returned edge changes when non-empty.
	EdgeIDs []string `json:"edgeIds,omitempty"`
	// NodeTitleSubstr restricts node changes to titles containing this substring.
	NodeTitleSubstr string `json:"nodeTitleSubstring,omitempty"`
}

// DiffRequest describes a bounded, optionally filtered comparison of two snapshots.
type DiffRequest struct {
	// Base identifies the older comparison snapshot.
	Base DiffSelector `json:"base"`
	// Target identifies the newer comparison snapshot.
	Target DiffSelector `json:"target"`
	// Filter optionally limits the returned changes.
	Filter DiffFilter `json:"filter,omitempty"`
	// MaxRows limits changes and context entries returned in this page.
	MaxRows int `json:"maxRows"`
	// MaxResponseBytes limits the JSON-encoded response size.
	MaxResponseBytes int `json:"maxResponseBytes"`
	// IncludeOneHop includes related unchanged nodes and edges within remaining budgets.
	IncludeOneHop bool `json:"includeOneHop,omitempty"`
	// ContinuationToken resumes a prior request with matching comparison and budgets.
	ContinuationToken string `json:"continuationToken,omitempty"`
}

// DiffEntry describes an added, removed, or modified graph entity.
type DiffEntry struct {
	// Entity is "node" or "edge".
	Entity string `json:"entity"`
	// Change is "added", "removed", or "modified".
	Change string `json:"change"`
	// ID identifies the changed entity.
	ID string `json:"id"`
	// Node is populated when Entity is "node".
	Node *Node `json:"node,omitempty"`
	// Edge is populated when Entity is "edge".
	Edge *Edge `json:"edge,omitempty"`
}

// DiffContext describes an unchanged entity included as one-hop context.
type DiffContext struct {
	// Entity is "node" or "edge".
	Entity string `json:"entity"`
	// ID identifies the context entity.
	ID string `json:"id"`
	// Node is populated for node context.
	Node *Node `json:"node,omitempty"`
	// Edge is populated for edge context.
	Edge *Edge `json:"edge,omitempty"`
}

// DiffResult is one bounded page of changes and optional related context.
type DiffResult struct {
	// BaseCommit is the resolved commit selected by Base.
	BaseCommit ObjectID `json:"baseCommit"`
	// TargetCommit is the resolved commit selected by Target.
	TargetCommit ObjectID `json:"targetCommit"`
	// Changes contains the ordered page of matching changes.
	Changes []DiffEntry `json:"changes"`
	// Context contains related unchanged entities when requested and budget permits.
	Context []DiffContext `json:"context,omitempty"`
	// ContinuationToken resumes remaining changes with the same request.
	ContinuationToken string `json:"continuationToken,omitempty"`
}

type diffToken struct {
	Fingerprint string `json:"fingerprint"`
	Offset      int    `json:"offset"`
}

// Diff returns a deterministic, budgeted page comparing two repository snapshots.
func (r *Repository) Diff(request DiffRequest) (DiffResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.ensureOpenLocked(); err != nil {
		return DiffResult{}, err
	}
	base, err := r.resolveDiffSelectorLocked(request.Base)
	if err != nil {
		return DiffResult{}, err
	}
	target, err := r.resolveDiffSelectorLocked(request.Target)
	if err != nil {
		return DiffResult{}, err
	}
	fingerprint := diffFingerprint(base, target, request)
	offset, err := decodeDiffToken(request.ContinuationToken, fingerprint)
	if err != nil {
		return DiffResult{}, err
	}
	changes := r.diffChangesLocked(base, target, request.Filter)
	if request.MaxRows <= 0 {
		return DiffResult{}, ErrInvalidDiffBudget
	}
	if offset > len(changes) {
		return DiffResult{}, ErrInvalidContinuation
	}
	result := DiffResult{BaseCommit: base, TargetCommit: target, Changes: make([]DiffEntry, 0)}
	if !diffResultFits(result, request.MaxResponseBytes) {
		return DiffResult{}, ErrResponseBudgetTooSmall
	}
	limit := request.MaxRows
	next := offset
	for next < len(changes) && len(result.Changes) < limit {
		candidate := result
		candidate.Changes = append(append([]DiffEntry(nil), result.Changes...), changes[next])
		if next+1 < len(changes) {
			candidate.ContinuationToken = encodeDiffToken(fingerprint, next+1)
		}
		if !diffResultFits(candidate, request.MaxResponseBytes) {
			break
		}
		result = candidate
		next++
	}
	if next < len(changes) {
		if next == offset {
			return DiffResult{}, ErrResponseBudgetTooSmall
		}
		result.ContinuationToken = encodeDiffToken(fingerprint, next)
		if !diffResultFits(result, request.MaxResponseBytes) {
			return DiffResult{}, ErrResponseBudgetTooSmall
		}
	}
	if request.IncludeOneHop {
		for _, context := range r.diffContextLocked(base, target, result.Changes) {
			if len(result.Changes)+len(result.Context) >= limit {
				break
			}
			candidate := result
			candidate.Context = append(append([]DiffContext(nil), result.Context...), context)
			if !diffResultFits(candidate, request.MaxResponseBytes) {
				break
			}
			result = candidate
		}
	}
	return result, nil
}

func (r *Repository) resolveDiffSelectorLocked(selector DiffSelector) (ObjectID, error) {
	if (selector.Branch == "") == (selector.Commit == "") {
		return "", ErrInvalidDiffSelector
	}
	if selector.Branch != "" {
		commit, ok := r.branches[selector.Branch]
		if !ok {
			return "", ErrBranchNotFound
		}
		return commit, nil
	}
	commit := ObjectID(selector.Commit)
	if _, ok := r.commits[commit]; !ok {
		return "", ErrCommitNotFound
	}
	return commit, nil
}

func (r *Repository) diffChangesLocked(base, target ObjectID, filter DiffFilter) []DiffEntry {
	baseSnapshot, targetSnapshot := r.commits[base].Snapshot, r.commits[target].Snapshot
	baseNodes, targetNodes := r.projections[r.snapshots[baseSnapshot].NodeRoot], r.projections[r.snapshots[targetSnapshot].NodeRoot]
	baseEdges, targetEdges := r.edgeProjections[baseSnapshot], r.edgeProjections[targetSnapshot]
	changes := make([]DiffEntry, 0)
	changes = append(changes, diffNodeChanges(baseNodes, targetNodes, filter)...)
	changes = append(changes, diffEdgeChanges(baseEdges, targetEdges, filter)...)
	return changes
}

func diffNodeChanges(base, target map[string]Node, filter DiffFilter) []DiffEntry {
	return diffEntries(base, target, filter.NodeIDs, filter.NodeTitleSubstr, func(node Node) DiffEntry {
		return DiffEntry{Entity: "node", ID: node.ID, Node: &node}
	})
}

func diffEdgeChanges(base, target map[string]Edge, filter DiffFilter) []DiffEntry {
	if filter.NodeTitleSubstr != "" {
		return nil
	}
	return diffEntries(base, target, filter.EdgeIDs, "", func(edge Edge) DiffEntry {
		return DiffEntry{Entity: "edge", ID: edge.ID, Edge: &edge}
	})
}

func diffEntries[T comparable](base, target map[string]T, ids []string, title string, entry func(T) DiffEntry) []DiffEntry {
	allowed := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		allowed[id] = struct{}{}
	}
	byChange := map[string][]DiffEntry{"added": {}, "removed": {}, "modified": {}}
	consider := func(id string) bool {
		if len(allowed) > 0 {
			if _, ok := allowed[id]; !ok {
				return false
			}
		}
		if title != "" {
			node, ok := any(target[id]).(Node)
			return ok && contains(node.Title, title)
		}
		return true
	}
	for id, targetValue := range target {
		if !consider(id) {
			continue
		}
		baseValue, exists := base[id]
		change := "added"
		if exists {
			if baseValue == targetValue {
				continue
			}
			change = "modified"
		}
		value := targetValue
		item := entry(value)
		item.Change = change
		byChange[change] = append(byChange[change], item)
	}
	for id, baseValue := range base {
		if _, exists := target[id]; exists {
			continue
		}
		if len(allowed) > 0 {
			if _, ok := allowed[id]; !ok {
				continue
			}
		}
		if title != "" {
			node, ok := any(baseValue).(Node)
			if !ok || !contains(node.Title, title) {
				continue
			}
		}
		item := entry(baseValue)
		item.Change = "removed"
		byChange["removed"] = append(byChange["removed"], item)
	}
	result := make([]DiffEntry, 0)
	for _, change := range []string{"added", "removed", "modified"} {
		sort.Slice(byChange[change], func(i, j int) bool { return byChange[change][i].ID < byChange[change][j].ID })
		result = append(result, byChange[change]...)
	}
	return result
}

func (r *Repository) diffContextLocked(base, target ObjectID, changes []DiffEntry) []DiffContext {
	baseSnapshot, targetSnapshot := r.commits[base].Snapshot, r.commits[target].Snapshot
	nodes := make(map[string]Node)
	edges := make(map[string]Edge)
	for _, snapshot := range []ObjectID{baseSnapshot, targetSnapshot} {
		for id, node := range r.projections[r.snapshots[snapshot].NodeRoot] {
			nodes[id] = node
		}
		for id, edge := range r.edgeProjections[snapshot] {
			edges[id] = edge
		}
	}
	changed := make(map[string]struct{}, len(changes))
	anchors := make(map[string]struct{})
	for _, change := range changes {
		changed[change.Entity+":"+change.ID] = struct{}{}
		if change.Entity == "node" {
			anchors[change.ID] = struct{}{}
			continue
		}
		for _, snapshot := range []ObjectID{baseSnapshot, targetSnapshot} {
			if edge, exists := r.edgeProjections[snapshot][change.ID]; exists {
				anchors[edge.Source], anchors[edge.Target] = struct{}{}, struct{}{}
			}
		}
	}
	related := make(map[string]struct{})
	contextEdges := make(map[string]struct{})
	for _, edge := range edges {
		_, source := anchors[edge.Source]
		_, target := anchors[edge.Target]
		if source || target {
			related[edge.Source], related[edge.Target] = struct{}{}, struct{}{}
			contextEdges[edge.ID] = struct{}{}
		}
	}
	context := make([]DiffContext, 0)
	for id, node := range nodes {
		if _, isChanged := changed["node:"+id]; !isChanged {
			if _, related := related[id]; related {
				value := node
				context = append(context, DiffContext{Entity: "node", ID: id, Node: &value})
			}
		}
	}
	for id, edge := range edges {
		if _, isChanged := changed["edge:"+id]; !isChanged {
			if _, isContext := contextEdges[id]; isContext {
				value := edge
				context = append(context, DiffContext{Entity: "edge", ID: id, Edge: &value})
			}
		}
	}
	sort.Slice(context, func(i, j int) bool {
		if context[i].Entity != context[j].Entity {
			return context[i].Entity == "node"
		}
		return context[i].ID < context[j].ID
	})
	return context
}

func diffResultFits(result DiffResult, maxBytes int) bool {
	data, err := json.Marshal(result)
	return err == nil && len(data) <= maxBytes
}

func diffFingerprint(base, target ObjectID, request DiffRequest) string {
	filter := request.Filter
	filter.NodeIDs = append([]string(nil), filter.NodeIDs...)
	filter.EdgeIDs = append([]string(nil), filter.EdgeIDs...)
	sort.Strings(filter.NodeIDs)
	sort.Strings(filter.EdgeIDs)
	data, _ := json.Marshal(struct {
		Base             ObjectID
		Target           ObjectID
		Filter           DiffFilter
		MaxRows          int
		MaxResponseBytes int
	}{base, target, filter, request.MaxRows, request.MaxResponseBytes})
	sum := sha256.Sum256(data)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func encodeDiffToken(fingerprint string, offset int) string {
	data, _ := json.Marshal(diffToken{Fingerprint: fingerprint, Offset: offset})
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeDiffToken(token, fingerprint string) (int, error) {
	if token == "" {
		return 0, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, ErrInvalidContinuation
	}
	var decoded diffToken
	if json.Unmarshal(data, &decoded) != nil || decoded.Fingerprint != fingerprint || decoded.Offset < 0 {
		return 0, ErrInvalidContinuation
	}
	return decoded.Offset, nil
}

func contains(value, substring string) bool {
	return len(substring) == 0 || (len(value) >= len(substring) && containsAt(value, substring))
}

func containsAt(value, substring string) bool {
	for i := 0; i+len(substring) <= len(value); i++ {
		if value[i:i+len(substring)] == substring {
			return true
		}
	}
	return false
}
