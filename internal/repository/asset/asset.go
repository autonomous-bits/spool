// Package asset provides content-addressable storage, URI resolution,
// and MIME detection for contextual reference assets in Spool repositories.
package asset

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/autonomous-bits/spool/graphcontract"
	"lukechampine.com/blake3"
)

const (
	// URIPrefix is the canonical URI scheme prefix for stored repository assets.
	URIPrefix = "spool://assets/"

	// HashHexLength is the required hexadecimal character length of a 256-bit BLAKE3 hash.
	HashHexLength = 64

	// DefaultBufferPoolSize is the chunk size used for streaming asset reads/writes.
	DefaultBufferPoolSize = 32 * 1024

	// LooseSubdir is the repository state subdirectory for loose asset blobs.
	LooseSubdir = "assets/loose"
)

var (
	// ErrAssetNotFound reports that a requested asset blob is absent from storage.
	ErrAssetNotFound = errors.New("asset not found")
	// ErrInvalidAssetURI reports an asset URI that does not conform to the canonical scheme.
	ErrInvalidAssetURI = errors.New("invalid asset URI")
	// ErrInvalidAssetHash reports an invalid or non-hexadecimal BLAKE3 asset hash.
	ErrInvalidAssetHash = errors.New("invalid asset hash")
	// ErrCorruptAsset reports an asset blob whose stored contents do not match its content hash.
	ErrCorruptAsset = errors.New("corrupt asset blob")
)

// Metadata captures the descriptive properties of an asset stored in Spool.
type Metadata struct {
	AssetURI         string `json:"assetUri"`
	Hash             string `json:"hash"`
	ByteSize         int64  `json:"byteSize"`
	MIMEType         string `json:"mimeType"`
	OriginalFilename string `json:"originalFilename,omitempty"`
}

// AddRequest specifies parameters for ingesting an asset into repository storage.
type AddRequest struct {
	Branch   string
	FilePath string
	Reader   io.Reader
	Filename string
	Title    string
	ID       string
}

// AddResult defines the structured outcome of ingesting an asset into Spool,
// complying with contract-asset-add-cli-payload.
type AddResult struct {
	Node     string `json:"node"`
	AssetURI string `json:"assetUri"`
	Hash     string `json:"hash"`
	Size     int64  `json:"size"`
	MIMEType string `json:"mimeType"`
	Branch   string `json:"branch"`
	Staged   bool   `json:"staged"`
	Status   string `json:"status"`
}

// Store manages content-addressable storage for loose asset blobs.
type Store struct {
	mu       sync.RWMutex
	looseDir string
	memStore map[string][]byte
}

// NewStore initializes an asset Store. If stateDir is non-empty, blobs are
// durably persisted beneath filepath.Join(stateDir, "assets", "loose").
// If stateDir is empty, blobs are retained in memory.
func NewStore(stateDir string) *Store {
	store := &Store{
		memStore: make(map[string][]byte),
	}
	if stateDir != "" {
		store.looseDir = filepath.Join(stateDir, "assets", "loose")
	}
	return store
}

// LooseDir returns the on-disk directory where loose blobs are stored, or "" if in-memory.
func (s *Store) LooseDir() string {
	return s.looseDir
}

// FormatLocator returns the canonical URI for a given BLAKE3 hash.
func FormatLocator(hash string) string {
	return URIPrefix + strings.ToLower(strings.TrimSpace(hash))
}

// ParseLocator parses a locator string, which may be a canonical URI
// (spool://assets/{hash}) or a bare 64-character hex hash, and returns
// the normalized lowercase BLAKE3 hash.
func ParseLocator(locator string) (string, error) {
	trimmed := strings.TrimSpace(locator)
	hasPrefix := strings.HasPrefix(trimmed, URIPrefix)
	if hasPrefix {
		trimmed = strings.TrimPrefix(trimmed, URIPrefix)
	} else if strings.Contains(trimmed, "://") {
		return "", fmt.Errorf("%w: %q (unsupported URI scheme)", ErrInvalidAssetURI, locator)
	}
	trimmed = strings.ToLower(trimmed)
	if !IsValidHash(trimmed) {
		if hasPrefix {
			return "", fmt.Errorf("%w: %q (invalid hash in URI)", ErrInvalidAssetURI, locator)
		}
		return "", fmt.Errorf("%w: %q", ErrInvalidAssetHash, locator)
	}
	return trimmed, nil
}

