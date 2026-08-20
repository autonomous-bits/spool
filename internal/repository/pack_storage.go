package repository

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/klauspost/compress/zstd"
)

const (
	packIndexMagic      = "IDGI"
	packIndexHeaderSize = 14
	packIndexEntrySize  = 60
	packHeaderSize      = 12
	maxPackIDLength     = 64
)

type memoryPackIndex struct {
	metadata packIndexMetadata
	entries  []PackIndexEntry
	byObject map[ObjectID]PackIndexEntry
}

func (i *memoryPackIndex) Metadata() packIndexMetadata { return i.metadata }

func (i *memoryPackIndex) Lookup(id ObjectID) (PackIndexEntry, bool, error) {
	entry, ok := i.byObject[id]
	return entry, ok, nil
}

func (i *memoryPackIndex) ForEach(fn func(PackIndexEntry) error) error {
	for _, entry := range i.entries {
		if err := fn(entry); err != nil {
			return err
		}
	}
	return nil
}

func (i *memoryPackIndex) Close() error { return nil }

// binaryPackIndexStore is a compact immutable sidecar index. Its entries are
// sorted by object ID, while the pack index offsets address compressed envelopes.
type binaryPackIndexStore struct{}

func (binaryPackIndexStore) Open(path string) (packIndex, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect pack index: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, &PackCorruptionError{Detail: "pack index is not a regular file"}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pack index: %w", err)
	}
	if len(data) < packIndexHeaderSize {
		return nil, &PackCorruptionError{Detail: "index is shorter than its header"}
	}
	if string(data[:4]) != packIndexMagic {
		return nil, &PackCorruptionError{Detail: "index has an invalid magic"}
	}
	version := binary.BigEndian.Uint32(data[4:8])
	if version != PackIndexFormatVersion {
		return nil, &UnsupportedPackVersionError{Format: "pack index", Version: version}
	}
	idLength := int(binary.BigEndian.Uint16(data[8:10]))
	objectCount := binary.BigEndian.Uint32(data[10:14])
	if idLength == 0 || idLength > maxPackIDLength || len(data) < packIndexHeaderSize+idLength {
		return nil, &PackCorruptionError{Detail: "index has an invalid pack ID length"}
	}
	packID := PackID(string(data[packIndexHeaderSize : packIndexHeaderSize+idLength]))
	if !validPackID(packID) {
		return nil, &PackCorruptionError{Pack: packID, Detail: "index has an invalid pack ID"}
	}
	entryStart := packIndexHeaderSize + idLength
	if uint64(objectCount) > uint64((len(data)-entryStart)/packIndexEntrySize) ||
		len(data)-entryStart != int(objectCount)*packIndexEntrySize {
		return nil, &PackCorruptionError{Pack: packID, Detail: "index length does not match its object count"}
	}
	entries := make([]PackIndexEntry, 0, objectCount)
	byObject := make(map[ObjectID]PackIndexEntry, objectCount)
	var previous ObjectID
	for offset := entryStart; offset < len(data); offset += packIndexEntrySize {
		rawID := data[offset : offset+32]
		id := ObjectID(hex.EncodeToString(rawID))
		entry := PackIndexEntry{
			Object:           id,
			Offset:           binary.BigEndian.Uint64(data[offset+32 : offset+40]),
			CompressedSize:   binary.BigEndian.Uint64(data[offset+40 : offset+48]),
			UncompressedSize: binary.BigEndian.Uint64(data[offset+48 : offset+56]),
			CRC32:            binary.BigEndian.Uint32(data[offset+56 : offset+60]),
		}
		if err := validatePackIndexEntry(packID, entry); err != nil {
			return nil, err
		}
		if previous != "" && previous >= id {
			return nil, &PackCorruptionError{Pack: packID, Object: id, Offset: entry.Offset, Detail: "index entries are not strictly sorted"}
		}
		previous = id
		entries = append(entries, entry)
		byObject[id] = entry
	}
	return &memoryPackIndex{
		metadata: packIndexMetadata{Version: version, Pack: packID, ObjectCount: objectCount},
		entries:  entries, byObject: byObject,
	}, nil
}

