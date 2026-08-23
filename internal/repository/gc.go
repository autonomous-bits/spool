package repository

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

type looseObjectFile struct {
	id      ObjectID
	object  verifiedObject
	path    string
	size    int64
	modTime time.Time
}

// GC packs reachable durable objects and prunes only grace-expired unreachable
// loose objects. A durable repository's process lock is held for its lifetime;
// the repository mutex serializes this operation with in-process mutations.
func (r *Repository) GC(options GCOptions) (GCResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpenLocked(); err != nil {
		return GCResult{}, err
	}
	if options.GracePeriod < 0 {
		return GCResult{}, fmt.Errorf("GC grace period must not be negative")
	}
	grace := options.GracePeriod
	if grace == 0 {
		grace = DefaultGCGracePeriod
	}

	scan, err := r.scanRetentionLocked()
	if err != nil {
		return GCResult{}, gcCorruption(err)
	}
	result := GCResult{Roots: uint64(len(scan.roots)), ReachableObjects: uint64(len(scan.reachable))}
	if r.mergeStateDir == "" {
		return result, nil
	}

	manifest, err := r.objectStore.readPackManifest()
	if err != nil {
		return result, gcCorruption(err)
	}
	loose, err := r.objectStore.scanLooseObjectFiles()
	if err != nil {
		return result, gcCorruption(err)
	}
	active, err := collectPackedObjects(r.objectStore, manifest)
	if err != nil {
		return result, gcCorruption(err)
	}

	replacement := make(map[ObjectID]verifiedObject)
	postPublish := make(map[ObjectID]verifiedObject)
	if options.Repack {
		for id, object := range active {
			replacement[id] = object
			postPublish[id] = object
			if looseFile, exists := loose[id]; exists && !sameVerifiedObject(object, looseFile.object) {
				return result, gcCorruption(&PackCorruptionError{
					Object: id, Detail: "loose and packed copies have different payloads or types",
				})
			}
		}
	}
	for id := range scan.reachable {
		packed, packedOK := active[id]
		looseFile, looseOK := loose[id]
		if !packedOK && !looseOK {
			return result, gcCorruption(fmt.Errorf("reachable object %s has no durable location", id))
		}
		if packedOK && looseOK && !sameVerifiedObject(packed, looseFile.object) {
			return result, gcCorruption(&PackCorruptionError{
				Object: id, Detail: "loose and packed copies have different payloads or types",
			})
		}
		switch {
		case options.Repack:
			if !packedOK {
				replacement[id] = looseFile.object
				postPublish[id] = looseFile.object
			}
		case packedOK:
			postPublish[id] = packed
		case looseOK:
			replacement[id] = looseFile.object
			postPublish[id] = looseFile.object
		}
	}

	now := r.now()
	duplicateLoose, staleLoose := make([]looseObjectFile, 0), make([]looseObjectFile, 0)
	for _, file := range loose {
		if _, reachable := scan.reachable[file.id]; reachable {
			if _, packed := postPublish[file.id]; packed {
				duplicateLoose = append(duplicateLoose, file)
			}
			continue
		}
		if file.modTime.Before(now.Add(-grace)) {
			staleLoose = append(staleLoose, file)
		} else {
			result.RetainedUnreachableObjects++
		}
	}
	sortLooseObjectFiles(duplicateLoose)
	sortLooseObjectFiles(staleLoose)
	var plannedReclaimed uint64
	for _, file := range duplicateLoose {
		plannedReclaimed += uint64(file.size)
	}
	for _, file := range staleLoose {
		plannedReclaimed += uint64(file.size)
	}
	if options.DryRun {
		result.PackedObjects = uint64(len(replacement))
		result.PrunedLooseObjects = uint64(len(staleLoose))
		result.ReclaimedBytes = plannedReclaimed
		if options.Repack {
			result.RetiredPacks = uint64(len(manifest.Packs))
		}
		return result, nil
	}

	published := false
	previousManifest := manifest
	var cleanupErr error
	if len(replacement) != 0 {
		metadata, err := r.objectStore.writeAndPublishPack(replacement, manifest, options.Repack)
		if err != nil {
			var closeErr *packGenerationCloseError
			if !errors.As(err, &closeErr) {
				return result, err
			}
			cleanupErr = errors.Join(cleanupErr, err)
		}
		published = true
		result.PackedObjects = uint64(len(replacement))
		if options.Repack {
			manifest = PackManifest{Version: PackManifestFormatVersion, Packs: []PackMetadata{metadata}}
		} else {
			manifest.Packs = append(manifest.Packs, metadata)
		}
	}

	var removedBytes uint64
	if len(duplicateLoose) != 0 {
		count, bytes, err := removeLooseObjectFiles(duplicateLoose)
		_ = count
		removedBytes += bytes
		cleanupErr = errors.Join(cleanupErr, err)
	}
	if len(staleLoose) != 0 {
		count, bytes, err := removeLooseObjectFiles(staleLoose)
		result.PrunedLooseObjects = count
		removedBytes += bytes
		cleanupErr = errors.Join(cleanupErr, err)
	}
	result.ReclaimedBytes = removedBytes
	if options.Repack && published {
		retired, bytes, err := r.objectStore.retirePacks(previousManifest.Packs)
		result.RetiredPacks = retired
		result.ReclaimedBytes += bytes
		cleanupErr = errors.Join(cleanupErr, err)
	}
	if cleanupErr != nil {
		if published || len(manifest.Packs) != 0 {
			return result, &GCCommittedWithWarningError{Result: result, Err: cleanupErr}
		}
		return result, cleanupErr
	}
	return result, nil
}

