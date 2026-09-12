package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/resolve"
)

func toolValidate(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_validate",
		Description: "Validate an immutable branch snapshot against the repository schema.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to validate",
				},
				"commit": map[string]any{
					"type":        "string",
					"description": "Optional reachable commit ID",
				},
			},
			"required": []string{"branch"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch string  `json:"branch"`
				Commit *string `json:"commit,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLValidateSchema(ctx, resolve.SchemaValidationRequest{
					Selector: resolve.SnapshotSelector{
						Branch: in.Branch,
						Commit: in.Commit,
					},
				})
			})
		},
	}
}
