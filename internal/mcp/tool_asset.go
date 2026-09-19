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
)

const maxAssetReadBytes = 32 * 1024 * 1024 // 32MB limit for MCP tool responses

func toolAssetAdd(rt *runtime) Tool {
	return Tool{
		Name:        "spl_asset_add",
		Description: "Ingest a reference document into the bound context git checkout and open a short-lived branch + PR. Unbound workspaces refuse writes.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
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
				"author": map[string]any{
					"type":        "string",
					"description": "Optional git author",
				},
				"message": map[string]any{
					"type":        "string",
					"description": "Optional commit/PR message",
				},
			},
			"required": []string{"file"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				FilePath string `json:"file"`
				Title    string `json:"title,omitempty"`
				ID       string `json:"id,omitempty"`
				Author   string `json:"author,omitempty"`
				Message  string `json:"message,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.FilePath == "" {
				return nil, errors.New("file is required")
			}
			fileInfo, err := os.Stat(in.FilePath)
			if err != nil {
				return nil, fmt.Errorf("stat asset file: %w", err)
			}
			if fileInfo.IsDir() {
				return nil, fmt.Errorf("path %q is a directory, expected a regular file", in.FilePath)
			}
			session, err := rt.requireSession(ctx)
			if err != nil {
				return nil, err
			}
			added, write, err := session.AddAsset(ctx, in.FilePath, in.ID, in.Title, in.Author, in.Message)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"node":        added.Node,
				"assetUri":    added.AssetURI,
				"hash":        added.Hash,
				"size":        added.Size,
				"mimeType":    added.MIMEType,
				"branch":      write.Branch,
				"commit":      write.Commit,
				"pullRequest": write.PR,
				"status":      added.Status,
			}, nil
		},
	}
}

func toolAssetRead(rt *runtime) Tool {
	return Tool{
		Name:        "spl_asset_read",
		Description: "Read asset blob contents from the bound context checkout (hash, assets/ path, or Asset node ID).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target": map[string]any{
					"type":        "string",
					"description": "Asset locator (assets/<hash>[/name]), raw hash, or graph node ID",
				},
			},
			"required": []string{"target"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Target string `json:"target"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Target == "" {
				return nil, errors.New("target is required")
			}
			session, err := rt.requireSession(ctx)
			if err != nil {
				return nil, err
			}
			reader, size, meta, err := session.ReadAsset(ctx, in.Target)
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
		},
	}
}
