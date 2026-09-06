package graphcontract

import (
	"bytes"
	"errors"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

func TestSnapshotCanonicalRoundTrip(t *testing.T) {
	snapshot, err := NewSnapshot("node-root", "edge-root", "out-adj-root", "in-adj-root", "schema-root", 3, 5)
	if err != nil {
		t.Fatalf("NewSnapshot: %v", err)
	}
	data, err := MarshalSnapshot(snapshot)
	if err != nil {
		t.Fatalf("MarshalSnapshot: %v", err)
	}
	decoded, err := UnmarshalSnapshot(data)
	if err != nil {
		t.Fatalf("UnmarshalSnapshot: %v", err)
	}
	if !decoded.Equal(snapshot) {
		t.Fatalf("decoded snapshot = %+v, want %+v", decoded, snapshot)
	}
	roundTripped, err := MarshalSnapshot(decoded)
	if err != nil {
		t.Fatalf("MarshalSnapshot(decoded): %v", err)
	}
	if !bytes.Equal(data, roundTripped) {
		t.Fatalf("round-trip CBOR mismatch: %x != %x", data, roundTripped)
	}
}

func TestSnapshotMarshalCBORMatchesUnmarshalCBOR(t *testing.T) {
	snapshot, err := NewSnapshot("node-root", "edge-root", "out-adj-root", "in-adj-root", "schema-root", 1, 2)
	if err != nil {
		t.Fatalf("NewSnapshot: %v", err)
	}
	data, err := snapshot.MarshalCBOR()
	if err != nil {
		t.Fatalf("MarshalCBOR: %v", err)
	}
	var decoded Snapshot
	if err := decoded.UnmarshalCBOR(data); err != nil {
		t.Fatalf("UnmarshalCBOR: %v", err)
	}
	if !decoded.Equal(snapshot) {
		t.Fatalf("decoded snapshot = %+v, want %+v", decoded, snapshot)
	}
}

func TestSnapshotObjectIDStability(t *testing.T) {
	snapshot, err := NewSnapshot("node-root", "edge-root", "out-adj-root", "in-adj-root", "schema-root", 3, 5)
	if err != nil {
		t.Fatalf("NewSnapshot: %v", err)
	}
	data, err := MarshalSnapshot(snapshot)
	if err != nil {
		t.Fatalf("MarshalSnapshot: %v", err)
	}
	first := ObjectIDForEncoded("graph-snapshot", data)
	for i := 0; i < 5; i++ {
		data, err := MarshalSnapshot(snapshot)
		if err != nil {
			t.Fatalf("MarshalSnapshot: %v", err)
		}
		if got := ObjectIDForEncoded("graph-snapshot", data); got != first {
			t.Fatalf("ObjectIDForEncoded is not stable: got %s, want %s", got, first)
		}
	}

	other, err := NewSnapshot("node-root", "edge-root", "out-adj-root", "in-adj-root", "schema-root", 3, 6)
	if err != nil {
		t.Fatalf("NewSnapshot: %v", err)
	}
	otherData, err := MarshalSnapshot(other)
	if err != nil {
		t.Fatalf("MarshalSnapshot: %v", err)
	}
	if got := ObjectIDForEncoded("graph-snapshot", otherData); got == first {
		t.Fatalf("distinct snapshots produced the same ObjectID: %s", got)
	}
}

func TestSnapshotNormalizeIdempotent(t *testing.T) {
	snapshot := Snapshot{
		NodeRoot: "node-root", EdgeRoot: "edge-root", OutAdjRoot: "out-adj-root",
		InAdjRoot: "in-adj-root", SchemaRoot: "schema-root", NodeCount: 3, EdgeCount: 5,
	}
	once, err := snapshot.Normalize()
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	twice, err := once.Normalize()
	if err != nil {
		t.Fatalf("Normalize (twice): %v", err)
	}
	if once != twice {
		t.Fatalf("Normalize is not idempotent: %+v != %+v", once, twice)
	}
	if once != snapshot {
		t.Fatalf("Normalize changed a snapshot with no collections to canonicalize: %+v != %+v", once, snapshot)
	}
}

func TestSnapshotEqualAndClone(t *testing.T) {
	snapshot, err := NewSnapshot("node-root", "edge-root", "out-adj-root", "in-adj-root", "schema-root", 3, 5)
	if err != nil {
		t.Fatalf("NewSnapshot: %v", err)
	}
	clone := snapshot.Clone()
	if !clone.Equal(snapshot) {
		t.Fatalf("Clone() is not Equal to the original: %+v != %+v", clone, snapshot)
	}
	clone.NodeRoot = "changed"
	if clone.Equal(snapshot) {
		t.Fatalf("mutated clone unexpectedly Equal to the original")
	}
	if snapshot.NodeRoot != "node-root" {
		t.Fatalf("mutating the clone mutated the original: %+v", snapshot)
	}

	differentCount, err := NewSnapshot("node-root", "edge-root", "out-adj-root", "in-adj-root", "schema-root", 3, 6)
	if err != nil {
		t.Fatalf("NewSnapshot: %v", err)
	}
	if snapshot.Equal(differentCount) {
		t.Fatalf("snapshots with different EdgeCount compared Equal")
	}
}

// reorderedSnapshotCBOR carries the same field tags as snapshotCBOR but
// declares them in a different order, so a non-canonical (plain) CBOR
// encoding emits map keys out of ascending order while still decoding to an
// identical Snapshot value.
type reorderedSnapshotCBOR struct {
	EdgeCount  uint64   `cbor:"7,keyasint"`
	NodeCount  uint64   `cbor:"6,keyasint"`
	SchemaRoot ObjectID `cbor:"5,keyasint"`
	InAdjRoot  ObjectID `cbor:"4,keyasint"`
	OutAdjRoot ObjectID `cbor:"3,keyasint"`
	EdgeRoot   ObjectID `cbor:"2,keyasint"`
	NodeRoot   ObjectID `cbor:"1,keyasint"`
}

func TestUnmarshalSnapshotRejectsNonCanonicalCBOR(t *testing.T) {
	snapshot, err := NewSnapshot("node-root", "edge-root", "out-adj-root", "in-adj-root", "schema-root", 3, 5)
	if err != nil {
		t.Fatalf("NewSnapshot: %v", err)
	}
	data, err := MarshalSnapshot(snapshot)
	if err != nil {
		t.Fatalf("MarshalSnapshot: %v", err)
	}
	corrupted := append([]byte(nil), data...)
	corrupted[len(corrupted)-1] ^= 0xFF
	if _, err := UnmarshalSnapshot(corrupted); err == nil {
		t.Fatalf("UnmarshalSnapshot accepted corrupted bytes")
	}

	if _, err := UnmarshalSnapshot([]byte("not cbor")); !errors.Is(err, ErrInvalidCanonicalCBOR) {
		t.Fatalf("UnmarshalSnapshot error = %v, want %v", err, ErrInvalidCanonicalCBOR)
	}

	// A valid, decodable CBOR map with keys out of canonical ascending order
	// must still be rejected: it decodes to the same Snapshot value, but its
	// bytes are not the canonical re-encoding.
	nonCanonical, err := cbor.Marshal(reorderedSnapshotCBOR{
		NodeRoot: "node-root", EdgeRoot: "edge-root", OutAdjRoot: "out-adj-root",
		InAdjRoot: "in-adj-root", SchemaRoot: "schema-root", NodeCount: 3, EdgeCount: 5,
	})
	if err != nil {
		t.Fatalf("marshal non-canonical snapshot: %v", err)
	}
	if bytes.Equal(nonCanonical, data) {
		t.Fatalf("reordered encoding unexpectedly matched the canonical encoding")
	}
	var probe map[int]any
	if err := cbor.Unmarshal(nonCanonical, &probe); err != nil {
		t.Fatalf("non-canonical bytes must still be valid, decodable CBOR: %v", err)
	}
	if _, err := UnmarshalSnapshot(nonCanonical); !errors.Is(err, ErrInvalidCanonicalCBOR) {
		t.Fatalf("UnmarshalSnapshot non-canonical-order error = %v, want %v", err, ErrInvalidCanonicalCBOR)
	}
}
