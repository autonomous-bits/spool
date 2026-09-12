package repository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/autonomous-bits/spool/internal/remote"
	"github.com/autonomous-bits/spool/internal/repository/asset"
)

// StageAsset ingests an external reference document into the loose content-addressable
// storage and stages a corresponding Asset node into the branch mutation set.
func (r *Repository) StageAsset(ctx context.Context, req AssetAddRequest) (AssetAddResult, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return AssetAddResult{}, err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.ensureOpenLocked(); err != nil {
		return AssetAddResult{}, err
	}

	if req.Branch == "" {
		return AssetAddResult{}, ErrBranchRequired
	}
	head, exists := r.branches[req.Branch]
	if !exists {
		return AssetAddResult{}, ErrBranchNotFound
	}

	var reader io.Reader
	var filename string

	if req.Reader != nil {
		reader = req.Reader
		filename = req.Filename
	} else if req.FilePath != "" {
		file, err := os.Open(req.FilePath)
		if err != nil {
			return AssetAddResult{}, fmt.Errorf("open asset file: %w", err)
		}
		defer func() { _ = file.Close() }()
		reader = file
		filename = filepath.Base(req.FilePath)
	} else {
		return AssetAddResult{}, errors.New("either FilePath or Reader is required")
	}

	// Buffer up to 512 leading bytes to sniff MIME content type
	header := make([]byte, 512)
	n, err := io.ReadFull(reader, header)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return AssetAddResult{}, fmt.Errorf("read asset header: %w", err)
	}
	peek := header[:n]
	fullReader := io.MultiReader(bytes.NewReader(peek), reader)

	hash, size, err := r.assetStore.WriteBlob(fullReader)
	if err != nil {
		return AssetAddResult{}, fmt.Errorf("store asset blob: %w", err)
	}

	mimeType := asset.DetectMIMEType(filename, peek)
	assetURI := asset.FormatLocator(hash)

	nodeID := req.ID
	if nodeID == "" {
		nodeID = "asset-" + hash[:16]
	}

	title := req.Title
	if title == "" {
		if filename != "" {
			title = filename
		} else {
			title = "Asset " + hash[:8]
		}
	}

	props := map[string]PropertyValue{
		"assetUri": StringPropertyValue(assetURI),
		"byteSize": IntegerPropertyValue(size),
		"mimeType": StringPropertyValue(mimeType),
	}
	if filename != "" {
		props["originalFilename"] = StringPropertyValue(filename)
	}

	op := MutationOperation{
		Action:     "add",
		Entity:     "node",
		ID:         nodeID,
		Title:      title,
		Labels:     []string{"Asset"},
		Properties: props,
	}

	// Stage the operation on the branch
	var stagedOps []MutationOperation
	if existingStaged, ok := r.stagedMutations[req.Branch]; ok {
		updated := false
		stagedOps = make([]MutationOperation, len(existingStaged.Operations))
		copy(stagedOps, existingStaged.Operations)
		for i, existingOp := range stagedOps {
			if existingOp.Entity == "node" && existingOp.ID == nodeID {
				stagedOps[i] = op
				updated = true
				break
			}
		}
		if !updated {
			stagedOps = append(stagedOps, op)
		}
	} else {
		stagedOps = []MutationOperation{op}
	}

	normalizedOps, err := normalizeMutationOperations(stagedOps)
	if err != nil {
		return AssetAddResult{}, err
	}

	targetSchema := r.stagedMutations[req.Branch].TargetSchema
	staged := StagedMutationSet{
		Branch:       req.Branch,
		BaseCommit:   head,
		Operations:   normalizedOps,
		TargetSchema: targetSchema,
	}

	if _, _, err := r.candidateGraphLocked(head, staged); err != nil {
		return AssetAddResult{}, err
	}

	if _, err := r.replaceStagedMutationsLocked(staged); err != nil {
		return AssetAddResult{}, err
	}

	return AssetAddResult{
		Node:     nodeID,
		AssetURI: assetURI,
		Hash:     hash,
		Size:     size,
		MIMEType: mimeType,
		Branch:   req.Branch,
		Staged:   true,
		Status:   "staged",
	}, nil
}

// OpenAsset opens the content-addressed asset blob keyed by BLAKE3 hash for streaming.
func (r *Repository) OpenAsset(hash string) (io.ReadCloser, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.ensureOpenLocked(); err != nil {
		return nil, 0, err
	}
	return r.assetStore.ReadBlob(hash)
}

