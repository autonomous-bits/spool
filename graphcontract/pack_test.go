package graphcontract_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"

	"github.com/autonomous-bits/spool/graphcontract"
	"github.com/klauspost/compress/zstd"
)

const packFixtureFormatVersion = 1

type packFixture struct {
	FormatVersion int                          `json:"format_version"`
	PackID        graphcontract.PackID         `json:"pack_id"`
	PackBase64    string                       `json:"pack_base64"`
	ObjectType    string                       `json:"object_type"`
	ObjectDataB64 string                       `json:"object_data_base64"`
	Entry         graphcontract.PackIndexEntry `json:"entry"`
}

func loadPackFixture(t *testing.T) (packFixture, []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "pack", "v1", "valid-pack.json"))
	if err != nil {
		t.Fatalf("read pack fixture: %v", err)
	}
	var fixture packFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode pack fixture: %v", err)
	}
	if fixture.FormatVersion != packFixtureFormatVersion {
		t.Fatalf("format version = %d, want %d", fixture.FormatVersion, packFixtureFormatVersion)
	}
	packBytes, err := base64.StdEncoding.DecodeString(fixture.PackBase64)
	if err != nil {
		t.Fatalf("decode pack bytes: %v", err)
	}
	return fixture, packBytes
}

// TestValidPackFixtureRoundTrip proves a well-formed pack stream reads,
// validates, and decompresses to the original canonical object bytes.
func TestValidPackFixtureRoundTrip(t *testing.T) {
	fixture, packBytes := loadPackFixture(t)

	header, err := graphcontract.ReadPackHeader(bytes.NewReader(packBytes))
	if err != nil {
		t.Fatalf("read pack header: %v", err)
	}
	if err := graphcontract.ValidatePackHeader(header); err != nil {
		t.Fatalf("validate pack header: %v", err)
	}
	if header.ObjectCount != 1 {
		t.Fatalf("header object count = %d, want 1", header.ObjectCount)
	}

	headerAt, err := graphcontract.ReadPackHeaderAt(bytes.NewReader(packBytes))
	if err != nil {
		t.Fatalf("read pack header at: %v", err)
	}
	if headerAt != header {
		t.Fatalf("ReadPackHeaderAt = %+v, want %+v", headerAt, header)
	}

	if err := graphcontract.ValidatePackIndexEntry(fixture.PackID, fixture.Entry); err != nil {
		t.Fatalf("validate pack index entry: %v", err)
	}
	if err := graphcontract.ValidatePackEntries(fixture.PackID, header, uint64(len(packBytes)), []graphcontract.PackIndexEntry{fixture.Entry}); err != nil {
		t.Fatalf("validate pack entries: %v", err)
	}

	compressed := packBytes[fixture.Entry.Offset : fixture.Entry.Offset+fixture.Entry.CompressedSize]
	objectType, objectData, err := graphcontract.DecompressPackedObject(fixture.Entry, compressed)
	if err != nil {
		t.Fatalf("decompress packed object: %v", err)
	}
	if objectType != fixture.ObjectType {
		t.Fatalf("object type = %q, want %q", objectType, fixture.ObjectType)
	}
	wantData, err := base64.StdEncoding.DecodeString(fixture.ObjectDataB64)
	if err != nil {
		t.Fatalf("decode want object data: %v", err)
	}
	if !bytes.Equal(objectData, wantData) {
		t.Fatalf("object data = %x, want %x", objectData, wantData)
	}
}

func TestValidatePackHeaderRejectsBadMagicAndVersion(t *testing.T) {
	_, packBytes := loadPackFixture(t)
	header, err := graphcontract.ReadPackHeader(bytes.NewReader(packBytes))
	if err != nil {
		t.Fatalf("read pack header: %v", err)
	}

	badMagic := header
	badMagic.Magic = [4]byte{'X', 'X', 'X', 'X'}
	if err := graphcontract.ValidatePackHeader(badMagic); err == nil || !errors.Is(err, graphcontract.ErrPackCorrupt) {
		t.Fatalf("validate bad magic error = %v, want ErrPackCorrupt", err)
	}

	badVersion := header
	badVersion.Version = header.Version + 1
	err = graphcontract.ValidatePackHeader(badVersion)
	var unsupported *graphcontract.UnsupportedPackVersionError
	if !errors.As(err, &unsupported) || !errors.Is(err, graphcontract.ErrUnsupportedPackVersion) {
		t.Fatalf("validate bad version error = %v, want UnsupportedPackVersionError", err)
	}
}

