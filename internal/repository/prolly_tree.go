package repository

import (
	"fmt"
	"sort"
	"strings"
)

// prollyTreeFanout bounds both leaf entries and internal child references.
// Keeping it fixed makes tree shape, object identity, and traversal cost
// deterministic for a given sorted entry sequence.
const prollyTreeFanout = 32

const (
	prollyTreeLeafType     = "prolly-tree-leaf"
	prollyTreeInternalType = "prolly-tree-internal"
)

type prollyTreeEntry struct {
	Key   string   `cbor:"1,keyasint"`
	Value ObjectID `cbor:"2,keyasint"`
}

type prollyTreeLeaf struct {
	Entries []prollyTreeEntry `cbor:"1,keyasint"`
}

type prollyTreeChild struct {
	LastKey string   `cbor:"1,keyasint"`
	Object  ObjectID `cbor:"2,keyasint"`
}

type prollyTreeInternal struct {
	Children []prollyTreeChild `cbor:"1,keyasint"`
}

// adjacencyKey encodes an endpoint and edge ID without changing their tuple order.
// NUL is escaped as NUL-1 and NUL-NUL terminates the endpoint.
func adjacencyKey(endpoint, edgeID string) string {
	var key strings.Builder
	key.Grow(len(endpoint) + len(edgeID) + 2)
	writeAdjacencyKeyComponent(&key, endpoint)
	key.WriteByte(0)
	key.WriteByte(0)
	writeAdjacencyKeyComponent(&key, edgeID)
	return key.String()
}

func writeAdjacencyKeyComponent(key *strings.Builder, component string) {
	for i := 0; i < len(component); i++ {
		key.WriteByte(component[i])
		if component[i] == 0 {
			key.WriteByte(1)
		}
	}
}

func (r *Repository) storeProllyTreeLocked(entries []prollyTreeEntry) (ObjectID, error) {
	if !validProllyEntries(entries, false) {
		return "", fmt.Errorf("invalid prolly tree entries")
	}
	if len(entries) == 0 {
		return r.objectStore.put(prollyTreeLeafType, prollyTreeLeaf{Entries: []prollyTreeEntry{}})
	}

	children := make([]prollyTreeChild, 0, (len(entries)+prollyTreeFanout-1)/prollyTreeFanout)
	for start := 0; start < len(entries); start += prollyTreeFanout {
		end := min(start+prollyTreeFanout, len(entries))
		leafEntries := append([]prollyTreeEntry(nil), entries[start:end]...)
		object, err := r.objectStore.put(prollyTreeLeafType, prollyTreeLeaf{Entries: leafEntries})
		if err != nil {
			return "", err
		}
		children = append(children, prollyTreeChild{LastKey: leafEntries[len(leafEntries)-1].Key, Object: object})
	}
	for len(children) > 1 {
		next := make([]prollyTreeChild, 0, (len(children)+prollyTreeFanout-1)/prollyTreeFanout)
		for start := 0; start < len(children); start += prollyTreeFanout {
			end := min(start+prollyTreeFanout, len(children))
			internalChildren := append([]prollyTreeChild(nil), children[start:end]...)
			object, err := r.objectStore.put(prollyTreeInternalType, prollyTreeInternal{Children: internalChildren})
			if err != nil {
				return "", err
			}
			next = append(next, prollyTreeChild{LastKey: internalChildren[len(internalChildren)-1].LastKey, Object: object})
		}
		children = next
	}
	return children[0].Object, nil
}

func (r *Repository) loadProllyTreeLocked(root ObjectID) ([]prollyTreeEntry, error) {
	return r.loadProllyTreeObjectLocked(root)
}

func (r *Repository) loadProllyTreeObjectLocked(id ObjectID) ([]prollyTreeEntry, error) {
	var leaf prollyTreeLeaf
	if err := r.loadObject(id, prollyTreeLeafType, &leaf); err == nil {
		if !validProllyEntries(leaf.Entries, true) {
			return nil, fmt.Errorf("decode prolly leaf %s", id)
		}
		return append([]prollyTreeEntry(nil), leaf.Entries...), nil
	}
	var internal prollyTreeInternal
	if err := r.loadObject(id, prollyTreeInternalType, &internal); err != nil {
		return nil, fmt.Errorf("read prolly tree object %s: %w", id, err)
	}
	if !validProllyChildren(internal.Children) {
		return nil, fmt.Errorf("decode prolly internal %s", id)
	}
	entries := make([]prollyTreeEntry, 0)
	for _, child := range internal.Children {
		childEntries, err := r.loadProllyTreeObjectLocked(child.Object)
		if err != nil {
			return nil, err
		}
		if len(childEntries) == 0 || childEntries[len(childEntries)-1].Key != child.LastKey {
			return nil, fmt.Errorf("invalid prolly child boundary %s", child.Object)
		}
		entries = append(entries, childEntries...)
	}
	if !validProllyEntries(entries, false) {
		return nil, fmt.Errorf("invalid prolly tree ordering")
	}
	return entries, nil
}