func gcCorruption(err error) error {
	if errors.Is(err, ErrGCCorrupt) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrGCCorrupt, err)
}

func sameVerifiedObject(left, right verifiedObject) bool {
	return left.objectType == right.objectType && bytes.Equal(left.data, right.data)
}

func (s *looseObjectStore) scanLooseObjectFiles() (map[ObjectID]looseObjectFile, error) {
	if s.looseDir == "" {
		return nil, errors.New("loose object store is not durable")
	}
	rootInfo, err := os.Lstat(s.looseDir)
	if err != nil {
		return nil, fmt.Errorf("inspect loose object directory: %w", err)
	}
	if !rootInfo.IsDir() {
		return nil, errors.New("loose object path is not a directory")
	}
	files := make(map[ObjectID]looseObjectFile)
	err = filepath.WalkDir(s.looseDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(s.looseDir, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		if entry.IsDir() {
			if strings.Contains(relative, string(filepath.Separator)) || filepath.Base(relative) != relative ||
				len(relative) != 2 || !validLooseObjectID(ObjectID(relative+"00000000000000000000000000000000000000000000000000000000000000")) {
				return fmt.Errorf("invalid loose object directory %q", relative)
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("loose object %q is not a regular file", relative)
		}
		relativeSlash := filepath.ToSlash(relative)
		parts := strings.Split(relativeSlash, "/")
		if len(parts) != 2 || len(parts[0]) != 2 || len(parts[1]) != objectIDHexLength-2 {
			return fmt.Errorf("invalid loose object path %q", relativeSlash)
		}
		id := ObjectID(parts[0] + parts[1])
		if !validLooseObjectID(id) {
			return fmt.Errorf("invalid loose object ID %q", id)
		}
		objectType, data, err := readLooseObjectAny(path, id)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if _, duplicate := files[id]; duplicate {
			return fmt.Errorf("duplicate loose object %s", id)
		}
		files[id] = looseObjectFile{
			id: id, object: verifiedObject{objectType: objectType, data: data},
			path: path, size: info.Size(), modTime: info.ModTime(),
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan loose objects: %w", err)
	}
	return files, nil
}

func collectPackedObjects(store *looseObjectStore, manifest PackManifest) (map[ObjectID]verifiedObject, error) {
	objects := make(map[ObjectID]verifiedObject)
	for _, metadata := range manifest.Packs {
		index, err := store.openPackIndex(metadata)
		if err != nil {
			return nil, err
		}
		packPath, err := store.packPath(metadata.ID)
		if err == nil {
			err = index.ForEach(func(entry PackIndexEntry) error {
				objectType, data, err := readPackedObjectFile(metadata.ID, packPath, entry, metadata.ObjectCount)
				if err != nil {
					return err
				}
				return addPackedObject(objects, metadata.ID, entry, verifiedObject{objectType: objectType, data: data})
			})
		}
		closeErr := index.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close pack index: %w", closeErr)
		}
	}
	return objects, nil
}

func addPackedObject(objects map[ObjectID]verifiedObject, packID PackID, entry PackIndexEntry, object verifiedObject) error {
	if existing, duplicate := objects[entry.Object]; duplicate && !sameVerifiedObject(existing, object) {
		return &PackCorruptionError{
			Pack: packID, Object: entry.Object, Offset: entry.Offset,
			Detail: "duplicate object has different payload or type",
		}
	}
	objects[entry.Object] = object
	return nil
}

func (s *looseObjectStore) writeAndPublishPack(
	objects map[ObjectID]verifiedObject, previous PackManifest, replace bool,
) (metadata PackMetadata, err error) {
	if err := s.ensurePackDirectories(); err != nil {
		return PackMetadata{}, err
	}
	packID, err := newPackID()
	if err != nil {
		return PackMetadata{}, err
	}
	packPath, err := s.packPath(packID)
	if err != nil {
		return PackMetadata{}, err
	}
	indexPath, err := s.packIndexPath(packID)
	if err != nil {
		return PackMetadata{}, err
	}
	for _, path := range []string{packPath, indexPath} {
		if _, err := os.Lstat(path); err == nil {
			return PackMetadata{}, fmt.Errorf("pack generation %q already exists", packID)
		} else if !os.IsNotExist(err) {
			return PackMetadata{}, fmt.Errorf("inspect pack generation: %w", err)
		}
	}

	packFile, err := os.CreateTemp(s.packDirectory(), "."+string(packID)+"-*.pack")
	if err != nil {
		return PackMetadata{}, fmt.Errorf("create temporary pack: %w", err)
	}
	packTemp := packFile.Name()
	indexTemp := filepath.Join(s.packDirectory(), "."+string(packID)+".idx.tmp")
	defer func() {
		if packFile != nil {
			if closeErr := packFile.Close(); closeErr != nil && err == nil {
				err = fmt.Errorf("close temporary pack: %w", closeErr)
			}
		}
		for _, path := range []string{packTemp, indexTemp} {
			if path == "" {
				continue
			}
			if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) && err == nil {
				err = fmt.Errorf("remove incomplete pack generation: %w", removeErr)
			}
		}
	}()

	entries, err := writePackFile(packFile, packID, objects)
	if err != nil {
		return PackMetadata{}, err
	}
	if err := packFile.Sync(); err != nil {
		return PackMetadata{}, fmt.Errorf("sync temporary pack: %w", err)
	}
	if err := packFile.Close(); err != nil {
		return PackMetadata{}, fmt.Errorf("close temporary pack: %w", err)
	}
	packFile = nil
	metadata = PackMetadata{
		ID: packID, Version: PackFormatVersion, Compression: PackCompressionZstd, ObjectCount: uint32(len(entries)),
	}
	if err := s.packIndexes.Write(indexTemp, packIndexMetadata{
		Version: PackIndexFormatVersion, Pack: packID, ObjectCount: uint32(len(entries)),
	}, entries); err != nil {
		return PackMetadata{}, err
	}
	if err := verifyPackFiles(s.packIndexes, packID, packTemp, indexTemp); err != nil {
		return PackMetadata{}, err
	}

	if err := os.Rename(packTemp, packPath); err != nil {
		return PackMetadata{}, fmt.Errorf("publish pack: %w", err)
	}
	packTemp = ""
	if err := syncMergeStateDirectory(s.packDirectory()); err != nil {
		return PackMetadata{}, fmt.Errorf("sync published pack directory: %w", err)
	}
	if err := os.Rename(indexTemp, indexPath); err != nil {
		return PackMetadata{}, fmt.Errorf("publish pack index: %w", err)
	}
	indexTemp = ""
	if err := syncMergeStateDirectory(s.packDirectory()); err != nil {
		return PackMetadata{}, fmt.Errorf("sync published pack index directory: %w", err)
	}
	next := PackManifest{Version: PackManifestFormatVersion}
	if replace {
		next.Packs = []PackMetadata{metadata}
	} else {
		next.Packs = append(append([]PackMetadata(nil), previous.Packs...), metadata)
	}
	if err := s.writePackManifest(next); err != nil {
		return metadata, err
	}
	return metadata, nil
}

func newPackID() (PackID, error) {
	var random [16]byte
	if _, err := io.ReadFull(rand.Reader, random[:]); err != nil {
		return "", fmt.Errorf("generate pack ID: %w", err)
	}
	return PackID(hex.EncodeToString(random[:])), nil
}

func writePackFile(file *os.File, packID PackID, objects map[ObjectID]verifiedObject) ([]PackIndexEntry, error) {
	if uint64(len(objects)) > math.MaxUint32 {
		return nil, &PackCorruptionError{Pack: packID, Detail: "too many objects for one pack"}
	}
	var header [packHeaderSize]byte
	copy(header[:4], PackMagic)
	binary.BigEndian.PutUint32(header[4:8], PackFormatVersion)
	binary.BigEndian.PutUint32(header[8:12], uint32(len(objects)))
	if _, err := file.Write(header[:]); err != nil {
		return nil, fmt.Errorf("write pack header: %w", err)
	}
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		return nil, fmt.Errorf("create zstd encoder: %w", err)
	}
	defer func() { _ = encoder.Close() }()
	ids := make([]ObjectID, 0, len(objects))
	for id := range objects {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	offset := uint64(packHeaderSize)
	entries := make([]PackIndexEntry, 0, len(ids))
	for _, id := range ids {
		object := objects[id]
		if !validLooseObjectID(id) || object.objectType == "" || objectIDForEncoded(object.objectType, object.data) != id {
			return nil, &PackCorruptionError{Pack: packID, Object: id, Offset: offset, Detail: "invalid object selected for packing"}
		}
		envelope, err := canonicalCBOR.Marshal(looseObjectEnvelope{Type: object.objectType, Data: object.data})
		if err != nil {
			return nil, fmt.Errorf("encode packed object %s: %w", id, err)
		}
		compressed := encoder.EncodeAll(envelope, nil)
		if len(compressed) == 0 || offset > math.MaxUint64-uint64(len(compressed)) {
			return nil, &PackCorruptionError{Pack: packID, Object: id, Offset: offset, Detail: "compressed object size overflows pack"}
		}
		if _, err := file.Write(compressed); err != nil {
			return nil, fmt.Errorf("write packed object %s: %w", id, err)
		}
		entries = append(entries, PackIndexEntry{
			Object: id, Offset: offset, CompressedSize: uint64(len(compressed)),
			UncompressedSize: uint64(len(envelope)), CRC32: crc32.ChecksumIEEE(compressed),
		})
		offset += uint64(len(compressed))
	}
	return entries, nil
}

func removeLooseObjectFiles(files []looseObjectFile) (uint64, uint64, error) {
	var count, reclaimed uint64
	directories := make(map[string]struct{})
	var result error
	for _, file := range files {
		info, err := os.Lstat(file.path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			result = errors.Join(result, fmt.Errorf("inspect loose object %s before removal: %w", file.id, err))
			continue
		}
		if !info.Mode().IsRegular() {
			result = errors.Join(result, fmt.Errorf("loose object %s changed to a non-regular file", file.id))
			continue
		}
		objectType, data, err := readLooseObjectAny(file.path, file.id)
		if err != nil || !sameVerifiedObject(file.object, verifiedObject{objectType: objectType, data: data}) {
			if err == nil {
				err = errors.New("object content changed")
			}
			result = errors.Join(result, fmt.Errorf("verify loose object %s before removal: %w", file.id, err))
			continue
		}
		if err := os.Remove(file.path); err != nil {
			result = errors.Join(result, fmt.Errorf("remove loose object %s: %w", file.id, err))
			continue
		}
		count++
		reclaimed += uint64(info.Size())
		directories[filepath.Dir(file.path)] = struct{}{}
	}
	for _, directory := range sortedDirectories(directories) {
		if err := syncMergeStateDirectory(directory); err != nil {
			result = errors.Join(result, fmt.Errorf("sync loose object directory: %w", err))
		}
	}
	return count, reclaimed, result
}

func (s *looseObjectStore) retirePacks(metadata []PackMetadata) (uint64, uint64, error) {
	var retired, reclaimed uint64
	var result error
	changed := false
	for _, pack := range metadata {
		complete := true
		for _, pathFor := range []func(PackID) (string, error){s.packPath, s.packIndexPath} {
			path, err := pathFor(pack.ID)
			if err != nil {
				result = errors.Join(result, err)
				complete = false
				continue
			}
			info, err := os.Lstat(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				result = errors.Join(result, fmt.Errorf("inspect retired pack %q: %w", pack.ID, err))
				complete = false
				continue
			}
			if !info.Mode().IsRegular() {
				result = errors.Join(result, fmt.Errorf("retired pack %q is not a regular file", pack.ID))
				complete = false
				continue
			}
			if err := os.Remove(path); err != nil {
				result = errors.Join(result, fmt.Errorf("remove retired pack %q: %w", pack.ID, err))
				complete = false
				continue
			}
			reclaimed += uint64(info.Size())
			changed = true
		}
		if complete {
			retired++
		}
	}
	if changed {
		if err := syncMergeStateDirectory(s.packDirectory()); err != nil {
			result = errors.Join(result, fmt.Errorf("sync retired pack directory: %w", err))
		}
	}
	return retired, reclaimed, result
}

func sortLooseObjectFiles(files []looseObjectFile) {
	sort.Slice(files, func(left, right int) bool { return files[left].id < files[right].id })
}

func sortedDirectories(directories map[string]struct{}) []string {
	result := make([]string, 0, len(directories))
	for directory := range directories {
		result = append(result, directory)
	}
	sort.Strings(result)
	return result
}