func TestReadPackHeaderRejectsTruncatedInput(t *testing.T) {
	_, packBytes := loadPackFixture(t)
	truncated := packBytes[:graphcontract.PackHeaderSize-1]
	if _, err := graphcontract.ReadPackHeader(bytes.NewReader(truncated)); err == nil {
		t.Fatal("read truncated pack header: want error")
	}
	if _, err := graphcontract.ReadPackHeaderAt(bytes.NewReader(truncated)); err == nil {
		t.Fatal("read truncated pack header at: want error")
	}
}

func TestValidatePackManifest(t *testing.T) {
	validID := graphcontract.PackID("0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f")
	tests := []struct {
		name     string
		manifest graphcontract.PackManifest
		wantErr  error
	}{
		{
			name:     "valid",
			manifest: graphcontract.PackManifest{Version: graphcontract.PackManifestFormatVersion, Packs: []graphcontract.PackMetadata{}},
		},
		{
			name:     "unsupported version",
			manifest: graphcontract.PackManifest{Version: graphcontract.PackManifestFormatVersion + 1, Packs: []graphcontract.PackMetadata{}},
			wantErr:  graphcontract.ErrUnsupportedPackVersion,
		},
		{
			name:     "nil packs",
			manifest: graphcontract.PackManifest{Version: graphcontract.PackManifestFormatVersion},
			wantErr:  graphcontract.ErrPackCorrupt,
		},
		{
			name: "invalid pack ID",
			manifest: graphcontract.PackManifest{Version: graphcontract.PackManifestFormatVersion, Packs: []graphcontract.PackMetadata{
				{ID: "not-hex", Version: graphcontract.PackFormatVersion, Compression: graphcontract.PackCompressionZstd},
			}},
			wantErr: graphcontract.ErrPackCorrupt,
		},
		{
			name: "unsupported pack version",
			manifest: graphcontract.PackManifest{Version: graphcontract.PackManifestFormatVersion, Packs: []graphcontract.PackMetadata{
				{ID: validID, Version: graphcontract.PackFormatVersion + 1, Compression: graphcontract.PackCompressionZstd},
			}},
			wantErr: graphcontract.ErrUnsupportedPackVersion,
		},
		{
			name: "unsupported compression",
			manifest: graphcontract.PackManifest{Version: graphcontract.PackManifestFormatVersion, Packs: []graphcontract.PackMetadata{
				{ID: validID, Version: graphcontract.PackFormatVersion, Compression: "gzip"},
			}},
			wantErr: graphcontract.ErrPackCorrupt,
		},
		{
			name: "duplicate pack",
			manifest: graphcontract.PackManifest{Version: graphcontract.PackManifestFormatVersion, Packs: []graphcontract.PackMetadata{
				{ID: validID, Version: graphcontract.PackFormatVersion, Compression: graphcontract.PackCompressionZstd},
				{ID: validID, Version: graphcontract.PackFormatVersion, Compression: graphcontract.PackCompressionZstd},
			}},
			wantErr: graphcontract.ErrPackCorrupt,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := graphcontract.ValidatePackManifest(tt.manifest)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("validate manifest: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("validate manifest error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePackIndexMetadata(t *testing.T) {
	validID := graphcontract.PackID("0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f")
	tests := []struct {
		name     string
		metadata graphcontract.PackIndexMetadata
		wantErr  error
	}{
		{name: "valid", metadata: graphcontract.PackIndexMetadata{Version: graphcontract.PackIndexFormatVersion, Pack: validID}},
		{
			name:     "unsupported version",
			metadata: graphcontract.PackIndexMetadata{Version: graphcontract.PackIndexFormatVersion + 1, Pack: validID},
			wantErr:  graphcontract.ErrUnsupportedPackVersion,
		},
		{
			name:     "invalid pack ID",
			metadata: graphcontract.PackIndexMetadata{Version: graphcontract.PackIndexFormatVersion, Pack: "short"},
			wantErr:  graphcontract.ErrPackCorrupt,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := graphcontract.ValidatePackIndexMetadata(tt.metadata)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("validate index metadata: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("validate index metadata error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePackIndexEntryRejectsOutOfBoundsEntries(t *testing.T) {
	fixture, _ := loadPackFixture(t)
	base := fixture.Entry

	tests := []struct {
		name  string
		entry graphcontract.PackIndexEntry
	}{
		{name: "invalid object ID", entry: graphcontract.PackIndexEntry{Object: "not-hex", Offset: base.Offset, CompressedSize: base.CompressedSize, UncompressedSize: base.UncompressedSize, CRC32: base.CRC32}},
		{name: "offset before header", entry: graphcontract.PackIndexEntry{Object: base.Object, Offset: graphcontract.PackHeaderSize - 1, CompressedSize: base.CompressedSize, UncompressedSize: base.UncompressedSize, CRC32: base.CRC32}},
		{name: "zero compressed size", entry: graphcontract.PackIndexEntry{Object: base.Object, Offset: base.Offset, CompressedSize: 0, UncompressedSize: base.UncompressedSize, CRC32: base.CRC32}},
		{name: "zero uncompressed size", entry: graphcontract.PackIndexEntry{Object: base.Object, Offset: base.Offset, CompressedSize: base.CompressedSize, UncompressedSize: 0, CRC32: base.CRC32}},
		{name: "offset overflow", entry: graphcontract.PackIndexEntry{Object: base.Object, Offset: ^uint64(0), CompressedSize: base.CompressedSize, UncompressedSize: base.UncompressedSize, CRC32: base.CRC32}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := graphcontract.ValidatePackIndexEntry(fixture.PackID, tt.entry)
			if err == nil || !errors.Is(err, graphcontract.ErrPackCorrupt) {
				t.Fatalf("validate index entry error = %v, want ErrPackCorrupt", err)
			}
		})
	}

	// The base fixture entry itself must remain valid.
	if err := graphcontract.ValidatePackIndexEntry(fixture.PackID, base); err != nil {
		t.Fatalf("validate valid index entry: %v", err)
	}
}

func TestValidatePackEntriesRejectsNonContiguousAndTrailingData(t *testing.T) {
	fixture, packBytes := loadPackFixture(t)
	header, err := graphcontract.ReadPackHeader(bytes.NewReader(packBytes))
	if err != nil {
		t.Fatalf("read pack header: %v", err)
	}

	gapEntry := fixture.Entry
	gapEntry.Offset++
	if err := graphcontract.ValidatePackEntries(fixture.PackID, header, uint64(len(packBytes))+1, []graphcontract.PackIndexEntry{gapEntry}); err == nil || !errors.Is(err, graphcontract.ErrPackCorrupt) {
		t.Fatalf("validate non-contiguous entries error = %v, want ErrPackCorrupt", err)
	}

	if err := graphcontract.ValidatePackEntries(fixture.PackID, header, uint64(len(packBytes))+1, []graphcontract.PackIndexEntry{fixture.Entry}); err == nil || !errors.Is(err, graphcontract.ErrPackCorrupt) {
		t.Fatalf("validate trailing data error = %v, want ErrPackCorrupt", err)
	}

	mismatchedHeader := header
	mismatchedHeader.ObjectCount = 2
	if err := graphcontract.ValidatePackEntries(fixture.PackID, mismatchedHeader, uint64(len(packBytes)), []graphcontract.PackIndexEntry{fixture.Entry}); err == nil || !errors.Is(err, graphcontract.ErrPackCorrupt) {
		t.Fatalf("validate object count mismatch error = %v, want ErrPackCorrupt", err)
	}

	if err := graphcontract.ValidatePackEntries(fixture.PackID, header, uint64(len(packBytes)), []graphcontract.PackIndexEntry{fixture.Entry}); err != nil {
		t.Fatalf("validate matching entries: %v", err)
	}
}

func TestDecompressPackedObjectRejectsCorruption(t *testing.T) {
	fixture, packBytes := loadPackFixture(t)
	compressed := packBytes[fixture.Entry.Offset : fixture.Entry.Offset+fixture.Entry.CompressedSize]

	t.Run("valid", func(t *testing.T) {
		if _, _, err := graphcontract.DecompressPackedObject(fixture.Entry, compressed); err != nil {
			t.Fatalf("decompress valid entry: %v", err)
		}
	})

	t.Run("bad crc32", func(t *testing.T) {
		badEntry := fixture.Entry
		badEntry.CRC32++
		if _, _, err := graphcontract.DecompressPackedObject(badEntry, compressed); err == nil {
			t.Fatal("decompress bad CRC: want error")
		}
	})

	t.Run("truncated zstd payload", func(t *testing.T) {
		truncated := append([]byte{}, compressed[:len(compressed)/2]...)
		badEntry := fixture.Entry
		badEntry.CompressedSize = uint64(len(truncated))
		badEntry.CRC32 = crc32.ChecksumIEEE(truncated)
		if _, _, err := graphcontract.DecompressPackedObject(badEntry, truncated); err == nil {
			t.Fatal("decompress truncated zstd payload: want error")
		}
	})

	t.Run("uncompressed size mismatch", func(t *testing.T) {
		badEntry := fixture.Entry
		badEntry.UncompressedSize++
		if _, _, err := graphcontract.DecompressPackedObject(badEntry, compressed); err == nil {
			t.Fatal("decompress size mismatch: want error")
		}
	})

	t.Run("object hash mismatch", func(t *testing.T) {
		badEntry := fixture.Entry
		badEntry.Object = graphcontract.ObjectID("0000000000000000000000000000000000000000000000000000000000000000"[:64])
		if _, _, err := graphcontract.DecompressPackedObject(badEntry, compressed); err == nil {
			t.Fatal("decompress object hash mismatch: want error")
		}
	})

	t.Run("malformed canonical envelope", func(t *testing.T) {
		decoder, err := zstd.NewReader(nil)
		if err != nil {
			t.Fatalf("create zstd decoder: %v", err)
		}
		defer decoder.Close()
		envelope, err := decoder.DecodeAll(compressed, nil)
		if err != nil {
			t.Fatalf("decompress fixture envelope: %v", err)
		}
		malformed := append(append([]byte{}, envelope...), 0x00)

		encoder, err := zstd.NewWriter(nil)
		if err != nil {
			t.Fatalf("create zstd encoder: %v", err)
		}
		malformedCompressed := encoder.EncodeAll(malformed, nil)
		if err := encoder.Close(); err != nil {
			t.Fatalf("close zstd encoder: %v", err)
		}

		badEntry := fixture.Entry
		badEntry.CompressedSize = uint64(len(malformedCompressed))
		badEntry.UncompressedSize = uint64(len(malformed))
		badEntry.CRC32 = crc32.ChecksumIEEE(malformedCompressed)
		if _, _, err := graphcontract.DecompressPackedObject(badEntry, malformedCompressed); err == nil {
			t.Fatal("decompress malformed canonical envelope: want error")
		}
	})
}

func TestDecodePackedObjectEnvelopeRejectsMalformedEnvelope(t *testing.T) {
	fixture, packBytes := loadPackFixture(t)

	if _, _, err := graphcontract.DecodePackedObjectEnvelope([]byte("not cbor"), fixture.Entry.Object); err == nil {
		t.Fatal("decode non-CBOR envelope: want error")
	}

	compressed := packBytes[fixture.Entry.Offset : fixture.Entry.Offset+fixture.Entry.CompressedSize]
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		t.Fatalf("create zstd decoder: %v", err)
	}
	defer decoder.Close()
	envelope, err := decoder.DecodeAll(compressed, nil)
	if err != nil {
		t.Fatalf("decompress fixture envelope: %v", err)
	}
	trailingGarbage := append(append([]byte{}, envelope...), 0x00)
	if _, _, err := graphcontract.DecodePackedObjectEnvelope(trailingGarbage, fixture.Entry.Object); err == nil {
		t.Fatal("decode envelope with trailing garbage: want error")
	}
}

func TestValidPackIDRejectsMalformedIdentifiers(t *testing.T) {
	tests := []struct {
		name string
		id   graphcontract.PackID
		want bool
	}{
		{name: "valid", id: "0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f", want: true},
		{name: "too short", id: "0f0f", want: false},
		{name: "not hex", id: "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", want: false},
		{name: "uppercase", id: "0F0F0F0F0F0F0F0F0F0F0F0F0F0F0F0F", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := graphcontract.ValidPackID(tt.id); got != tt.want {
				t.Fatalf("ValidPackID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}
