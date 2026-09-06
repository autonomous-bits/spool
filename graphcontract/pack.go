package graphcontract

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/fxamacker/cbor/v2"
	"github.com/klauspost/compress/zstd"
	"lukechampine.com/blake3"
)

const (
	// PackMagic identifies an IDG pack stream.
	PackMagic = "IDGP"
	// PackFormatVersion is the version encoded in every pack header.
	PackFormatVersion uint32 = 2
	// PackIndexFormatVersion is the version of the sidecar object index.
	PackIndexFormatVersion uint32 = 1
	// PackManifestFormatVersion is the version of objects/info/packs.
	PackManifestFormatVersion uint32 = 1
	// PackHeaderSize is the fixed byte length of a pack stream header.
	PackHeaderSize = 12
	// MaxPackIDLength is the longest pack ID a verifier accepts.
	MaxPackIDLength = 64

	packObjectIDHexLength = 64
)

var (
	// ErrPackCorrupt reports malformed or inconsistent pack, index, or manifest data.
	ErrPackCorrupt = errors.New("pack storage is corrupt")
	// ErrUnsupportedPackVersion reports a pack storage format newer or older than this repository supports.
	ErrUnsupportedPackVersion = errors.New("unsupported pack storage version")
)

// PackID identifies one immutable pack and its paired index.
type PackID string

// PackCompression identifies the compression applied to a packed object envelope.
type PackCompression string

const (
	// PackCompressionZstd is the required compression for PackFormatVersion.
	PackCompressionZstd PackCompression = "zstd"
)

// PackMetadata identifies an active pack listed by a manifest.
type PackMetadata struct {
	ID          PackID          `json:"id"`
	Version     uint32          `json:"version"`
	Compression PackCompression `json:"compression"`
	ObjectCount uint32          `json:"objectCount"`
}

// PackHeader is the fixed 12-byte, big-endian prefix of every pack stream.
// Magic must equal PackMagic and Version must equal PackFormatVersion before
// any object entry is processed.
type PackHeader struct {
	Magic       [4]byte
	Version     uint32
	ObjectCount uint32
}

// PackManifest is the atomically replaced list of active packs.
type PackManifest struct {
	Version uint32         `json:"version"`
	Packs   []PackMetadata `json:"packs"`
}

// PackIndexEntry maps one object ID to its zstd-compressed canonical loose
// envelope in a pack. CRC32 is the IEEE CRC32 of the compressed bytes.
type PackIndexEntry struct {
	Object           ObjectID `json:"object"`
	Offset           uint64   `json:"offset"`
	CompressedSize   uint64   `json:"compressedSize"`
	UncompressedSize uint64   `json:"uncompressedSize"`
	CRC32            uint32   `json:"crc32"`
}

// PackIndexMetadata is persisted by a pack-index implementation and binds the
// index to one pack before any entry lookup is trusted.
type PackIndexMetadata struct {
	Version     uint32
	Pack        PackID
	ObjectCount uint32
}

// PackCorruptionError identifies a failed pack, index, or manifest validation.
// Object and Offset are omitted when corruption is not associated with an entry.
type PackCorruptionError struct {
	Pack   PackID
	Object ObjectID
	Offset uint64
	Detail string
}

// Error implements error.
func (e *PackCorruptionError) Error() string {
	location := "pack storage"
	if e.Pack != "" {
		location = fmt.Sprintf("pack %q", e.Pack)
	}
	if e.Object != "" {
		location += fmt.Sprintf(" object %q", e.Object)
	}
	if e.Object != "" || e.Offset != 0 {
		location += fmt.Sprintf(" at offset %d", e.Offset)
	}
	if e.Detail == "" {
		return fmt.Sprintf("%s: %v", location, ErrPackCorrupt)
	}
	return fmt.Sprintf("%s: %v: %s", location, ErrPackCorrupt, e.Detail)
}

// Unwrap makes PackCorruptionError match ErrPackCorrupt.
func (e *PackCorruptionError) Unwrap() error { return ErrPackCorrupt }

// UnsupportedPackVersionError identifies a pack, index, or manifest format this
// repository cannot safely read.
type UnsupportedPackVersionError struct {
	Format  string
	Version uint32
}

// Error implements error.
func (e *UnsupportedPackVersionError) Error() string {
	return fmt.Sprintf("unsupported %s version %d", e.Format, e.Version)
}

// Unwrap makes UnsupportedPackVersionError match ErrUnsupportedPackVersion.
func (e *UnsupportedPackVersionError) Unwrap() error { return ErrUnsupportedPackVersion }

