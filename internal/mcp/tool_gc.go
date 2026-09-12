package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/autonomous-bits/spool/internal/repository"
)

func toolGC(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_gc",
		Description: "Pack reachable objects into packfiles and prune unreachable loose objects past the retention grace period.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"dry_run": map[string]any{
					"type":        "boolean",
					"description": "Report maintenance work without modifying object storage",
				},
				"repack": map[string]any{
					"type":        "boolean",
					"description": "Compact active packs into one replacement generation",
				},
				"grace_period": map[string]any{
					"type":        "string",
					"description": "Duration string to retain unreachable loose objects (e.g. '336h')",
				},
			},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				DryRun      bool   `json:"dry_run,omitempty"`
				Repack      bool   `json:"repack,omitempty"`
				GracePeriod string `json:"grace_period,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}

			var graceDuration time.Duration
			if in.GracePeriod != "" {
				d, err := time.ParseDuration(in.GracePeriod)
				if err != nil {
					return nil, err
				}
				graceDuration = d
			}

			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				result, err := repo.GC(repository.GCOptions{
					DryRun:      in.DryRun,
					Repack:      in.Repack,
					GracePeriod: graceDuration,
				})
				if err != nil {
					var warning *repository.GCCommittedWithWarningError
					if errors.As(err, &warning) {
						return result, err
					}
					return nil, err
				}
				return result, nil
			})
		},
	}
}