// IsValidHash reports whether hash is a valid 64-character lowercase hexadecimal BLAKE3 hash.
func IsValidHash(hash string) bool {
	if len(hash) != HashHexLength {
		return false
	}
	for i := 0; i < len(hash); i++ {
		c := hash[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// BlobPath returns the sharded on-disk path for a hash: looseDir/hash[:2]/hash[2:].
func (s *Store) BlobPath(hash string) (string, error) {
	if s.looseDir == "" {
		return "", errors.New("asset store has no durable directory")
	}
	if !IsValidHash(hash) {
		return "", fmt.Errorf("%w: %q", ErrInvalidAssetHash, hash)
	}
	return filepath.Join(s.looseDir, hash[:2], hash[2:]), nil
}

// HasBlob reports whether the blob identified by hash exists in the store.
func (s *Store) HasBlob(hash string) (bool, error) {
	cleanHash, err := ParseLocator(hash)
	if err != nil {
		return false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.looseDir == "" {
		_, ok := s.memStore[cleanHash]
		return ok, nil
	}

	path, err := s.BlobPath(cleanHash)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(path); err == nil {
		return true, nil
	}

	// Also check non-sharded fallback path: looseDir/hash
	flatPath := filepath.Join(s.looseDir, cleanHash)
	if _, err := os.Stat(flatPath); err == nil {
		return true, nil
	}
	return false, nil
}

// WriteBlob streams data from r, computes its BLAKE3-256 hash, and atomically
// stores the blob into the content-addressable store. If an identical blob
// already exists, writing is deduplicated.
func (s *Store) WriteBlob(r io.Reader) (hash string, size int64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.looseDir == "" {
		// In-memory mode
		data, err := io.ReadAll(r)
		if err != nil {
			return "", 0, fmt.Errorf("read asset data: %w", err)
		}
		sum := blake3.Sum256(data)
		h := hex.EncodeToString(sum[:])
		s.memStore[h] = data
		return h, int64(len(data)), nil
	}

	if err := os.MkdirAll(s.looseDir, 0o755); err != nil {
		return "", 0, fmt.Errorf("create loose asset directory: %w", err)
	}

	tempFile, err := os.CreateTemp(s.looseDir, ".tmp-asset-*")
	if err != nil {
		return "", 0, fmt.Errorf("create temporary asset file: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		if tempPath != "" {
			_ = os.Remove(tempPath)
		}
	}()

	hasher := blake3.New(32, nil)
	writer := io.MultiWriter(tempFile, hasher)

	buf := make([]byte, DefaultBufferPoolSize)
	written, err := io.CopyBuffer(writer, r, buf)
	if err != nil {
		_ = tempFile.Close()
		return "", 0, fmt.Errorf("write asset payload: %w", err)
	}

	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return "", 0, fmt.Errorf("sync temporary asset file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return "", 0, fmt.Errorf("close temporary asset file: %w", err)
	}

	sum := hasher.Sum(nil)
	hash = hex.EncodeToString(sum)
	size = written

	destPath, err := s.BlobPath(hash)
	if err != nil {
		return "", 0, err
	}

	// Check if already durably present (deduplication)
	if _, err := os.Stat(destPath); err == nil {
		return hash, size, nil
	}

	if err := ensureDurableDirectory(filepath.Dir(destPath)); err != nil {
		return "", 0, fmt.Errorf("create asset shard directory: %w", err)
	}

	if err := replaceDurableFile(tempPath, destPath); err != nil {
		return "", 0, fmt.Errorf("commit asset blob %s: %w", hash, err)
	}
	tempPath = "" // Disarm defer removal on success

	return hash, size, nil
}

// ReadBlob opens the asset blob identified by hash for streaming. The caller is
// responsible for closing the returned ReadCloser.
func (s *Store) ReadBlob(hash string) (io.ReadCloser, int64, error) {
	cleanHash, err := ParseLocator(hash)
	if err != nil {
		return nil, 0, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.looseDir == "" {
		data, ok := s.memStore[cleanHash]
		if !ok {
			return nil, 0, fmt.Errorf("%w: %s", ErrAssetNotFound, cleanHash)
		}
		return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
	}

	destPath, err := s.BlobPath(cleanHash)
	if err != nil {
		return nil, 0, err
	}

	file, err := os.Open(destPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Try non-sharded fallback path: looseDir/hash
			flatPath := filepath.Join(s.looseDir, cleanHash)
			fallbackFile, fallbackErr := os.Open(flatPath)
			if fallbackErr == nil {
				info, statErr := fallbackFile.Stat()
				if statErr != nil {
					_ = fallbackFile.Close()
					return nil, 0, fmt.Errorf("stat asset blob: %w", statErr)
				}
				return fallbackFile, info.Size(), nil
			}
			return nil, 0, fmt.Errorf("%w: %s", ErrAssetNotFound, cleanHash)
		}
		return nil, 0, fmt.Errorf("open asset blob %s: %w", cleanHash, err)
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, fmt.Errorf("stat asset blob %s: %w", cleanHash, err)
	}
	return file, info.Size(), nil
}

// DetectMIMEType determines the MIME type of content based on filename extension
// and leading peek bytes. If the extension is known, it takes precedence.
// If indeterminate, it sniffs leading content bytes, defaulting to application/octet-stream.
func DetectMIMEType(filename string, peek []byte) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".md", ".markdown":
		return "text/markdown; charset=utf-8"
	case ".json":
		return "application/json"
	case ".svg":
		return "image/svg+xml"
	case ".yaml", ".yml":
		return "application/yaml"
	case ".toml":
		return "application/toml"
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".csv":
		return "text/csv; charset=utf-8"
	case ".xml":
		return "application/xml"
	}

	if ext != "" {
		if t := mime.TypeByExtension(ext); t != "" {
			return t
		}
	}

	if len(peek) > 0 {
		sniffed := http.DetectContentType(peek)
		if sniffed != "" && sniffed != "application/octet-stream" {
			return sniffed
		}
	}
	return "application/octet-stream"
}

// ExtractAssetHash checks whether node has an assetUri property and extracts
// its canonical BLAKE3 hash.
func ExtractAssetHash(node graphcontract.Node) (string, bool) {
	val, ok := node.Properties["assetUri"]
	if !ok || val.Kind != graphcontract.PropertyString {
		return "", false
	}
	hash, err := ParseLocator(val.String)
	if err != nil {
		return "", false
	}
	return hash, true
}

// ExtractAssetHashes inspects a map of nodes and returns a sorted, unique list
// of referenced asset hashes.
func ExtractAssetHashes(nodes map[string]graphcontract.Node) []string {
	seen := make(map[string]struct{})
	for _, node := range nodes {
		if hash, ok := ExtractAssetHash(node); ok {
			seen[hash] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	hashes := make([]string, 0, len(seen))
	for h := range seen {
		hashes = append(hashes, h)
	}
	sort.Strings(hashes)
	return hashes
}

func replaceDurableFile(tempPath, path string) error {
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func ensureDurableDirectory(path string) error {
	var missing []string
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Stat(current)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("durable directory %q is not a directory", current)
			}
			break
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("inspect durable directory: %w", err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return fmt.Errorf("find existing durable directory parent for %q", path)
		}
		missing = append(missing, current)
	}
	if len(missing) == 0 {
		return nil
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	for index := len(missing) - 1; index >= 0; index-- {
		if err := syncDirectory(filepath.Dir(missing[index])); err != nil {
			return err
		}
	}
	return syncDirectory(path)
}

func syncDirectory(path string) (err error) {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory for sync: %w", err)
	}
	defer func() {
		if closeErr := directory.Close(); closeErr != nil {
			closeErr = fmt.Errorf("close directory: %w", closeErr)
			if err == nil {
				err = closeErr
			} else {
				err = errors.Join(err, closeErr)
			}
		}
	}()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync directory: %w", err)
	}
	return nil
}
