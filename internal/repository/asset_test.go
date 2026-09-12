package repository

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"lukechampine.com/blake3"
)

func TestRepositoryStageAssetAndRead(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}
	defer func() { _ = repo.Close() }()

	ctx := context.Background()

	// Ingest an asset file
	content := []byte("# Spool Asset Specification\n\nContextual asset document content.")
	docPath := filepath.Join(t.TempDir(), "spec.md")
	if err := os.WriteFile(docPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	addResult, err := repo.StageAsset(ctx, AssetAddRequest{
		Branch:   "main",
		FilePath: docPath,
		Title:    "Specification Document",
	})
	if err != nil {
		t.Fatalf("StageAsset: %v", err)
	}

	if addResult.Node == "" {
		t.Errorf("expected non-empty node ID")
	}
	if addResult.MIMEType != "text/markdown; charset=utf-8" {
		t.Errorf("expected text/markdown, got %s", addResult.MIMEType)
	}
	if addResult.Size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), addResult.Size)
	}
	if !addResult.Staged {
		t.Errorf("expected staged true")
	}

	// Staging status check
	status, err := repo.BranchStagingStatus("main")
	if err != nil {
		t.Fatalf("BranchStagingStatus: %v", err)
	}
	if status.Operations != 1 {
		t.Errorf("expected 1 staged operation, got %d", status.Operations)
	}

	// Resolve asset node while staged
	hash, meta, err := repo.ResolveAssetNode("main", addResult.Node)
	if err != nil {
		t.Fatalf("ResolveAssetNode while staged: %v", err)
	}
	if hash != addResult.Hash {
		t.Errorf("hash mismatch: got %s, want %s", hash, addResult.Hash)
	}
	if meta.OriginalFilename != "spec.md" {
		t.Errorf("originalFilename mismatch: got %s, want spec.md", meta.OriginalFilename)
	}

	// Read asset by node ID
	reader, size, _, err := repo.ReadAsset(ctx, "main", addResult.Node)
	if err != nil {
		t.Fatalf("ReadAsset by node: %v", err)
	}
	defer func() { _ = reader.Close() }()
	if size != int64(len(content)) {
		t.Errorf("size mismatch: got %d, want %d", size, len(content))
	}
	readBytes, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(readBytes, content) {
		t.Errorf("read content mismatch: got %q, want %q", readBytes, content)
	}

	// Read asset by locator URI
	readerURI, _, _, err := repo.ReadAsset(ctx, "main", addResult.AssetURI)
	if err != nil {
		t.Fatalf("ReadAsset by URI: %v", err)
	}
	defer func() { _ = readerURI.Close() }()
	readURIBytes, _ := io.ReadAll(readerURI)
	if !bytes.Equal(readURIBytes, content) {
		t.Errorf("read URI content mismatch: got %q, want %q", readURIBytes, content)
	}

	// Read asset by bare hash
	readerHash, _, _, err := repo.ReadAsset(ctx, "main", addResult.Hash)
	if err != nil {
		t.Fatalf("ReadAsset by hash: %v", err)
	}
	defer func() { _ = readerHash.Close() }()
	readHashBytes, _ := io.ReadAll(readerHash)
	if !bytes.Equal(readHashBytes, content) {
		t.Errorf("read hash content mismatch: got %q, want %q", readHashBytes, content)
	}

	// Commit staged mutations
	commitResult, err := repo.CommitStagedMutationBatch(CommitStagedMutationRequest{
		Branch:  "main",
		Author:  "Tester <tester@spool.io>",
		Message: "Commit staged asset",
	})
	if err != nil {
		t.Fatalf("CommitStagedMutationBatch: %v", err)
	}
	if commitResult.Commit == "" {
		t.Fatalf("expected non-empty commit hash")
	}

	// Resolve asset node after commit
	hashPostCommit, metaPostCommit, err := repo.ResolveAssetNode("main", addResult.Node)
	if err != nil {
		t.Fatalf("ResolveAssetNode after commit: %v", err)
	}
	if hashPostCommit != addResult.Hash {
		t.Errorf("hash mismatch after commit: got %s, want %s", hashPostCommit, addResult.Hash)
	}
	if metaPostCommit.MIMEType != "text/markdown; charset=utf-8" {
		t.Errorf("mimeType mismatch: got %s", metaPostCommit.MIMEType)
	}

	// Read asset after commit
	postReader, _, _, err := repo.ReadAsset(ctx, "main", addResult.Node)
	if err != nil {
		t.Fatalf("ReadAsset after commit: %v", err)
	}
	defer func() { _ = postReader.Close() }()
	postBytes, _ := io.ReadAll(postReader)
	if !bytes.Equal(postBytes, content) {
		t.Errorf("post-commit read mismatch: got %q, want %q", postBytes, content)
	}

	// Verify BuildPushPack extracts candidate asset hashes
	pack, err := repo.BuildPushPack(ctx, "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack: %v", err)
	}
	if len(pack.AssetHashes) != 1 || pack.AssetHashes[0] != addResult.Hash {
		t.Errorf("BuildPushPack AssetHashes mismatch: got %v, want [%s]", pack.AssetHashes, addResult.Hash)
	}
}