// ResolveAssetNode inspects a branch (checking staged mutations first, then the snapshot projection)
// to resolve an asset node by ID, extracting its canonical BLAKE3 hash and metadata.
func (r *Repository) ResolveAssetNode(branch, nodeID string) (string, AssetMetadata, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.ensureOpenLocked(); err != nil {
		return "", AssetMetadata{}, err
	}

	if branch == "" {
		branch = r.activeBranch
	}
	if _, exists := r.branches[branch]; !exists {
		return "", AssetMetadata{}, ErrBranchNotFound
	}

	var targetNode *Node

	// 1. Check staged mutations on branch first (reverse traversal for latest modification)
	if staged, exists := r.stagedMutations[branch]; exists {
		for i := len(staged.Operations) - 1; i >= 0; i-- {
			op := staged.Operations[i]
			if op.Entity == "node" && op.ID == nodeID {
				if op.Action == "delete" {
					return "", AssetMetadata{}, ErrNodeNotFound
				}
				n := Node{ID: op.ID, Title: op.Title, Labels: op.Labels, Properties: op.Properties}
				targetNode = &n
				break
			}
		}
	}

	// 2. If not found in staged, check branch head commit snapshot
	if targetNode == nil {
		head := r.branches[branch]
		commit, ok := r.commits[head]
		if !ok {
			return "", AssetMetadata{}, ErrCommitNotFound
		}
		if err := r.ensureSnapshotProjectionLocked(commit.Snapshot); err != nil {
			return "", AssetMetadata{}, err
		}
		snapshot := r.snapshots[commit.Snapshot]
		nodes := r.projections[snapshot.NodeRoot]
		if n, ok := nodes[nodeID]; ok {
			targetNode = &n
		}
	}

	if targetNode == nil {
		return "", AssetMetadata{}, ErrNodeNotFound
	}

	val, ok := targetNode.Properties["assetUri"]
	if !ok || val.Kind != PropertyString {
		return "", AssetMetadata{}, fmt.Errorf("node %q is not an asset (missing assetUri property)", nodeID)
	}

	cleanHash, err := asset.ParseLocator(val.String)
	if err != nil {
		return "", AssetMetadata{}, fmt.Errorf("node %q has invalid asset locator %q: %w", nodeID, val.String, err)
	}

	meta := AssetMetadata{
		AssetURI: asset.FormatLocator(cleanHash),
		Hash:     cleanHash,
	}
	if sz, ok := targetNode.Properties["byteSize"]; ok && sz.Kind == PropertyInteger {
		meta.ByteSize = sz.Integer
	}
	if mt, ok := targetNode.Properties["mimeType"]; ok && mt.Kind == PropertyString {
		meta.MIMEType = mt.String
	}
	if fn, ok := targetNode.Properties["originalFilename"]; ok && fn.Kind == PropertyString {
		meta.OriginalFilename = fn.String
	}

	return cleanHash, meta, nil
}

// ReadAsset resolves a locator (spool://assets/{hash} or raw hash) or node ID on branch,
// opening the underlying content-addressable blob stream. If the asset blob is missing
// locally and a Rack remote is configured, it attempts on-demand retrieval from the remote.
func (r *Repository) ReadAsset(ctx context.Context, branch, locatorOrNode string) (io.ReadCloser, int64, AssetMetadata, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, 0, AssetMetadata{}, err
		}
	}

	cleanHash, parseErr := asset.ParseLocator(locatorOrNode)
	if parseErr == nil {
		reader, size, err := r.openOrFetchAsset(ctx, cleanHash)
		if err != nil {
			return nil, 0, AssetMetadata{}, err
		}
		return reader, size, AssetMetadata{
			AssetURI: asset.FormatLocator(cleanHash),
			Hash:     cleanHash,
			ByteSize: size,
		}, nil
	}

	// Try resolving as a node ID on branch
	hash, meta, err := r.ResolveAssetNode(branch, locatorOrNode)
	if err != nil {
		return nil, 0, AssetMetadata{}, fmt.Errorf("resolve locator or node %q: %w", locatorOrNode, err)
	}

	reader, size, err := r.openOrFetchAsset(ctx, hash)
	if err != nil {
		return nil, 0, AssetMetadata{}, err
	}
	if meta.ByteSize == 0 {
		meta.ByteSize = size
	}
	return reader, size, meta, nil
}

func (r *Repository) openOrFetchAsset(ctx context.Context, hash string) (io.ReadCloser, int64, error) {
	reader, size, err := r.OpenAsset(hash)
	if err == nil {
		return reader, size, nil
	}
	if !errors.Is(err, asset.ErrAssetNotFound) {
		return nil, 0, err
	}

	// Check if a remote is configured
	cfg, ok, remoteErr := r.Remote()
	if remoteErr != nil || !ok {
		return nil, 0, err
	}

	// Resolve credential (from Keychain or Env)
	credential, _ := remote.ResolveCredential(cfg.WorkspaceOrRepoID(), cfg.AuthMode, remote.ResolveOptions{
		Keychain: remote.KeyringStore{},
		Getenv:   os.Getenv,
	})

	body, _, _, streamErr := remote.StreamAsset(ctx, nil, cfg, credential.Value, hash)
	if streamErr != nil {
		// Return original ErrAssetNotFound wrapped with stream error context
		return nil, 0, fmt.Errorf("%w (remote fetch failed: %v)", asset.ErrAssetNotFound, streamErr)
	}
	defer func() { _ = body.Close() }()

	// Store downloaded blob in local assetStore
	storedHash, _, writeErr := r.assetStore.WriteBlob(body)
	if writeErr != nil {
		return nil, 0, fmt.Errorf("store fetched asset blob: %w", writeErr)
	}
	if storedHash != hash {
		return nil, 0, fmt.Errorf("%w: fetched asset hash mismatch: got %s, want %s", asset.ErrCorruptAsset, storedHash, hash)
	}

	return r.OpenAsset(hash)
}

