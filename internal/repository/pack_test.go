package repository

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestGCPacksReachableObjectsAndReopensFromPack(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	head := repo.branches["main"]

	result, err := repo.GC(GCOptions{})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if result.PackedObjects == 0 {
		t.Fatal("GC did not pack reachable loose objects")
	}
	manifest, err := repo.objectStore.readPackManifest()
	if err != nil {
		t.Fatalf("read pack manifest: %v", err)
	}
	if len(manifest.Packs) != 1 {
		t.Fatalf("manifest packs = %d, want 1", len(manifest.Packs))
	}
	path, err := repo.objectStore.path(head)
	if err != nil {
		t.Fatalf("head path: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("packed head loose file still exists: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository with only packed objects: %v", err)
	}
	closeTestRepository(t, reopened)
	delete(reopened.objects, head)
	delete(reopened.objectStore.types, head)
	if _, err := reopened.objectStore.get(head, "commit"); err != nil {
		t.Fatalf("read packed commit after cache eviction: %v", err)
	}
}

func TestGCPrunesOnlyGraceExpiredUnreachableLooseObjects(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	closeTestRepository(t, repo)

	oldID, err := repo.objectStore.put("unreachable", map[string]string{"age": "old"})
	if err != nil {
		t.Fatalf("store old object: %v", err)
	}
	youngID, err := repo.objectStore.put("unreachable", map[string]string{"age": "young"})
	if err != nil {
		t.Fatalf("store young object: %v", err)
	}
	oldPath, err := repo.objectStore.path(oldID)
	if err != nil {
		t.Fatalf("old path: %v", err)
	}
	youngPath, err := repo.objectStore.path(youngID)
	if err != nil {
		t.Fatalf("young path: %v", err)
	}
	oldTime := time.Now().Add(-DefaultGCGracePeriod - time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("age old object: %v", err)
	}

	result, err := repo.GC(GCOptions{})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if result.PrunedLooseObjects != 1 {
		t.Fatalf("pruned loose objects = %d, want 1", result.PrunedLooseObjects)
	}
	if result.RetainedUnreachableObjects != 1 {
		t.Fatalf("retained unreachable objects = %d, want 1", result.RetainedUnreachableObjects)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expired loose object remains: %v", err)
	}
	if _, err := os.Stat(youngPath); err != nil {
		t.Fatalf("young loose object was not preserved: %v", err)
	}
}

func TestGCLeavesLooseObjectsUntouchedWhenLooseStateIsMalformed(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	closeTestRepository(t, repo)
	oldID, err := repo.objectStore.put("unreachable", map[string]string{"age": "old"})
	if err != nil {
		t.Fatalf("store old object: %v", err)
	}
	oldPath, err := repo.objectStore.path(oldID)
	if err != nil {
		t.Fatalf("old path: %v", err)
	}
	oldTime := time.Now().Add(-DefaultGCGracePeriod - time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("age old object: %v", err)
	}
	badID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	badPath := filepath.Join(repo.objectStore.looseDir, badID[:2], badID[2:])
	if err := os.MkdirAll(filepath.Dir(badPath), 0o700); err != nil {
		t.Fatalf("create malformed object parent: %v", err)
	}
	if err := os.WriteFile(badPath, []byte("not an object envelope"), 0o600); err != nil {
		t.Fatalf("write malformed object: %v", err)
	}

	if _, err := repo.GC(GCOptions{}); !errors.Is(err, ErrGCCorrupt) {
		t.Fatalf("GC malformed loose state error = %v, want ErrGCCorrupt", err)
	}
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("GC pruned loose object despite malformed state: %v", err)
	}
}

func TestGCCleansLooseCopyLeftAfterPublishedPack(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	head := repo.branches["main"]
	if _, err := repo.GC(GCOptions{}); err != nil {
		t.Fatalf("initial GC: %v", err)
	}
	objectType, data, err := repo.objectStore.getAny(head)
	if err != nil {
		t.Fatalf("read packed head: %v", err)
	}
	path, err := repo.objectStore.path(head)
	if err != nil {
		t.Fatalf("head path: %v", err)
	}
	if err := repo.objectStore.ensureDurable(path, head, objectType, data); err != nil {
		t.Fatalf("restore duplicate loose object: %v", err)
	}

	result, err := repo.GC(GCOptions{})
	if err != nil {
		t.Fatalf("GC recovery cleanup: %v", err)
	}
	if result.PackedObjects != 0 {
		t.Fatalf("recovery cleanup packed %d objects, want 0", result.PackedObjects)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("duplicate loose object remains after cleanup: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository after cleanup recovery: %v", err)
	}
	closeTestRepository(t, reopened)
}

