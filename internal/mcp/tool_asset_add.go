package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/autonomous-bits/spool/internal/repository"
)

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
