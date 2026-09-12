package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/autonomous-bits/spool/internal/repository"
)

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
				reader, mimeType, size, err := repo.ReadAsset(ctx, in.Branch, in.Target)
				if err != nil {
					return nil, err
				}
				defer func() { _ = reader.Close() }()
				data, err := io.ReadAll(reader)
				if err != nil {
					return nil, err
				}
				return map[string]any{
					"mimeType": mimeType,
					"size":     size,
					"content":  string(data),
				}, nil
			})
		},
	}
}