func TestGCCleanupFailureAfterPublicationReturnsWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions do not reliably prevent removal on Windows")
	}
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	closeTestRepository(t, repo)
	head := repo.branches["main"]
	if _, err := repo.GC(GCOptions{}); err != nil {
		t.Fatalf("initial GC: %v", err)
	}
	objectType, data, err := repo.objectStore.getAny(head)
	if err != nil {
		t.Fatalf("read packed head: %v", err)
	}
	path, err := repo.objectStore.path(head)
	if err != nil {
		t.Fatalf("head path: %v", err)
	}
	if err := repo.objectStore.ensureDurable(path, head, objectType, data); err != nil {
		t.Fatalf("restore duplicate loose object: %v", err)
	}
	parent := filepath.Dir(path)
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatalf("make loose directory read-only: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(parent, 0o700); err != nil {
			t.Errorf("restore loose directory permissions: %v", err)
		}
	})

	result, err := repo.GC(GCOptions{})
	var warning *GCCommittedWithWarningError
	if !errors.As(err, &warning) {
		t.Fatalf("GC cleanup error = %v, want GCCommittedWithWarningError", err)
	}
	if warning.Result != result {
		t.Fatalf("warning result = %#v, want %#v", warning.Result, result)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("failed cleanup removed loose object: %v", err)
	}
}