// packObjectEnvelope binds canonical object bytes to their repository type
// inside one packed, zstd-compressed entry. Its canonical CBOR representation
// matches the on-disk loose-object envelope format, so packed and loose
// objects hash identically.
type packObjectEnvelope struct {
	Type string `cbor:"1,keyasint"`
	Data []byte `cbor:"2,keyasint"`
}

// ValidPackID reports whether id is a well-formed 32-character lowercase hex
// pack identifier.
func ValidPackID(id PackID) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(string(id))
	return err == nil && strings.ToLower(string(id)) == string(id)
}

// ReadPackHeaderAt decodes the fixed-size pack header at the start of reader
// without validating its contents. Use ValidatePackHeader to check the
// result.
func ReadPackHeaderAt(reader io.ReaderAt) (PackHeader, error) {
	var raw [PackHeaderSize]byte
	if _, err := reader.ReadAt(raw[:], 0); err != nil {
		return PackHeader{}, fmt.Errorf("read pack header: %w", err)
	}
	return decodePackHeader(raw), nil
}

// ReadPackHeader decodes the fixed-size pack header from the start of reader
// without validating its contents. Use ValidatePackHeader to check the
// result.
func ReadPackHeader(reader io.Reader) (PackHeader, error) {
	var raw [PackHeaderSize]byte
	if _, err := io.ReadFull(reader, raw[:]); err != nil {
		return PackHeader{}, fmt.Errorf("read pack header: %w", err)
	}
	return decodePackHeader(raw), nil
}

func decodePackHeader(raw [PackHeaderSize]byte) PackHeader {
	var header PackHeader
	copy(header.Magic[:], raw[:4])
	header.Version = binary.BigEndian.Uint32(raw[4:8])
	header.ObjectCount = binary.BigEndian.Uint32(raw[8:12])
	return header
}

// ValidatePackHeader checks a decoded pack header's magic and format version.
func ValidatePackHeader(header PackHeader) error {
	if string(header.Magic[:]) != PackMagic {
		return &PackCorruptionError{Detail: "pack has an invalid magic"}
	}
	if header.Version != PackFormatVersion {
		return &UnsupportedPackVersionError{Format: "pack", Version: header.Version}
	}
	return nil
}