func TestRepositoryStageAssetErrors(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}
	defer func() { _ = repo.Close() }()

	ctx := context.Background()

	// Missing branch
	_, err = repo.StageAsset(ctx, AssetAddRequest{
		Branch: "",
		Reader: bytes.NewReader([]byte("data")),
	})
	if !errors.Is(err, ErrBranchRequired) {
		t.Errorf("expected ErrBranchRequired, got %v", err)
	}

	// Branch not found
	_, err = repo.StageAsset(ctx, AssetAddRequest{
		Branch: "nonexistent",
		Reader: bytes.NewReader([]byte("data")),
	})
	if !errors.Is(err, ErrBranchNotFound) {
		t.Errorf("expected ErrBranchNotFound, got %v", err)
	}

	// Missing file and reader
	_, err = repo.StageAsset(ctx, AssetAddRequest{
		Branch: "main",
	})
	if err == nil {
		t.Errorf("expected error when neither FilePath nor Reader provided")
	}

	// Read non-existent asset node
	_, _, err = repo.ResolveAssetNode("main", "nonexistent-node-id")
	if !errors.Is(err, ErrNodeNotFound) {
		t.Errorf("expected ErrNodeNotFound, got %v", err)
	}

	// Non-asset node (seed node)
	_, _, err = repo.ResolveAssetNode("main", SeedNodeID)
	if err == nil {
		t.Errorf("expected error for seed node without assetUri")
	}
}

func TestRepositoryReadAssetRemoteFallback(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}
	defer func() { _ = repo.Close() }()

	ctx := context.Background()
	content := []byte("remote blob data to stream on demand")
	sum := blake3.Sum256(content)
	hash := hex.EncodeToString(sum[:])

	// Mock remote server returning the blob
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/repos/my-repo/assets/blobs/"+hash {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(content)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	t.Setenv("SPOOL_RACK_TOKEN", "test-bearer-token")

	if err := repo.SetRemote(RemoteConfig{
		Endpoint: ts.URL,
		RepoID:   "my-repo",
		AuthMode: RemoteAuthModeBearer,
	}); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}

	// Verify asset is not initially present locally
	hasBlob, err := repo.assetStore.HasBlob(hash)
	if err != nil || hasBlob {
		t.Fatalf("expected asset to not exist locally, got hasBlob=%v, err=%v", hasBlob, err)
	}

	// ReadAsset should trigger remote fetch fallback
	reader, size, meta, err := repo.ReadAsset(ctx, "main", hash)
	if err != nil {
		t.Fatalf("ReadAsset with remote fallback: %v", err)
	}
	defer func() { _ = reader.Close() }()

	if size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), size)
	}
	if meta.Hash != hash {
		t.Errorf("expected hash %s, got %s", hash, meta.Hash)
	}

	readBytes, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(readBytes, content) {
		t.Errorf("content mismatch: got %q, want %q", readBytes, content)
	}

	// Verify asset is now cached locally in assetStore
	hasBlobNow, err := repo.assetStore.HasBlob(hash)
	if err != nil || !hasBlobNow {
		t.Fatalf("expected asset to be cached locally after fallback read")
	}
}

func TestReadAssetNodePrecedenceOverBareHash(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	defer func() { _ = repo.Close() }()

	ctx := context.Background()

	// 1. Create and store a raw asset blob A
	blobA := []byte("blob A content")
	hashA, _, err := repo.assetStore.WriteBlob(bytes.NewReader(blobA))
	if err != nil {
		t.Fatalf("WriteBlob A: %v", err)
	}

	// 2. Create another asset blob B
	blobB := []byte("blob B content (different from A)")
	hashB, _, err := repo.assetStore.WriteBlob(bytes.NewReader(blobB))
	if err != nil {
		t.Fatalf("WriteBlob B: %v", err)
	}

	// 3. Stage a node whose ID is EXACTLY hashA (a 64-char hex string),
	// but whose assetUri points to blob B.
	stageRes, err := repo.StageAsset(ctx, AssetAddRequest{
		Branch: "main",
		ID:     hashA, // Node ID is formatted as a 64-hex string
		Reader: bytes.NewReader(blobB),
		Title:  "Node With 64-Hex ID",
	})
	if err != nil {
		t.Fatalf("StageAsset: %v", err)
	}
	if stageRes.Hash != hashB {
		t.Fatalf("expected staged asset hash %s, got %s", hashB, stageRes.Hash)
	}

	// 4. Calling ReadAsset with hashA should resolve the NODE first (yielding blob B),
	// not the bare hash A!
	reader, size, meta, err := repo.ReadAsset(ctx, "main", hashA)
	if err != nil {
		t.Fatalf("ReadAsset: %v", err)
	}
	defer func() { _ = reader.Close() }()

	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(content, blobB) {
		t.Errorf("expected node ID precedence yielding blob B %q, got %q", blobB, content)
	}
	if size != int64(len(blobB)) {
		t.Errorf("expected size %d, got %d", len(blobB), size)
	}
	if meta.Hash != hashB {
		t.Errorf("expected meta hash %s, got %s", hashB, meta.Hash)
	}

	// 5. Explicit URI "spool://assets/" + hashA should still resolve blob A directly
	uriReader, _, _, err := repo.ReadAsset(ctx, "main", "spool://assets/"+hashA)
	if err != nil {
		t.Fatalf("ReadAsset by URI: %v", err)
	}
	defer func() { _ = uriReader.Close() }()
	uriContent, _ := io.ReadAll(uriReader)
	if !bytes.Equal(uriContent, blobA) {
		t.Errorf("expected URI locator yielding blob A %q, got %q", blobA, uriContent)
	}
}

