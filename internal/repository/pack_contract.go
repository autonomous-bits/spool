package repository

import (
	"errors"
	"fmt"
	"time"

	"github.com/autonomous-bits/spool/graphcontract"
)

const (
	// PackMagic identifies an IDG pack stream.
	PackMagic = graphcontract.PackMagic
	// PackFormatVersion is the version encoded in every pack header.
	PackFormatVersion = graphcontract.PackFormatVersion
	// PackIndexFormatVersion is the version of the sidecar object index.
	PackIndexFormatVersion = graphcontract.PackIndexFormatVersion
	// PackManifestFormatVersion is the version of objects/info/packs.
	PackManifestFormatVersion = graphcontract.PackManifestFormatVersion

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
	ErrPackCorrupt = graphcontract.ErrPackCorrupt
	// ErrUnsupportedPackVersion reports a pack storage format newer or older than this repository supports.
	ErrUnsupportedPackVersion = graphcontract.ErrUnsupportedPackVersion
	// ErrGCCorrupt reports corruption that prevents GC from safely deciding what to retain.
	ErrGCCorrupt = errors.New("GC cannot continue with corrupt repository data")
)

type (
	// PackID identifies one immutable pack and its paired index.
	PackID = graphcontract.PackID
	// PackCompression identifies the compression applied to a packed object envelope.
	PackCompression = graphcontract.PackCompression
	// PackMetadata identifies an active pack listed by a manifest.
	PackMetadata = graphcontract.PackMetadata
	// PackManifest is the atomically replaced list of active packs.
	PackManifest = graphcontract.PackManifest
	// PackIndexEntry maps one object ID to its zstd-compressed canonical loose
	// envelope in a pack. CRC32 is the IEEE CRC32 of the compressed bytes.
	PackIndexEntry = graphcontract.PackIndexEntry
	// PackCorruptionError identifies a failed pack, index, or manifest validation.
	PackCorruptionError = graphcontract.PackCorruptionError
	// UnsupportedPackVersionError identifies a pack, index, or manifest format
	// this repository cannot safely read.
	UnsupportedPackVersionError = graphcontract.UnsupportedPackVersionError
	// packHeader is the fixed 12-byte, big-endian prefix of every pack stream.
	packHeader = graphcontract.PackHeader
	// packIndexMetadata is persisted by a pack-index implementation and binds
	// the index to one pack before any entry lookup is trusted.
	packIndexMetadata = graphcontract.PackIndexMetadata
)

const (
	// PackCompressionZstd is the required compression for PackFormatVersion.
	PackCompressionZstd = graphcontract.PackCompressionZstd
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