// ValidatePackManifest checks a decoded pack manifest's format version and
// every listed pack's ID, version, and compression.
func ValidatePackManifest(manifest PackManifest) error {
	if manifest.Version != PackManifestFormatVersion {
		return &UnsupportedPackVersionError{Format: "pack manifest", Version: manifest.Version}
	}
	if manifest.Packs == nil {
		return &PackCorruptionError{Detail: "manifest packs must be an array"}
	}
	seen := make(map[PackID]struct{}, len(manifest.Packs))
	for _, metadata := range manifest.Packs {
		if !ValidPackID(metadata.ID) {
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

// ValidatePackIndexMetadata checks a decoded pack index's format version and
// pack ID before any of its entries are trusted.
func ValidatePackIndexMetadata(metadata PackIndexMetadata) error {
	if metadata.Version != PackIndexFormatVersion {
		return &UnsupportedPackVersionError{Format: "pack index", Version: metadata.Version}
	}
	if !ValidPackID(metadata.Pack) {
		return &PackCorruptionError{Pack: metadata.Pack, Detail: "invalid pack ID"}
	}
	return nil
}

// ValidatePackIndexEntry checks that entry's object ID is well-formed and its
// offset and size fields describe a region that cannot overflow a pack
// stream's addressable bounds.
func ValidatePackIndexEntry(packID PackID, entry PackIndexEntry) error {
	if !validPackObjectID(entry.Object) {
		return &PackCorruptionError{Pack: packID, Object: entry.Object, Offset: entry.Offset, Detail: "invalid object ID"}
	}
	if entry.Offset < PackHeaderSize || entry.CompressedSize == 0 || entry.UncompressedSize == 0 ||
		entry.Offset > math.MaxInt64 || entry.CompressedSize > math.MaxInt64 || entry.UncompressedSize > math.MaxInt64 ||
		entry.Offset > math.MaxUint64-entry.CompressedSize {
		return &PackCorruptionError{Pack: packID, Object: entry.Object, Offset: entry.Offset, Detail: "invalid object entry bounds"}
	}
	return nil
}

// ValidatePackEntries checks that a pack's header agrees with its enumerated
// index entries and that the entries form one contiguous, non-overlapping
// region spanning exactly [PackHeaderSize, packSize) when ordered by offset.
// Callers must pass entries already validated by ValidatePackIndexEntry and
// sorted by ascending Offset.
func ValidatePackEntries(packID PackID, header PackHeader, packSize uint64, entries []PackIndexEntry) error {
	if header.ObjectCount != uint32(len(entries)) {
		return &PackCorruptionError{Pack: packID, Detail: "pack and index object counts differ"}
	}
	nextOffset := uint64(PackHeaderSize)
	for _, entry := range entries {
		if entry.Offset != nextOffset {
			return &PackCorruptionError{Pack: packID, Object: entry.Object, Offset: entry.Offset, Detail: "pack entries are not contiguous"}
		}
		nextOffset += entry.CompressedSize
	}
	if nextOffset != packSize {
		return &PackCorruptionError{Pack: packID, Offset: nextOffset, Detail: "pack has unindexed trailing data"}
	}
	return nil
}

// ObjectIDForEncoded computes the content-derived ObjectID for encoded bytes
// of the given object type, using the BLAKE3-based scheme shared by loose and
// packed object storage.
func ObjectIDForEncoded(objectType string, encoded []byte) ObjectID {
	header := objectType + " " + strconv.Itoa(len(encoded)) + "\x00"
	sum := blake3.Sum256(append([]byte(header), encoded...))
	return ObjectID(hex.EncodeToString(sum[:]))
}

// DecodePackedObjectEnvelope decodes and fully verifies one packed object
// envelope: it must decode as canonical CBOR, carry a non-empty type, and
// hash to id under ObjectIDForEncoded. It returns the object's type and a
// defensive copy of its canonical bytes.
func DecodePackedObjectEnvelope(data []byte, id ObjectID) (objectType string, objectData []byte, err error) {
	var envelope packObjectEnvelope
	if err := cbor.Unmarshal(data, &envelope); err != nil {
		return "", nil, &PackCorruptionError{Object: id, Detail: fmt.Sprintf("decode object envelope: %v", err)}
	}
	canonical, err := canonicalCBOR.Marshal(envelope)
	if err != nil || !bytes.Equal(data, canonical) {
		return "", nil, &PackCorruptionError{Object: id, Detail: "non-canonical object envelope"}
	}
	if envelope.Type == "" {
		return "", nil, &PackCorruptionError{Object: id, Detail: "empty object type"}
	}
	if ObjectIDForEncoded(envelope.Type, envelope.Data) != id {
		return "", nil, &PackCorruptionError{Object: id, Detail: "object hash mismatch"}
	}
	return envelope.Type, append([]byte(nil), envelope.Data...), nil
}

// DecompressPackedObject validates a packed entry's CRC32, decompresses its
// zstd-compressed envelope, verifies the decompressed size, and decodes and
// verifies its canonical object envelope. It returns the object's type and
// canonical bytes. Decompression is streamed and bounded to
// entry.UncompressedSize+1 bytes so a corrupt or hostile entry cannot force
// unbounded memory allocation before the size check runs.
func DecompressPackedObject(entry PackIndexEntry, compressed []byte) (objectType string, objectData []byte, err error) {
	if crc32.ChecksumIEEE(compressed) != entry.CRC32 {
		return "", nil, &PackCorruptionError{Object: entry.Object, Offset: entry.Offset, Detail: "compressed entry CRC does not match"}
	}
	decoder, err := zstd.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return "", nil, fmt.Errorf("create zstd decoder: %w", err)
	}
	defer decoder.Close()
	limited := io.LimitReader(decoder, int64(entry.UncompressedSize)+1)
	envelope, err := io.ReadAll(limited)
	if err != nil {
		return "", nil, &PackCorruptionError{Object: entry.Object, Offset: entry.Offset, Detail: fmt.Sprintf("decompress entry: %v", err)}
	}
	if uint64(len(envelope)) != entry.UncompressedSize {
		return "", nil, &PackCorruptionError{Object: entry.Object, Offset: entry.Offset, Detail: "uncompressed entry size does not match"}
	}
	return DecodePackedObjectEnvelope(envelope, entry.Object)
}

func validPackObjectID(id ObjectID) bool {
	if len(id) != packObjectIDHexLength || strings.ToLower(string(id)) != string(id) {
		return false
	}
	decoded, err := hex.DecodeString(string(id))
	return err == nil && len(decoded) == packObjectIDHexLength/2
}
