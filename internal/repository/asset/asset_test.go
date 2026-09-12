package asset

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/autonomous-bits/spool/graphcontract"
	"lukechampine.com/blake3"
)

func TestWriteAndReadBlobDurable(t *testing.T) {
	tempDir := t.TempDir()
	store := NewStore(tempDir)

	content := []byte("# Architecture Decision Record\n\nThis document records context.")
	expectedHash := hex.EncodeToString(func() []byte {
		h := blake3.Sum256(content)
		return h[:]
	}())

	hash, size, err := store.WriteBlob(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("WriteBlob failed: %v", err)
	}
	if hash != expectedHash {
		t.Errorf("expected hash %s, got %s", expectedHash, hash)
	}
	if size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), size)
	}

	// Verify on-disk sharded path exists
	expectedPath := filepath.Join(tempDir, "assets", "loose", hash[:2], hash[2:])
	if _, err := os.Stat(expectedPath); err != nil {
		t.Errorf("expected file at %s: %v", expectedPath, err)
	}

	// Verify HasBlob
	has, err := store.HasBlob(hash)
	if err != nil || !has {
		t.Errorf("HasBlob(%s) = (%v, %v), want (true, nil)", hash, has, err)
	}
	hasUri, err := store.HasBlob("spool://assets/" + hash)
	if err != nil || !hasUri {
		t.Errorf("HasBlob(uri) = (%v, %v), want (true, nil)", hasUri, err)
	}

	// Read blob back
	reader, readSize, err := store.ReadBlob(hash)
	if err != nil {
		t.Fatalf("ReadBlob failed: %v", err)
	}
	defer func() { _ = reader.Close() }()

	if readSize != int64(len(content)) {
		t.Errorf("expected read size %d, got %d", len(content), readSize)
	}
	readData, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if !bytes.Equal(readData, content) {
		t.Errorf("content mismatch: got %q, want %q", readData, content)
	}

	// Test deduplication on subsequent write
	hash2, size2, err := store.WriteBlob(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("second WriteBlob failed: %v", err)
	}
	if hash2 != hash || size2 != size {
		t.Errorf("deduplicated write mismatch: got (%s, %d), want (%s, %d)", hash2, size2, hash, size)
	}
}

func TestWriteAndReadBlobMemory(t *testing.T) {
	store := NewStore("")

	content := []byte("in-memory blob content")
	expectedHash := hex.EncodeToString(func() []byte {
		h := blake3.Sum256(content)
		return h[:]
	}())

	hash, size, err := store.WriteBlob(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("WriteBlob failed: %v", err)
	}
	if hash != expectedHash {
		t.Errorf("expected hash %s, got %s", expectedHash, hash)
	}
	if size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), size)
	}

	has, err := store.HasBlob(hash)
	if err != nil || !has {
		t.Errorf("HasBlob(%s) = (%v, %v), want (true, nil)", hash, has, err)
	}

	reader, readSize, err := store.ReadBlob(hash)
	if err != nil {
		t.Fatalf("ReadBlob failed: %v", err)
	}
	defer func() { _ = reader.Close() }()

	if readSize != size {
		t.Errorf("expected read size %d, got %d", size, readSize)
	}
	readData, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if !bytes.Equal(readData, content) {
		t.Errorf("content mismatch: got %q, want %q", readData, content)
	}
}

func TestParseAndFormatLocator(t *testing.T) {
	validHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	uri := FormatLocator(validHash)
	expectedURI := "spool://assets/" + validHash
	if uri != expectedURI {
		t.Errorf("FormatLocator(%s) = %s, want %s", validHash, uri, expectedURI)
	}

	parsed, err := ParseLocator(uri)
	if err != nil {
		t.Fatalf("ParseLocator(%s) failed: %v", uri, err)
	}
	if parsed != validHash {
		t.Errorf("parsed hash %s, want %s", parsed, validHash)
	}

	// Bare hash
	parsedBare, err := ParseLocator(validHash)
	if err != nil || parsedBare != validHash {
		t.Errorf("ParseLocator(bare) = (%s, %v), want (%s, nil)", parsedBare, err, validHash)
	}

	// Uppercase hex should be normalized
	parsedUpper, err := ParseLocator("spool://assets/" + stringsToUpper(validHash))
	if err != nil || parsedUpper != validHash {
		t.Errorf("ParseLocator(upper) = (%s, %v), want (%s, nil)", parsedUpper, err, validHash)
	}

	// Invalid cases
	invalidCases := []string{
		"",
		"spool://assets/",
		"spool://assets/short",
		"spool://assets/not-hex-characters-here-0123456789abcdef0123456789abcdef0123456789",
		"http://example.com/file",
	}
	for _, tc := range invalidCases {
		if _, err := ParseLocator(tc); !errors.Is(err, ErrInvalidAssetHash) {
			t.Errorf("ParseLocator(%q) error = %v, want ErrInvalidAssetHash", tc, err)
		}
	}
}