func TestGCAssetRetentionAndPruning(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	defer func() { _ = repo.Close() }()

	ctx := context.Background()

	// 1. Stage an asset on main branch and commit it -> Reachable asset
	committedAssetContent := []byte("committed asset content")
	stagedResult, err := repo.StageAsset(ctx, AssetAddRequest{
		Branch: "main",
		Reader: bytes.NewReader(committedAssetContent),
		Title:  "Committed Asset",
	})
	if err != nil {
		t.Fatalf("StageAsset committed: %v", err)
	}
	committedHash := stagedResult.Hash

	_, err = repo.CommitStagedMutationBatch(CommitStagedMutationRequest{
		Branch:  "main",
		Message: "Add committed asset",
		Author:  "test-author",
	})
	if err != nil {
		t.Fatalf("CommitStagedMutationBatch: %v", err)
	}

	// 2. Stage another asset on main branch without committing -> Reachable via staged mutation
	stagedAssetContent := []byte("staged asset content")
	stagedResult2, err := repo.StageAsset(ctx, AssetAddRequest{
		Branch: "main",
		Reader: bytes.NewReader(stagedAssetContent),
		Title:  "Staged Asset",
	})
	if err != nil {
		t.Fatalf("StageAsset staged: %v", err)
	}
	stagedHash := stagedResult2.Hash

	// 3. Write unreferenced young asset directly to asset store
	youngAssetContent := []byte("young unreferenced asset")
	youngHash, _, err := repo.assetStore.WriteBlob(bytes.NewReader(youngAssetContent))
	if err != nil {
		t.Fatalf("WriteBlob young: %v", err)
	}

	// 4. Write unreferenced old asset directly to asset store, backdated past grace
	oldAssetContent := []byte("old unreferenced asset to be pruned")
	oldHash, _, err := repo.assetStore.WriteBlob(bytes.NewReader(oldAssetContent))
	if err != nil {
		t.Fatalf("WriteBlob old: %v", err)
	}

	oldPath, err := repo.assetStore.BlobPath(oldHash)
	if err != nil {
		t.Fatalf("BlobPath old: %v", err)
	}
	pastTime := time.Now().Add(-DefaultGCGracePeriod - 2*time.Hour)
	if err := os.Chtimes(oldPath, pastTime, pastTime); err != nil {
		t.Fatalf("Chtimes oldPath: %v", err)
	}

	// 5. Test DryRun first
	dryRunResult, err := repo.GC(GCOptions{DryRun: true})
	if err != nil {
		t.Fatalf("GC DryRun: %v", err)
	}
	if dryRunResult.PrunedLooseObjects < 1 {
		t.Errorf("DryRun expected PrunedLooseObjects >= 1, got %d", dryRunResult.PrunedLooseObjects)
	}

	// Verify old asset still exists after dry run
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("Old asset should still exist after DryRun: %v", err)
	}

	// 6. Run actual GC
	gcResult, err := repo.GC(GCOptions{})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}

	if gcResult.PrunedLooseObjects < 1 {
		t.Errorf("GC expected PrunedLooseObjects >= 1, got %d", gcResult.PrunedLooseObjects)
	}

	// 7. Verify states
	// Committed asset MUST exist
	hasCommitted, err := repo.assetStore.HasBlob(committedHash)
	if err != nil || !hasCommitted {
		t.Errorf("Committed asset %s should be retained, err: %v, has: %v", committedHash, err, hasCommitted)
	}

	// Staged asset MUST exist
	hasStaged, err := repo.assetStore.HasBlob(stagedHash)
	if err != nil || !hasStaged {
		t.Errorf("Staged asset %s should be retained, err: %v, has: %v", stagedHash, err, hasStaged)
	}

	// Young unreferenced asset MUST exist (within grace)
	hasYoung, err := repo.assetStore.HasBlob(youngHash)
	if err != nil || !hasYoung {
		t.Errorf("Young asset %s should be retained within grace, err: %v, has: %v", youngHash, err, hasYoung)
	}

	// Old unreferenced asset MUST be removed
	hasOld, err := repo.assetStore.HasBlob(oldHash)
	if err != nil {
		t.Fatalf("HasBlob old: %v", err)
	}
	if hasOld {
		t.Errorf("Old unreferenced asset %s should have been pruned by GC", oldHash)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("Old asset file %s still exists: %v", oldPath, err)
	}
}