func TestGCRepackPublishesOneReplacementGeneration(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	closeTestRepository(t, repo)
	if _, err := repo.GC(GCOptions{}); err != nil {
		t.Fatalf("initial GC: %v", err)
	}
	if _, err := repo.AdvanceBranch("main"); err != nil {
		t.Fatalf("AdvanceBranch: %v", err)
	}
	if _, err := repo.GC(GCOptions{}); err != nil {
		t.Fatalf("pack new loose commit: %v", err)
	}
	before, err := repo.objectStore.readPackManifest()
	if err != nil {
		t.Fatalf("read pre-repack manifest: %v", err)
	}
	if len(before.Packs) != 2 {
		t.Fatalf("pre-repack packs = %d, want 2", len(before.Packs))
	}

	result, err := repo.GC(GCOptions{Repack: true})
	if err != nil {
		t.Fatalf("repack: %v", err)
	}
	if result.RetiredPacks != uint64(len(before.Packs)) {
		t.Fatalf("retired packs = %d, want %d", result.RetiredPacks, len(before.Packs))
	}
	after, err := repo.objectStore.readPackManifest()
	if err != nil {
		t.Fatalf("read replacement manifest: %v", err)
	}
	if len(after.Packs) != 1 {
		t.Fatalf("replacement manifest packs = %d, want 1", len(after.Packs))
	}
	for _, pack := range before.Packs {
		path, err := repo.objectStore.packPath(pack.ID)
		if err != nil {
			t.Fatalf("old pack path: %v", err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("superseded pack %q remains: %v", pack.ID, err)
		}
	}
}

func TestGCRepackPreservesAllActivePackObjects(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	closeTestRepository(t, repo)

	orphan, err := repo.objectStore.put("node", Node{ID: "packed-orphan", Title: "packed orphan"})
	if err != nil {
		t.Fatalf("store orphan object: %v", err)
	}
	objectType, data, err := repo.objectStore.getAny(orphan)
	if err != nil {
		t.Fatalf("read orphan object: %v", err)
	}
	if _, err := repo.GC(GCOptions{}); err != nil {
		t.Fatalf("pack reachable objects: %v", err)
	}
	manifest, err := repo.objectStore.readPackManifest()
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if _, err := repo.objectStore.writeAndPublishPack(map[ObjectID]verifiedObject{
		orphan: {objectType: objectType, data: data},
	}, manifest, false); err != nil {
		t.Fatalf("publish orphan pack: %v", err)
	}
	orphanPath, err := repo.objectStore.path(orphan)
	if err != nil {
		t.Fatalf("orphan path: %v", err)
	}
	if err := os.Remove(orphanPath); err != nil {
		t.Fatalf("remove orphan loose copy: %v", err)
	}

	result, err := repo.GC(GCOptions{Repack: true})
	if err != nil {
		t.Fatalf("repack: %v", err)
	}
	if result.PackedObjects != result.ReachableObjects+1 {
		t.Fatalf("packed objects = %d, want reachable objects plus packed orphan (%d)", result.PackedObjects, result.ReachableObjects+1)
	}
	delete(repo.objects, orphan)
	delete(repo.objectStore.types, orphan)
	if _, err := repo.objectStore.get(orphan, "node"); err != nil {
		t.Fatalf("read orphan after repack: %v", err)
	}
}

type failingPackIndexStore struct{ err error }

func (s failingPackIndexStore) Open(path string) (packIndex, error) {
	return binaryPackIndexStore{}.Open(path)
}

func (s failingPackIndexStore) Write(string, packIndexMetadata, []PackIndexEntry) error {
	return s.err
}

type countingPackIndexStore struct {
	base       packIndexStore
	opens      int
	closes     int
	lookups    int
	failAt     int
	closeErrAt int
	err        error
}

func (s *countingPackIndexStore) Open(path string) (packIndex, error) {
	s.opens++
	if s.failAt == s.opens {
		return nil, s.err
	}
	index, err := s.base.Open(path)
	if err != nil {
		return nil, err
	}
	closeErr := error(nil)
	if s.closeErrAt == s.opens {
		closeErr = s.err
	}
	return countingPackIndex{
		packIndex: index,
		onClose:   func() { s.closes++ },
		onLookup:  func() { s.lookups++ },
		closeErr:  closeErr,
	}, nil
}

func (s *countingPackIndexStore) Write(path string, metadata packIndexMetadata, entries []PackIndexEntry) error {
	return s.base.Write(path, metadata, entries)
}

type countingPackIndex struct {
	packIndex
	onClose  func()
	onLookup func()
	closeErr error
}

func (i countingPackIndex) Lookup(id ObjectID) (PackIndexEntry, bool, error) {
	i.onLookup()
	return i.packIndex.Lookup(id)
}

func (i countingPackIndex) Close() error {
	i.onClose()
	return errors.Join(i.packIndex.Close(), i.closeErr)
}

func TestPackedReadsReuseGenerationIndexesAndCloseThemWithRepository(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	head := repo.branches["main"]
	if _, err := repo.GC(GCOptions{}); err != nil {
		t.Fatalf("GC: %v", err)
	}
	indexes := &countingPackIndexStore{base: binaryPackIndexStore{}}
	repo.objectStore.packIndexes = indexes
	delete(repo.objects, head)
	delete(repo.objectStore.types, head)

	if _, err := repo.objectStore.get(head, "commit"); err != nil {
		t.Fatalf("first packed read: %v", err)
	}
	if indexes.opens != 1 {
		t.Fatalf("first packed read opened %d indexes, want 1", indexes.opens)
	}
	if len(repo.objectStore.packGeneration.readers) != 1 {
		t.Fatalf("generation readers = %d, want 1", len(repo.objectStore.packGeneration.readers))
	}
	manifest, err := repo.objectStore.readPackManifest()
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	reader := repo.objectStore.packGeneration.readers[manifest.Packs[0].ID]
	if reader == nil {
		t.Fatal("generation is missing the active pack reader")
	}
	delete(repo.objects, head)
	delete(repo.objectStore.types, head)
	if _, err := repo.objectStore.get(head, "commit"); err != nil {
		t.Fatalf("second packed read: %v", err)
	}
	if indexes.opens != 1 {
		t.Fatalf("second packed read opened %d indexes, want 1", indexes.opens)
	}
	if indexes.lookups != 0 {
		t.Fatalf("packed reads performed %d index lookups, want direct locator only", indexes.lookups)
	}

	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if indexes.closes != 1 {
		t.Fatalf("Close closed %d indexes, want 1", indexes.closes)
	}
	if _, err := reader.file.Stat(); err == nil {
		t.Fatal("Close left the generation pack reader open")
	}
}

func TestConcurrentPackedReadsInitializeOneGeneration(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	closeTestRepository(t, repo)
	head := repo.branches["main"]
	if _, err := repo.GC(GCOptions{}); err != nil {
		t.Fatalf("GC: %v", err)
	}
	indexes := &countingPackIndexStore{base: binaryPackIndexStore{}}
	repo.objectStore.packIndexes = indexes
	delete(repo.objects, head)
	delete(repo.objectStore.types, head)

	var start sync.WaitGroup
	start.Add(1)
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			start.Wait()
			_, err := repo.objectStore.get(head, "commit")
			errs <- err
		}()
	}
	start.Done()
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent packed read: %v", err)
		}
	}
	if indexes.opens != 1 {
		t.Fatalf("concurrent packed reads opened %d indexes, want 1", indexes.opens)
	}
}