func validProllyEntries(entries []prollyTreeEntry, leaf bool) bool {
	if leaf && len(entries) > prollyTreeFanout {
		return false
	}
	for i, entry := range entries {
		if entry.Key == "" || entry.Value == "" || (i > 0 && entries[i-1].Key >= entry.Key) {
			return false
		}
	}
	return true
}

func validProllyChildren(children []prollyTreeChild) bool {
	if len(children) == 0 || len(children) > prollyTreeFanout {
		return false
	}
	for i, child := range children {
		if child.LastKey == "" || child.Object == "" || (i > 0 && children[i-1].LastKey >= child.LastKey) {
			return false
		}
	}
	return true
}

func sortedProllyEntries(entries []prollyTreeEntry) []prollyTreeEntry {
	sorted := append([]prollyTreeEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Key < sorted[j].Key })
	return sorted
}

func (r *Repository) reconstructSnapshotProjectionsLocked(snapshotID ObjectID, snapshot graphSnapshot) error {
	nodeEntries, err := r.loadProllyTreeLocked(snapshot.NodeRoot)
	if err != nil {
		return err
	}
	edgeEntries, err := r.loadProllyTreeLocked(snapshot.EdgeRoot)
	if err != nil {
		return err
	}
	outEntries, err := r.loadProllyTreeLocked(snapshot.OutAdjRoot)
	if err != nil {
		return err
	}
	inEntries, err := r.loadProllyTreeLocked(snapshot.InAdjRoot)
	if err != nil {
		return err
	}
	nodes := make(map[string]Node, len(nodeEntries))
	for _, entry := range nodeEntries {
		var node Node
		if err := r.loadObject(entry.Value, "node", &node); err != nil {
			return fmt.Errorf("decode node %s: %w", entry.Value, err)
		}
		normalized, normalizeErr := node.Normalize()
		if node.ID == "" || node.ID != entry.Key || normalizeErr != nil || !node.Equal(normalized) {
			return fmt.Errorf("decode node %s", entry.Value)
		}
		nodes[node.ID] = node
	}
	edges := make(map[string]Edge, len(edgeEntries))
	for _, entry := range edgeEntries {
		var edge Edge
		if err := r.loadObject(entry.Value, "edge", &edge); err != nil {
			return fmt.Errorf("decode edge %s: %w", entry.Value, err)
		}
		normalized, normalizeErr := edge.Normalize()
		if edge.ID == "" || edge.ID != entry.Key || normalizeErr != nil || !edge.Equal(normalized) {
			return fmt.Errorf("decode edge %s", entry.Value)
		}
		edges[edge.ID] = edge
	}
	if uint64(len(nodes)) != snapshot.NodeCount || uint64(len(edges)) != snapshot.EdgeCount {
		return fmt.Errorf("snapshot counts do not match prolly trees")
	}
	expectedOut := make([]prollyTreeEntry, 0, len(edges))
	expectedIn := make([]prollyTreeEntry, 0, len(edges))
	for _, entry := range edgeEntries {
		edge := edges[entry.Key]
		expectedOut = append(expectedOut, prollyTreeEntry{Key: adjacencyKey(edge.Source, edge.ID), Value: entry.Value})
		expectedIn = append(expectedIn, prollyTreeEntry{Key: adjacencyKey(edge.Target, edge.ID), Value: entry.Value})
	}
	if !sameProllyEntries(sortedProllyEntries(expectedOut), outEntries) ||
		!sameProllyEntries(sortedProllyEntries(expectedIn), inEntries) {
		return fmt.Errorf("snapshot adjacency trees do not match edge tree")
	}
	r.projections[snapshot.NodeRoot] = nodes
	r.edgeProjections[snapshotID] = edges
	if r.materializedSnapshots == nil {
		r.materializedSnapshots = make(map[ObjectID]struct{})
	}
	r.materializedSnapshots[snapshotID] = struct{}{}
	r.touchHistoricalProjectionLocked(snapshotID)
	return nil
}

func sameProllyEntries(left, right []prollyTreeEntry) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