func (binaryPackIndexStore) Write(path string, metadata packIndexMetadata, entries []PackIndexEntry) (err error) {
	if err := validatePackIndexMetadata(metadata); err != nil {
		return err
	}
	if uint64(len(entries)) != uint64(metadata.ObjectCount) {
		return &PackCorruptionError{Pack: metadata.Pack, Detail: "index object count does not match entries"}
	}
	if len(metadata.Pack) > math.MaxUint16 {
		return &PackCorruptionError{Pack: metadata.Pack, Detail: "pack ID is too long"}
	}
	var previous ObjectID
	for _, entry := range entries {
		if err := validatePackIndexEntry(metadata.Pack, entry); err != nil {
			return err
		}
		if previous != "" && previous >= entry.Object {
			return &PackCorruptionError{Pack: metadata.Pack, Object: entry.Object, Offset: entry.Offset, Detail: "index entries are not strictly sorted"}
		}
		previous = entry.Object
	}

	var data bytes.Buffer
	data.Grow(packIndexHeaderSize + len(metadata.Pack) + len(entries)*packIndexEntrySize)
	data.WriteString(packIndexMagic)
	_ = binary.Write(&data, binary.BigEndian, metadata.Version)
	_ = binary.Write(&data, binary.BigEndian, uint16(len(metadata.Pack)))
	_ = binary.Write(&data, binary.BigEndian, metadata.ObjectCount)
	data.WriteString(string(metadata.Pack))
	for _, entry := range entries {
		rawID, _ := hex.DecodeString(string(entry.Object))
		data.Write(rawID)
		_ = binary.Write(&data, binary.BigEndian, entry.Offset)
		_ = binary.Write(&data, binary.BigEndian, entry.CompressedSize)
		_ = binary.Write(&data, binary.BigEndian, entry.UncompressedSize)
		_ = binary.Write(&data, binary.BigEndian, entry.CRC32)
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create pack index: %w", err)
	}
	remove := true
	closed := false
	defer func() {
		if !closed {
			closeErr := file.Close()
			closed = true
			if closeErr != nil && err == nil {
				err = fmt.Errorf("close pack index: %w", closeErr)
			}
		}
		if remove {
			if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) && err == nil {
				err = fmt.Errorf("remove incomplete pack index: %w", removeErr)
			}
		}
	}()
	if _, err = file.Write(data.Bytes()); err != nil {
		return fmt.Errorf("write pack index: %w", err)
	}
	if err = file.Sync(); err != nil {
		return fmt.Errorf("sync pack index: %w", err)
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("close pack index: %w", err)
	}
	closed = true
	remove = false
	return nil
}

func validatePackIndexMetadata(metadata packIndexMetadata) error {
	if metadata.Version != PackIndexFormatVersion {
		return &UnsupportedPackVersionError{Format: "pack index", Version: metadata.Version}
	}
	if !validPackID(metadata.Pack) {
		return &PackCorruptionError{Pack: metadata.Pack, Detail: "invalid pack ID"}
	}
	return nil
}

func validatePackIndexEntry(packID PackID, entry PackIndexEntry) error {
	if !validLooseObjectID(entry.Object) {
		return &PackCorruptionError{Pack: packID, Object: entry.Object, Offset: entry.Offset, Detail: "invalid object ID"}
	}
	if entry.Offset < packHeaderSize || entry.CompressedSize == 0 || entry.UncompressedSize == 0 ||
		entry.Offset > math.MaxInt64 || entry.CompressedSize > math.MaxInt64 || entry.UncompressedSize > math.MaxInt64 ||
		entry.Offset > math.MaxUint64-entry.CompressedSize {
		return &PackCorruptionError{Pack: packID, Object: entry.Object, Offset: entry.Offset, Detail: "invalid object entry bounds"}
	}
	return nil
}

func validPackID(id PackID) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(string(id))
	return err == nil && strings.ToLower(string(id)) == string(id)
}

func (s *looseObjectStore) packDirectory() string {
	if s.looseDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(s.looseDir), packDirectoryName)
}

func (s *looseObjectStore) packInfoDirectory() string {
	if s.looseDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(s.looseDir), packInfoDirectoryName)
}