func TestGCInvalidatesGenerationAfterCommittedManifestWarning(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	closeTestRepository(t, repo)
	head := repo.branches["main"]
	if _, err := repo.GC(GCOptions{}); err != nil {
		t.Fatalf("initial GC: %v", err)
	}
	delete(repo.objects, head)
	delete(repo.objectStore.types, head)
	if _, err := repo.objectStore.get(head, "commit"); err != nil {
		t.Fatalf("read packed head: %v", err)
	}
	injected := errors.New("injected manifest directory sync warning")
	repo.objectStore.writeStateFile = func(path string, data []byte) error {
		if err := writeDurableStateFile(path, data); err != nil {
			return err
		}
		return durableWriteCommittedError{err: injected}
	}
	if _, err := repo.AdvanceBranch("main"); err != nil {
		t.Fatalf("AdvanceBranch: %v", err)
	}
	newHead := repo.branches["main"]

	result, err := repo.GC(GCOptions{})
	var warning *GCCommittedWithWarningError
	if !errors.As(err, &warning) {
		t.Fatalf("GC error = %v, want GCCommittedWithWarningError", err)
	}
	if !errors.Is(err, injected) || result.PackedObjects == 0 || warning.Result != result {
		t.Fatalf("GC result = %#v, error = %v, want committed warning", result, err)
	}
	if repo.objectStore.packGeneration != nil {
		t.Fatal("committed manifest warning retained the old generation")
	}
	delete(repo.objects, newHead)
	delete(repo.objectStore.types, newHead)
	if _, err := repo.objectStore.get(newHead, "commit"); err != nil {
		t.Fatalf("read newly packed head after warning: %v", err)
	}
}

func TestPackedObjectLocatorRetainsEquivalentDuplicateLocations(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	closeTestRepository(t, repo)
	head := repo.branches["main"]
	if _, err := repo.GC(GCOptions{}); err != nil {
		t.Fatalf("initial GC: %v", err)
	}
	objectType, data, err := repo.objectStore.getAny(head)
	if err != nil {
		t.Fatalf("read packed head: %v", err)
	}
	manifest, err := repo.objectStore.readPackManifest()
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if _, err := repo.objectStore.writeAndPublishPack(map[ObjectID]verifiedObject{
		head: {objectType: objectType, data: data},
	}, manifest, false); err != nil {
		t.Fatalf("publish equivalent duplicate: %v", err)
	}
	delete(repo.objects, head)
	delete(repo.objectStore.types, head)

	if _, err := repo.objectStore.get(head, "commit"); err != nil {
		t.Fatalf("read equivalent duplicate: %v", err)
	}
	if got := len(repo.objectStore.packGeneration.objects[head]); got != 2 {
		t.Fatalf("locator duplicate locations = %d, want 2", got)
	}
}

func TestPackedObjectLocatorReportsMissingObject(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	closeTestRepository(t, repo)
	if _, err := repo.GC(GCOptions{}); err != nil {
		t.Fatalf("GC: %v", err)
	}
	missing := ObjectID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	if _, err := repo.objectStore.get(missing, "commit"); !errors.Is(err, errLooseObjectNotFound) {
		t.Fatalf("read missing object error = %v, want errLooseObjectNotFound", err)
	}
	if _, exists := repo.objectStore.packGeneration.objects[missing]; exists {
		t.Fatal("locator contains missing object")
	}
}

