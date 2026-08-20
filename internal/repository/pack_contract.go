package repository

import (
	"errors"
	"fmt"
	"time"
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

	packDirectoryName      = "pack"
	packInfoDirectoryName  = "info"
	packManifestFilename   = "packs"
	packFileExtension      = ".pack"
	packIndexFileExtension = ".idx"
	packLockFilename       = "pack.lock"
	defaultGCGracePeriod   = 14 * 24 * time.Hour
)

var (
	// ErrPackCorrupt reports malformed or inconsistent pack, index, or manifest data.
	ErrPackCorrupt = errors.New("pack storage is corrupt")
	// ErrUnsupportedPackVersion reports a pack storage format newer or older than this repository supports.
	ErrUnsupportedPackVersion = errors.New("unsupported pack storage version")
	// ErrGCCorrupt reports corruption that prevents GC from safely deciding what to retain.
	ErrGCCorrupt = errors.New("GC cannot continue with corrupt repository data")
)

// PackID identifies one immutable pack and its paired index.
type PackID string

// PackCompression identifies the compression applied to a packed object envelope.
type PackCompression string

const (
	// PackCompressionZstd is the required compression for PackFormatVersion.
	PackCompressionZstd PackCompression = "zstd"
)

// GCOptions configures explicit object-store maintenance.
type GCOptions struct {
	// DryRun reports planned work without publishing a pack or deleting loose objects.
	DryRun bool
	// Repack compacts active packs and reachable loose objects into one replacement generation.
	Repack bool
	// GracePeriod overrides the retention period for unreachable loose objects.
	// A zero value uses DefaultGCGracePeriod.
	GracePeriod time.Duration
}

// DefaultGCGracePeriod is the retention period applied when GCOptions.GracePeriod is zero.
const DefaultGCGracePeriod = defaultGCGracePeriod

// GCResult is the complete, machine-readable report from one GC attempt.
type GCResult struct {
	Roots                      uint64 `json:"roots"`
	ReachableObjects           uint64 `json:"reachableObjects"`
	PackedObjects              uint64 `json:"packedObjects"`
	RetainedUnreachableObjects uint64 `json:"retainedUnreachableObjects"`
	PrunedLooseObjects         uint64 `json:"prunedLooseObjects"`
	RetiredPacks               uint64 `json:"retiredPacks"`
	ReclaimedBytes             uint64 `json:"reclaimedBytes"`
}

// GCCommittedWithWarningError reports cleanup that failed after a replacement
// pack generation was durably published. Result remains authoritative.
type GCCommittedWithWarningError struct {
	Result GCResult
	Err    error
}

// Error implements error.
func (e *GCCommittedWithWarningError) Error() string {
	if e.Err == nil {
		return "GC committed with cleanup warning"
	}
	return fmt.Sprintf("GC committed with cleanup warning: %v", e.Err)
}

// Unwrap returns the cleanup warning.
func (e *GCCommittedWithWarningError) Unwrap() error { return e.Err }

// PackMetadata identifies an active pack listed by a manifest.
type PackMetadata struct {
	ID          PackID          `json:"id"`
	Version     uint32          `json:"version"`
	Compression PackCompression `json:"compression"`
	ObjectCount uint32          `json:"objectCount"`
}

// packHeader is the fixed 12-byte, big-endian prefix of every pack stream.
// Magic must equal PackMagic and Version must equal PackFormatVersion before
// any object entry is processed.
type packHeader struct {
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

// packIndexMetadata is persisted by a pack-index implementation and binds the
// index to one pack before any entry lookup is trusted.
type packIndexMetadata struct {
	Version     uint32
	Pack        PackID
	ObjectCount uint32
}

// packIndex deliberately hides its on-disk representation. Implementations
// must return entries by exact object ID and expose all entries for verification
// and compaction.
type packIndex interface {
	Metadata() packIndexMetadata
	Lookup(ObjectID) (PackIndexEntry, bool, error)
	ForEach(func(PackIndexEntry) error) error
	Close() error
}

// packIndexStore opens verified immutable indexes and creates their durable
// replacements. A bbolt implementation can therefore be replaced without
// changing pack readers or maintenance logic.
type packIndexStore interface {
	Open(path string) (packIndex, error)
	Write(path string, metadata packIndexMetadata, entries []PackIndexEntry) error
}