func (s *looseObjectStore) packManifestPath() string {
	if s.looseDir == "" {
		return ""
	}
	return filepath.Join(s.packInfoDirectory(), packManifestFilename)
}

func (s *looseObjectStore) packPath(id PackID) (string, error) {
	if !validPackID(id) {
		return "", &PackCorruptionError{Pack: id, Detail: "invalid pack ID"}
	}
	return filepath.Join(s.packDirectory(), string(id)+packFileExtension), nil
}

func (s *looseObjectStore) packIndexPath(id PackID) (string, error) {
	if !validPackID(id) {
		return "", &PackCorruptionError{Pack: id, Detail: "invalid pack ID"}
	}
	return filepath.Join(s.packDirectory(), string(id)+packIndexFileExtension), nil
}

func (s *looseObjectStore) ensurePackDirectories() error {
	if s.looseDir == "" {
		return errors.New("pack storage is not durable")
	}
	for _, path := range []string{filepath.Dir(s.looseDir), s.packDirectory(), s.packInfoDirectory()} {
		if err := ensureDurableDirectory(path); err != nil {
			return fmt.Errorf("create pack directory: %w", err)
		}
	}
	return nil
}

func (s *looseObjectStore) readPackManifest() (PackManifest, error) {
	if s.looseDir == "" {
		return PackManifest{Version: PackManifestFormatVersion, Packs: []PackMetadata{}}, nil
	}
	path := s.packManifestPath()
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return PackManifest{Version: PackManifestFormatVersion, Packs: []PackMetadata{}}, nil
	}
	if err != nil {
		return PackManifest{}, fmt.Errorf("inspect pack manifest: %w", err)
	}
	if !info.Mode().IsRegular() {
		return PackManifest{}, &PackCorruptionError{Detail: "pack manifest is not a regular file"}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return PackManifest{}, fmt.Errorf("read pack manifest: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest PackManifest
	if err := decoder.Decode(&manifest); err != nil {
		return PackManifest{}, &PackCorruptionError{Detail: "decode pack manifest: " + err.Error()}
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return PackManifest{}, &PackCorruptionError{Detail: "pack manifest has trailing data"}
	}
	if err := validatePackManifest(manifest); err != nil {
		return PackManifest{}, err
	}
	return manifest, nil
}

func validatePackManifest(manifest PackManifest) error {
	if manifest.Version != PackManifestFormatVersion {
		return &UnsupportedPackVersionError{Format: "pack manifest", Version: manifest.Version}
	}
	if manifest.Packs == nil {
		return &PackCorruptionError{Detail: "manifest packs must be an array"}
	}
	seen := make(map[PackID]struct{}, len(manifest.Packs))
	for _, metadata := range manifest.Packs {
		if !validPackID(metadata.ID) {
			return &PackCorruptionError{Pack: metadata.ID, Detail: "manifest has an invalid pack ID"}
		}
		if metadata.Version != PackFormatVersion {
			return &UnsupportedPackVersionError{Format: "pack", Version: metadata.Version}
		}
		if metadata.Compression != PackCompressionZstd {
			return &PackCorruptionError{Pack: metadata.ID, Detail: "manifest has an unsupported compression"}
		}
		if _, duplicate := seen[metadata.ID]; duplicate {
			return &PackCorruptionError{Pack: metadata.ID, Detail: "manifest lists a pack more than once"}
		}
		seen[metadata.ID] = struct{}{}
	}
	return nil
}

func (s *looseObjectStore) writePackManifest(manifest PackManifest) error {
	if err := validatePackManifest(manifest); err != nil {
		return err
	}
	if err := s.ensurePackDirectories(); err != nil {
		return err
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode pack manifest: %w", err)
	}
	if err := writeDurableStateFile(s.packManifestPath(), data); err != nil {
		return fmt.Errorf("publish pack manifest: %w", err)
	}
	return nil
}

func (s *looseObjectStore) readPackedObject(id ObjectID) (string, []byte, error) {
	manifest, err := s.readPackManifest()
	if err != nil {
		return "", nil, err
	}
	var found bool
	var objectType string
	var objectData []byte
	for _, metadata := range manifest.Packs {
		index, err := s.openPackIndex(metadata)
		if err != nil {
			return "", nil, err
		}
		entry, exists, lookupErr := index.Lookup(id)
		closeErr := index.Close()
		if lookupErr != nil {
			return "", nil, fmt.Errorf("lookup pack index: %w", lookupErr)
		}
		if closeErr != nil {
			return "", nil, fmt.Errorf("close pack index: %w", closeErr)
		}
		if !exists {
			continue
		}
		packPath, err := s.packPath(metadata.ID)
		if err != nil {
			return "", nil, err
		}
		nextType, nextData, err := readPackedObjectFile(metadata.ID, packPath, entry, metadata.ObjectCount)
		if err != nil {
			return "", nil, err
		}
		if found && (objectType != nextType || !bytes.Equal(objectData, nextData)) {
			return "", nil, &PackCorruptionError{Pack: metadata.ID, Object: id, Offset: entry.Offset, Detail: "duplicate object has different payload or type"}
		}
		found, objectType, objectData = true, nextType, nextData
	}
	if !found {
		return "", nil, fmt.Errorf("%w: %s", errLooseObjectNotFound, id)
	}
	return objectType, objectData, nil
}

func (s *looseObjectStore) openPackIndex(metadata PackMetadata) (packIndex, error) {
	indexPath, err := s.packIndexPath(metadata.ID)
	if err != nil {
		return nil, err
	}
	index, err := s.packIndexes.Open(indexPath)
	if err != nil {
		return nil, err
	}
	indexMetadata := index.Metadata()
	if indexMetadata.Version != PackIndexFormatVersion || indexMetadata.Pack != metadata.ID ||
		indexMetadata.ObjectCount != metadata.ObjectCount {
		_ = index.Close()
		return nil, &PackCorruptionError{Pack: metadata.ID, Detail: "index metadata does not match manifest"}
	}
	return index, nil
}

func readPackedObjectFile(packID PackID, path string, entry PackIndexEntry, objectCount uint32) (objectType string, objectData []byte, err error) {
	if err := validatePackIndexEntry(packID, entry); err != nil {
		return "", nil, err
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return "", nil, fmt.Errorf("inspect pack %q: %w", packID, err)
	}
	if !pathInfo.Mode().IsRegular() {
		return "", nil, &PackCorruptionError{Pack: packID, Detail: "pack is not a regular file"}
	}
	file, err := os.Open(path)
	if err != nil {
		return "", nil, fmt.Errorf("open pack %q: %w", packID, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close pack %q: %w", packID, closeErr)
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return "", nil, fmt.Errorf("inspect pack %q: %w", packID, err)
	}
	if !info.Mode().IsRegular() {
		return "", nil, &PackCorruptionError{Pack: packID, Detail: "pack is not a regular file"}
	}
	if entry.Offset > uint64(info.Size()) || entry.CompressedSize > uint64(info.Size())-entry.Offset {
		return "", nil, &PackCorruptionError{Pack: packID, Object: entry.Object, Offset: entry.Offset, Detail: "entry exceeds pack length"}
	}
	header, err := readPackHeader(file)
	if err != nil {
		return "", nil, &PackCorruptionError{Pack: packID, Object: entry.Object, Offset: entry.Offset, Detail: err.Error()}
	}
	if err := validatePackHeader(header); err != nil {
		return "", nil, err
	}
	if header.ObjectCount != objectCount {
		return "", nil, &PackCorruptionError{Pack: packID, Object: entry.Object, Offset: entry.Offset, Detail: "pack and index object counts differ"}
	}
	compressed := make([]byte, int(entry.CompressedSize))
	if _, err := file.ReadAt(compressed, int64(entry.Offset)); err != nil {
		return "", nil, &PackCorruptionError{Pack: packID, Object: entry.Object, Offset: entry.Offset, Detail: "read compressed entry: " + err.Error()}
	}
	if crc32.ChecksumIEEE(compressed) != entry.CRC32 {
		return "", nil, &PackCorruptionError{Pack: packID, Object: entry.Object, Offset: entry.Offset, Detail: "compressed entry CRC does not match"}
	}
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return "", nil, fmt.Errorf("create zstd decoder: %w", err)
	}
	defer decoder.Close()
	envelope, err := decoder.DecodeAll(compressed, nil)
	if err != nil {
		return "", nil, &PackCorruptionError{Pack: packID, Object: entry.Object, Offset: entry.Offset, Detail: "decompress entry: " + err.Error()}
	}
	if uint64(len(envelope)) != entry.UncompressedSize {
		return "", nil, &PackCorruptionError{Pack: packID, Object: entry.Object, Offset: entry.Offset, Detail: "uncompressed entry size does not match"}
	}
	objectType, objectData, err = decodeObjectEnvelope(envelope, entry.Object)
	if err != nil {
		return "", nil, &PackCorruptionError{Pack: packID, Object: entry.Object, Offset: entry.Offset, Detail: err.Error()}
	}
	return objectType, objectData, nil
}

func readPackHeader(reader io.Reader) (packHeader, error) {
	var raw [packHeaderSize]byte
	if _, err := io.ReadFull(reader, raw[:]); err != nil {
		return packHeader{}, fmt.Errorf("read pack header: %w", err)
	}
	var header packHeader
	copy(header.Magic[:], raw[:4])
	header.Version = binary.BigEndian.Uint32(raw[4:8])
	header.ObjectCount = binary.BigEndian.Uint32(raw[8:12])
	return header, nil
}

func validatePackHeader(header packHeader) error {
	if string(header.Magic[:]) != PackMagic {
		return &PackCorruptionError{Detail: "pack has an invalid magic"}
	}
	if header.Version != PackFormatVersion {
		return &UnsupportedPackVersionError{Format: "pack", Version: header.Version}
	}
	return nil
}

func verifyPackFiles(indexStore packIndexStore, packID PackID, packPath, indexPath string) (result error) {
	index, err := indexStore.Open(indexPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := index.Close(); err != nil && result == nil {
			result = fmt.Errorf("close pack index %q: %w", packID, err)
		}
	}()
	metadata := index.Metadata()
	if metadata.Pack != packID || metadata.Version != PackIndexFormatVersion {
		return &PackCorruptionError{Pack: packID, Detail: "index metadata does not match pack"}
	}
	entries := make([]PackIndexEntry, 0, metadata.ObjectCount)
	if err := index.ForEach(func(entry PackIndexEntry) error {
		entries = append(entries, entry)
		return nil
	}); err != nil {
		return err
	}
	if uint64(len(entries)) != uint64(metadata.ObjectCount) {
		return &PackCorruptionError{Pack: packID, Detail: "index enumeration count does not match metadata"}
	}
	file, err := os.Open(packPath)
	if err != nil {
		return fmt.Errorf("open pack %q: %w", packID, err)
	}
	header, headerErr := readPackHeader(file)
	info, statErr := file.Stat()
	closeErr := file.Close()
	if headerErr != nil {
		return &PackCorruptionError{Pack: packID, Detail: headerErr.Error()}
	}
	if statErr != nil {
		return fmt.Errorf("inspect pack %q: %w", packID, statErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close pack %q: %w", packID, closeErr)
	}
	if !info.Mode().IsRegular() {
		return &PackCorruptionError{Pack: packID, Detail: "pack is not a regular file"}
	}
	if err := validatePackHeader(header); err != nil {
		return err
	}
	if header.ObjectCount != metadata.ObjectCount {
		return &PackCorruptionError{Pack: packID, Detail: "pack and index object counts differ"}
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Offset < entries[right].Offset })
	nextOffset := uint64(packHeaderSize)
	for _, entry := range entries {
		if entry.Offset != nextOffset {
			return &PackCorruptionError{Pack: packID, Object: entry.Object, Offset: entry.Offset, Detail: "pack entries are not contiguous"}
		}
		if _, _, err := readPackedObjectFile(packID, packPath, entry, metadata.ObjectCount); err != nil {
			return err
		}
		nextOffset += entry.CompressedSize
	}
	if nextOffset != uint64(info.Size()) {
		return &PackCorruptionError{Pack: packID, Offset: nextOffset, Detail: "pack has unindexed trailing data"}
	}
	return nil
}