func TestFailedPackGenerationInitializationClosesOpenedIndexes(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	closeTestRepository(t, repo)
	if _, err := repo.GC(GCOptions{}); err != nil {
		t.Fatalf("initial GC: %v", err)
	}
	if _, err := repo.AdvanceBranch("main"); err != nil {
		t.Fatalf("AdvanceBranch: %v", err)
	}
	head := repo.branches["main"]
	if _, err := repo.GC(GCOptions{}); err != nil {
		t.Fatalf("second GC: %v", err)
	}
	injected := errors.New("injected index open failure")
	indexes := &countingPackIndexStore{base: binaryPackIndexStore{}, failAt: 2, err: injected}
	repo.objectStore.packIndexes = indexes
	delete(repo.objects, head)
	delete(repo.objectStore.types, head)

	if _, err := repo.objectStore.get(head, "commit"); !errors.Is(err, injected) {
		t.Fatalf("packed read error = %v, want injected index open failure", err)
	}
	if indexes.opens != 2 {
		t.Fatalf("generation initialization opened %d indexes, want 2", indexes.opens)
	}
	if indexes.closes != 1 {
		t.Fatalf("failed generation initialization closed %d indexes, want 1", indexes.closes)
	}
	if repo.objectStore.packGeneration != nil {
		t.Fatal("failed generation initialization retained a cache")
	}
}

func TestGCReportsCloseFailureAfterPublishingPackGeneration(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	closeTestRepository(t, repo)
	head := repo.branches["main"]
	if _, err := repo.GC(GCOptions{}); err != nil {
		t.Fatalf("initial GC: %v", err)
	}
	injected := errors.New("injected index close failure")
	indexes := &countingPackIndexStore{base: binaryPackIndexStore{}, closeErrAt: 1, err: injected}
	repo.objectStore.packIndexes = indexes
	delete(repo.objects, head)
	delete(repo.objectStore.types, head)
	if _, err := repo.objectStore.get(head, "commit"); err != nil {
		t.Fatalf("read packed head: %v", err)
	}
	if _, err := repo.AdvanceBranch("main"); err != nil {
		t.Fatalf("AdvanceBranch: %v", err)
	}

	result, err := repo.GC(GCOptions{})
	var warning *GCCommittedWithWarningError
	if !errors.As(err, &warning) {
		t.Fatalf("GC error = %v, want GCCommittedWithWarningError", err)
	}
	if !errors.Is(err, injected) {
		t.Fatalf("GC error = %v, want injected close failure", err)
	}
	if result.PackedObjects == 0 || warning.Result != result {
		t.Fatalf("GC result = %#v, warning result = %#v, want published pack result", result, warning.Result)
	}
	manifest, err := repo.objectStore.readPackManifest()
	if err != nil {
		t.Fatalf("read published manifest: %v", err)
	}
	if len(manifest.Packs) != 2 {
		t.Fatalf("published manifest packs = %d, want 2", len(manifest.Packs))
	}
}

func TestGCPackIndexFailureLeavesOldGenerationReadable(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	head := repo.branches["main"]
	headPath, err := repo.objectStore.path(head)
	if err != nil {
		t.Fatalf("head path: %v", err)
	}
	injected := errors.New("injected index write failure")
	repo.objectStore.packIndexes = failingPackIndexStore{err: injected}

	result, err := repo.GC(GCOptions{})
	if !errors.Is(err, injected) {
		t.Fatalf("GC error = %v, want injected index write failure", err)
	}
	if result.PackedObjects != 0 || result.PrunedLooseObjects != 0 || result.ReclaimedBytes != 0 {
		t.Fatalf("failed GC result = %#v, want no completed maintenance", result)
	}
	if _, err := os.Stat(headPath); err != nil {
		t.Fatalf("failed GC removed reachable loose object: %v", err)
	}
	manifest, err := repo.objectStore.readPackManifest()
	if err != nil {
		t.Fatalf("read manifest after failed GC: %v", err)
	}
	if len(manifest.Packs) != 0 {
		t.Fatalf("failed GC published %d packs, want none", len(manifest.Packs))
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("close repository: %v", err)
	}
	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("reopen repository after failed GC: %v", err)
	}
	closeTestRepository(t, reopened)
}

