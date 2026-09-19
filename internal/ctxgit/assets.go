package ctxgit

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/repository/asset"
	"lukechampine.com/blake3"
)

func isPlainGitPath(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".json", ".toml", ".txt", ".md", ".csv", ".yaml", ".yml":
		return true
	}
	return strings.EqualFold(name, schemaFileName) || strings.EqualFold(name, readmeFileName)
}

func shouldUseLFS(name string, size, threshold int64) bool {
	if isPlainGitPath(name) {
		return false
	}
	return size >= threshold
}

func shouldWarnLargeText(name string, size int64) bool {
	return isPlainGitPath(name) && size > LargeTextWarnBytes
}

func assetRelPath(hash, filename string) string {
	hash = strings.ToLower(strings.TrimSpace(hash))
	filename = filepath.Base(strings.TrimSpace(filename))
	if filename == "" || filename == "." || filename == string(os.PathSeparator) {
		return filepath.ToSlash(filepath.Join(assetsDirName, hash))
	}
	return filepath.ToSlash(filepath.Join(assetsDirName, hash, filename))
}

// StagedAsset is a blob waiting to be written with the next mutation commit.
type StagedAsset struct {
	Hash     string
	Size     int64
	MIMEType string
	Filename string
	RelPath  string
	Bytes    []byte
	UseLFS   bool
	Warn     string
}

func ingestAsset(filePath string, threshold int64) (StagedAsset, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return StagedAsset{}, fmt.Errorf("read asset: %w", err)
	}
	return ingestAssetBytes(filepath.Base(filePath), data, threshold)
}

func ingestAssetBytes(filename string, data []byte, threshold int64) (StagedAsset, error) {
	sum := blake3.Sum256(data)
	hash := fmt.Sprintf("%x", sum[:])
	peek := data
	if len(peek) > 512 {
		peek = peek[:512]
	}
	mimeType := asset.DetectMIMEType(filename, peek)
	rel := assetRelPath(hash, filename)
	staged := StagedAsset{
		Hash:     hash,
		Size:     int64(len(data)),
		MIMEType: mimeType,
		Filename: filename,
		RelPath:  rel,
		Bytes:    data,
		UseLFS:   shouldUseLFS(filename, int64(len(data)), threshold),
	}
	if shouldWarnLargeText(filename, int64(len(data))) {
		staged.Warn = fmt.Sprintf("%s is %d bytes (plain git; consider splitting above ~1 MiB)", rel, len(data))
	}
	return staged, nil
}

func persistAsset(ctx context.Context, git GitRunner, root string, staged StagedAsset) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	abs := filepath.Join(root, filepath.FromSlash(staged.RelPath))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(abs, staged.Bytes, 0o644); err != nil {
		return err
	}
	if !staged.UseLFS {
		return nil
	}
	if _, err := git.Run(ctx, root, "lfs", "version"); err != nil {
		return fmt.Errorf("asset %s is %d bytes and requires Git LFS (threshold %d bytes), but git lfs is not available: %w", staged.RelPath, staged.Size, DefaultLFSThreshold, err)
	}
	if _, err := git.Run(ctx, root, "lfs", "track", staged.RelPath); err != nil {
		return fmt.Errorf("git lfs track %s: %w", staged.RelPath, err)
	}
	return nil
}

func assetNodeOperation(staged StagedAsset, id, title, repositoryID string) repository.MutationOperation {
	if id == "" {
		id = "asset-" + staged.Hash[:16]
	}
	if title == "" {
		if staged.Filename != "" {
			title = staged.Filename
		} else {
			title = "Asset " + staged.Hash[:8]
		}
	}
	props := map[string]repository.PropertyValue{
		"assetPath": repository.StringPropertyValue(staged.RelPath),
		"hash":      repository.StringPropertyValue(staged.Hash),
		"byteSize":  repository.IntegerPropertyValue(staged.Size),
		"mimeType":  repository.StringPropertyValue(staged.MIMEType),
	}
	if staged.Filename != "" {
		props["originalFilename"] = repository.StringPropertyValue(staged.Filename)
	}
	return repository.MutationOperation{
		Action:     "add",
		Entity:     "node",
		ID:         NamespaceID(repositoryID, id),
		Title:      title,
		Labels:     []string{"Asset"},
		Properties: props,
	}
}

func readCheckoutAsset(root, locator string) (io.ReadCloser, int64, asset.Metadata, error) {
	locator = strings.TrimSpace(locator)
	hash := locator
	if parsed, err := asset.ParseLocator(locator); err == nil {
		hash = parsed
	}
	hash = strings.TrimPrefix(hash, assetsDirName+"/")
	hash = strings.Trim(hash, "/")
	if i := strings.IndexByte(hash, '/'); i >= 0 {
		hash = hash[:i]
	}
	if !asset.IsValidHash(hash) {
		// Treat locator as a node id; caller should resolve the node first.
		return nil, 0, asset.Metadata{}, fmt.Errorf("asset locator %q is not a content hash or assets/ path", locator)
	}
	base := filepath.Join(root, assetsDirName, hash)
	info, err := os.Stat(base)
	if err != nil {
		return nil, 0, asset.Metadata{}, fmt.Errorf("open asset %s: %w", hash, err)
	}
	path := base
	filename := ""
	if info.IsDir() {
		entries, err := os.ReadDir(base)
		if err != nil {
			return nil, 0, asset.Metadata{}, err
		}
		if len(entries) == 0 {
			return nil, 0, asset.Metadata{}, fmt.Errorf("asset %s directory is empty", hash)
		}
		filename = entries[0].Name()
		path = filepath.Join(base, filename)
		info, err = os.Stat(path)
		if err != nil {
			return nil, 0, asset.Metadata{}, err
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, asset.Metadata{}, err
	}
	return f, info.Size(), asset.Metadata{
		Hash:             hash,
		ByteSize:         info.Size(),
		OriginalFilename: filename,
	}, nil
}
