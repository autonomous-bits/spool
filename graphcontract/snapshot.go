package graphcontract

import (
	"bytes"
	"fmt"

	"github.com/fxamacker/cbor/v2"
)

// Snapshot is the immutable, content-addressed root set for one graph
// version: the node, edge, incoming-adjacency, outgoing-adjacency, and
// schema tree roots, plus their entity counts.
type Snapshot struct {
	// NodeRoot identifies the durable root of the snapshot's node projection.
	NodeRoot ObjectID `json:"nodeRoot" cbor:"1,keyasint"`
	// EdgeRoot identifies the durable root of the snapshot's edge projection.
	EdgeRoot ObjectID `json:"edgeRoot" cbor:"2,keyasint"`
	// OutAdjRoot identifies the durable root of the outgoing adjacency projection.
	OutAdjRoot ObjectID `json:"outAdjRoot" cbor:"3,keyasint"`
	// InAdjRoot identifies the durable root of the incoming adjacency projection.
	InAdjRoot ObjectID `json:"inAdjRoot" cbor:"4,keyasint"`
	// SchemaRoot identifies the schema used to validate the snapshot's graph.
	SchemaRoot ObjectID `json:"schemaRoot" cbor:"5,keyasint"`
	// NodeCount is the number of nodes reachable from NodeRoot.
	NodeCount uint64 `json:"nodeCount" cbor:"6,keyasint"`
	// EdgeCount is the number of edges reachable from EdgeRoot.
	EdgeCount uint64 `json:"edgeCount" cbor:"7,keyasint"`
}

// snapshotCBOR prevents Snapshot.MarshalCBOR from recursively dispatching
// while retaining the exact field tags of Snapshot.
type snapshotCBOR Snapshot

// NewSnapshot constructs a normalized snapshot root record.
func NewSnapshot(nodeRoot, edgeRoot, outAdjRoot, inAdjRoot, schemaRoot ObjectID, nodeCount, edgeCount uint64) (Snapshot, error) {
	return Snapshot{
		NodeRoot:   nodeRoot,
		EdgeRoot:   edgeRoot,
		OutAdjRoot: outAdjRoot,
		InAdjRoot:  inAdjRoot,
		SchemaRoot: schemaRoot,
		NodeCount:  nodeCount,
		EdgeCount:  edgeCount,
	}.Normalize()
}

// Normalize returns the canonical snapshot representation. Snapshot has no
// collections to sort or deduplicate; normalization is a defensive copy.
func (s Snapshot) Normalize() (Snapshot, error) {
	return Snapshot{
		NodeRoot:   s.NodeRoot,
		EdgeRoot:   s.EdgeRoot,
		OutAdjRoot: s.OutAdjRoot,
		InAdjRoot:  s.InAdjRoot,
		SchemaRoot: s.SchemaRoot,
		NodeCount:  s.NodeCount,
		EdgeCount:  s.EdgeCount,
	}, nil
}

// MarshalCBOR returns the normalized, canonical CBOR encoding of s.
func (s Snapshot) MarshalCBOR() ([]byte, error) {
	normalized, err := s.Normalize()
	if err != nil {
		return nil, err
	}
	return canonicalCBOR.Marshal(snapshotCBOR(normalized))
}

// UnmarshalCBOR decodes and verifies canonical CBOR for s.
func (s *Snapshot) UnmarshalCBOR(data []byte) error {
	decoded, err := UnmarshalSnapshot(data)
	if err != nil {
		return err
	}
	*s = decoded
	return nil
}

// Equal reports semantic equality after canonical normalization.
func (s Snapshot) Equal(other Snapshot) bool {
	normalized, err := s.Normalize()
	if err != nil {
		return false
	}
	otherNormalized, err := other.Normalize()
	return err == nil && normalized == otherNormalized
}

// Clone returns a copy of s. Snapshot has no reference fields, so Clone
// returns s by value.
func (s Snapshot) Clone() Snapshot {
	return s
}

// MarshalSnapshot returns the normalized, canonical CBOR encoding of s.
func MarshalSnapshot(s Snapshot) ([]byte, error) {
	return s.MarshalCBOR()
}

// UnmarshalSnapshot decodes and verifies canonical CBOR for a snapshot root
// record.
func UnmarshalSnapshot(data []byte) (Snapshot, error) {
	var snapshot snapshotCBOR
	if err := cbor.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("%w: decode snapshot: %v", ErrInvalidCanonicalCBOR, err)
	}
	normalized, err := Snapshot(snapshot).Normalize()
	if err != nil {
		return Snapshot{}, err
	}
	canonical, err := normalized.MarshalCBOR()
	if err != nil || !bytes.Equal(data, canonical) {
		return Snapshot{}, fmt.Errorf("%w: snapshot", ErrInvalidCanonicalCBOR)
	}
	return normalized, nil
}
