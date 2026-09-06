package graphcontract

import (
	"bytes"
	"errors"
	"testing"
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
}
