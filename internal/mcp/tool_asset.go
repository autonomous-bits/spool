package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"unicode/utf8"

	"github.com/autonomous-bits/spool/internal/repository"
)

const maxAssetReadBytes = 32 * 1024 * 1024 // 32MB limit for MCP tool responses

func toolAssetAdd(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_asset_add",
		Description: "Ingest a reference document into content-addressable storage and stage a corresponding Asset node on a branch.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch on which to stage the asset node",
				},
				"file": map[string]any{
					"type":        "string",
					"description": "Path to the reference document to ingest",
				},
				"title": map[string]any{
					"type":        "string",
					"description": "Optional descriptive title for the Asset graph node",
				},
				"id": map[string]any{
					"type":        "string",
					"description": "Optional explicit node ID for the Asset graph node",
				},
			},
			"required": []string{"branch", "file"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch   string `json:"branch"`
				FilePath string `json:"file"`
				Title    string `json:"title,omitempty"`
				ID       string `json:"id,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" || in.FilePath == "" {
				return nil, errors.New("branch and file are required")
			}
			fileInfo, err := os.Stat(in.FilePath)
			if err != nil {
				return nil, fmt.Errorf("stat asset file: %w", err)
			}
			if fileInfo.IsDir() {
				return nil, fmt.Errorf("path %q is a directory, expected a regular file", in.FilePath)
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				return repo.StageAsset(ctx, repository.AssetAddRequest{
					Branch:   in.Branch,
					FilePath: in.FilePath,
					Title:    in.Title,
					ID:       in.ID,
				})
			})
		},
	}
}

func toolAssetRead(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_asset_read",
		Description: "Read asset blob contents by locator URI (spool://assets/{hash}), hash, or graph node ID.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target": map[string]any{
					"type":        "string",
					"description": "Asset locator URI (spool://assets/{hash}), raw BLAKE3 hash, or graph node ID",
				},
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to resolve node ID from (defaults to active branch)",
				},
			},
			"required": []string{"target"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Target string `json:"target"`
				Branch string `json:"branch,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Target == "" {
				return nil, errors.New("target is required")
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				reader, size, meta, err := repo.ReadAsset(ctx, in.Branch, in.Target)
				if err != nil {
					return nil, err
				}
				defer func() { _ = reader.Close() }()

				limitedReader := io.LimitReader(reader, maxAssetReadBytes+1)
				data, err := io.ReadAll(limitedReader)
				if err != nil {
					return nil, err
				}
				if len(data) > maxAssetReadBytes {
					return nil, fmt.Errorf("asset size (%d bytes) exceeds maximum readable size (%d bytes) for MCP", size, maxAssetReadBytes)
				}

				encoding := "utf-8"
				content := string(data)
				if !utf8.Valid(data) {
					encoding = "base64"
					content = base64.StdEncoding.EncodeToString(data)
				}

				return map[string]any{
					"mimeType": meta.MIMEType,
					"size":     size,
					"encoding": encoding,
					"content":  content,
				}, nil
			})
		},
	}
}