func TestDetectMIMEType(t *testing.T) {
	tests := []struct {
		filename string
		peek     []byte
		expected string
	}{
		{"spec.md", []byte("# Header"), "text/markdown; charset=utf-8"},
		{"spec.markdown", []byte("# Header"), "text/markdown; charset=utf-8"},
		{"data.json", []byte("{\"key\": 1}"), "application/json"},
		{"diagram.svg", []byte("<svg></svg>"), "image/svg+xml"},
		{"config.yaml", []byte("key: val"), "application/yaml"},
		{"config.yml", []byte("key: val"), "application/yaml"},
		{"doc.pdf", []byte("%PDF-1.4"), "application/pdf"},
		{"image.png", []byte("\x89PNG\r\n\x1a\n"), "image/png"},
		{"unknown.bin", []byte{0x00, 0x01, 0x02, 0x03}, "application/octet-stream"},
	}

	for _, tc := range tests {
		got := DetectMIMEType(tc.filename, tc.peek)
		if got != tc.expected {
			t.Errorf("DetectMIMEType(%q) = %q, want %q", tc.filename, got, tc.expected)
		}
	}
}

func TestExtractAssetHash(t *testing.T) {
	hash1 := "1111111111111111111111111111111111111111111111111111111111111111"
	hash2 := "2222222222222222222222222222222222222222222222222222222222222222"

	node1 := graphcontract.Node{
		ID: "node-1",
		Properties: map[string]graphcontract.PropertyValue{
			"assetUri": graphcontract.StringPropertyValue("spool://assets/" + hash1),
		},
	}
	node2 := graphcontract.Node{
		ID: "node-2",
		Properties: map[string]graphcontract.PropertyValue{
			"assetUri": graphcontract.StringPropertyValue("spool://assets/" + hash2),
		},
	}
	nodeDup := graphcontract.Node{
		ID: "node-dup",
		Properties: map[string]graphcontract.PropertyValue{
			"assetUri": graphcontract.StringPropertyValue(hash1),
		},
	}
	nodeNoAsset := graphcontract.Node{
		ID: "node-3",
		Properties: map[string]graphcontract.PropertyValue{
			"title": graphcontract.StringPropertyValue("Not an asset"),
		},
	}

	h, ok := ExtractAssetHash(node1)
	if !ok || h != hash1 {
		t.Errorf("ExtractAssetHash(node1) = (%s, %v), want (%s, true)", h, ok, hash1)
	}

	_, ok = ExtractAssetHash(nodeNoAsset)
	if ok {
		t.Errorf("ExtractAssetHash(nodeNoAsset) = true, want false")
	}

	allNodes := map[string]graphcontract.Node{
		"n1":   node1,
		"n2":   node2,
		"ndup": nodeDup,
		"n3":   nodeNoAsset,
	}
	hashes := ExtractAssetHashes(allNodes)
	if len(hashes) != 2 {
		t.Fatalf("expected 2 unique hashes, got %d: %v", len(hashes), hashes)
	}
	if hashes[0] != hash1 || hashes[1] != hash2 {
		t.Errorf("unexpected hashes order/content: %v", hashes)
	}
}

func TestReadBlobNotFound(t *testing.T) {
	store := NewStore(t.TempDir())
	nonExistentHash := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	_, _, err := store.ReadBlob(nonExistentHash)
	if !errors.Is(err, ErrAssetNotFound) {
		t.Errorf("ReadBlob(nonExistent) err = %v, want ErrAssetNotFound", err)
	}
}

func TestLargeBlobStreaming(t *testing.T) {
	tempDir := t.TempDir()
	store := NewStore(tempDir)

	// 5MB random data
	data := make([]byte, 5*1024*1024)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand.Read failed: %v", err)
	}

	hasher := blake3.New(32, nil)
	hasher.Write(data)
	expectedHash := hex.EncodeToString(hasher.Sum(nil))

	hash, size, err := store.WriteBlob(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("WriteBlob 5MB failed: %v", err)
	}
	if hash != expectedHash {
		t.Errorf("hash mismatch: got %s, want %s", hash, expectedHash)
	}
	if size != int64(len(data)) {
		t.Errorf("size mismatch: got %d, want %d", size, len(data))
	}

	reader, readSize, err := store.ReadBlob(hash)
	if err != nil {
		t.Fatalf("ReadBlob failed: %v", err)
	}
	defer func() { _ = reader.Close() }()

	if readSize != size {
		t.Errorf("readSize mismatch: got %d, want %d", readSize, size)
	}

	// Verify streaming read matches without holding full copy in memory
	readHasher := blake3.New(32, nil)
	n, err := io.Copy(readHasher, reader)
	if err != nil {
		t.Fatalf("io.Copy failed: %v", err)
	}
	if n != size {
		t.Errorf("copied bytes mismatch: got %d, want %d", n, size)
	}
	readHash := hex.EncodeToString(readHasher.Sum(nil))
	if readHash != expectedHash {
		t.Errorf("read hash mismatch: got %s, want %s", readHash, expectedHash)
	}
}

func stringsToUpper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - ('a' - 'A')
		}
	}
	return string(b)
}