func TestGCMissingActivePackPreventsLoosePruning(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	closeTestRepository(t, repo)
	head := repo.branches["main"]
	orphan, err := repo.objectStore.put("unreachable", map[string]string{"age": "old"})
	if err != nil {
		t.Fatalf("store orphan object: %v", err)
	}
	if _, err := repo.GC(GCOptions{}); err != nil {
		t.Fatalf("initial GC: %v", err)
	}
	orphanPath, err := repo.objectStore.path(orphan)
	if err != nil {
		t.Fatalf("orphan path: %v", err)
	}
	oldTime := time.Now().Add(-DefaultGCGracePeriod - time.Hour)
	if err := os.Chtimes(orphanPath, oldTime, oldTime); err != nil {
		t.Fatalf("age orphan: %v", err)
	}
	objectType, data, err := repo.objectStore.getAny(head)
	if err != nil {
		t.Fatalf("read packed head: %v", err)
	}
	headPath, err := repo.objectStore.path(head)
	if err != nil {
		t.Fatalf("head path: %v", err)
	}
	if err := repo.objectStore.ensureDurable(headPath, head, objectType, data); err != nil {
		t.Fatalf("restore head loose copy: %v", err)
	}
	manifest, err := repo.objectStore.readPackManifest()
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	packPath, err := repo.objectStore.packPath(manifest.Packs[0].ID)
	if err != nil {
		t.Fatalf("active pack path: %v", err)
	}
	if err := os.Remove(packPath); err != nil {
		t.Fatalf("remove active pack: %v", err)
	}

	if _, err := repo.GC(GCOptions{}); !errors.Is(err, ErrGCCorrupt) {
		t.Fatalf("GC error = %v, want ErrGCCorrupt", err)
	}
	if _, err := os.Stat(orphanPath); err != nil {
		t.Fatalf("GC pruned old loose object despite missing active pack: %v", err)
	}
}

func TestPackedDuplicateMismatchIsRejected(t *testing.T) {
	id := ObjectID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	objects := map[ObjectID]verifiedObject{
		id: {objectType: "first", data: []byte("first")},
	}
	err := addPackedObject(objects, PackID("0123456789abcdef0123456789abcdef"), PackIndexEntry{
		Object: id, Offset: packHeaderSize, CompressedSize: 1, UncompressedSize: 1,
	}, verifiedObject{objectType: "second", data: []byte("second")})
	if !errors.Is(err, ErrPackCorrupt) {
		t.Fatalf("duplicate mismatch error = %v, want ErrPackCorrupt", err)
	}
}

func TestUnpublishedPackFilesAreIgnoredOnOpen(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	if err := repo.objectStore.ensurePackDirectories(); err != nil {
		t.Fatalf("ensure pack directories: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo.objectStore.packDirectory(), "orphan.pack"), []byte("partial"), 0o600); err != nil {
		t.Fatalf("write orphan pack: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository ignored unpublished pack incorrectly: %v", err)
	}
	closeTestRepository(t, reopened)
}

func TestReadPackManifestRejectsMissingOrNullPackList(t *testing.T) {
	for name, data := range map[string][]byte{
		"missing": []byte(`{"version":1}`),
		"null":    []byte(`{"version":1,"packs":null}`),
	} {
		t.Run(name, func(t *testing.T) {
			stateDir := t.TempDir()
			cache := make(map[ObjectID][]byte)
			store := newLooseObjectStore(stateDir, &cache)
			if err := store.ensurePackDirectories(); err != nil {
				t.Fatalf("ensure pack directories: %v", err)
			}
			if err := os.WriteFile(store.packManifestPath(), data, 0o600); err != nil {
				t.Fatalf("write manifest: %v", err)
			}
			if _, err := store.readPackManifest(); !errors.Is(err, ErrPackCorrupt) {
				t.Fatalf("read manifest error = %v, want ErrPackCorrupt", err)
			}
		})
	}
}
