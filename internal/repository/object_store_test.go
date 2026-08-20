package repository

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

func TestLooseObjectStoreWritesCanonicalEnvelopeAndLoadsCache(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), ".spl")
	cache := make(map[ObjectID][]byte)
	store := newLooseObjectStore(stateDir, &cache)
	value := map[string]string{"beta": "two", "alpha": "one"}

	id, err := store.put("fixture", value)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	const wantID ObjectID = "84d795f2ecfdd03ad938173fd4be707b7f1e99f91dc1c8ac0d9077364ae3a752"
	if id != wantID {
		t.Fatalf("object ID = %q, want %q", id, wantID)
	}

	path := filepath.Join(stateDir, "objects", "loose", string(id[:2]), string(id[2:]))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read loose object: %v", err)
	}
	var envelope looseObjectEnvelope
	if err := cbor.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if envelope.Type != "fixture" {
		t.Fatalf("envelope type = %q, want fixture", envelope.Type)
	}
	wantPayload, err := canonicalObjectEncoding(value)
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	if !bytes.Equal(envelope.Data, wantPayload) {
		t.Fatal("envelope payload is not canonical object encoding")
	}
	wantEnvelope, err := canonicalCBOR.Marshal(envelope)
	if err != nil {
		t.Fatalf("encode envelope: %v", err)
	}
	if !bytes.Equal(data, wantEnvelope) {
		t.Fatal("loose-object envelope is not canonical CBOR")
	}

	delete(cache, id)
	got, err := store.get(id, "fixture")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !bytes.Equal(got, wantPayload) {
		t.Fatal("read object bytes differ from stored canonical bytes")
	}
	if !bytes.Equal(cache[id], wantPayload) {
		t.Fatal("verified read did not repopulate the in-memory cache")
	}

	if secondID, err := store.put("fixture", value); err != nil || secondID != id {
		t.Fatalf("idempotent put = (%q, %v), want (%q, nil)", secondID, err, id)
	}
}

func TestLooseObjectStoreReadsVerifiedObjectWithoutExpectedType(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), ".spl")
	cache := make(map[ObjectID][]byte)
	store := newLooseObjectStore(stateDir, &cache)
	id, err := store.put("future-object", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	delete(cache, id)

	objectType, data, err := store.getAny(id)
	if err != nil {
		t.Fatalf("getAny: %v", err)
	}
	if objectType != "future-object" || len(data) == 0 {
		t.Fatalf("getAny = (%q, %d bytes), want future-object payload", objectType, len(data))
	}
	if _, err := store.get(id, "other-object"); !errors.Is(err, errLooseObjectType) {
		t.Fatalf("typed get error = %v, want type mismatch", err)
	}
}

func TestLooseObjectStoreRejectsInvalidOrTamperedObjects(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), ".spl")
	cache := make(map[ObjectID][]byte)
	store := newLooseObjectStore(stateDir, &cache)
	id, err := store.put("fixture", map[string]string{"alpha": "one"})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	path, err := store.path(id)
	if err != nil {
		t.Fatalf("path: %v", err)
	}

	delete(cache, id)
	if _, err := store.get(id, "other-fixture"); !errors.Is(err, errLooseObjectType) {
		t.Fatalf("get wrong type error = %v, want type mismatch", err)
	}

	tampered, err := canonicalCBOR.Marshal(looseObjectEnvelope{Type: "fixture", Data: []byte("tampered")})
	if err != nil {
		t.Fatalf("encode tampered envelope: %v", err)
	}
	if err := os.WriteFile(path, tampered, 0o600); err != nil {
		t.Fatalf("tamper loose object: %v", err)
	}
	if _, err := store.get(id, "fixture"); !errors.Is(err, errLooseObjectCorrupt) {
		t.Fatalf("get tampered error = %v, want corrupt object", err)
	}

	if _, err := store.path(ObjectID("../outside")); !errors.Is(err, errInvalidLooseObjectID) {
		t.Fatalf("unsafe path error = %v, want invalid object ID", err)
	}
}

func TestDurableRepositorySeedsLooseObjectCache(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), ".spl")
	repo, err := NewSeedRepositoryWithMergeState(stateDir)
	if err != nil {
		t.Fatalf("create durable repository: %v", err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Errorf("close repository: %v", err)
		}
	})

	node := Node{ID: SeedNodeID, Title: "EDG walking skeleton"}
	nodeID := repo.objectID("node", node)
	delete(repo.objects, nodeID)
	if _, err := repo.objectStore.get(nodeID, "node"); err != nil {
		t.Fatalf("read seeded loose object: %v", err)
	}
	if _, ok := repo.objects[nodeID]; !ok {
		t.Fatal("loose-object read did not restore repository object cache")
	}
}
