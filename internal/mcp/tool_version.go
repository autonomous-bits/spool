package mcp

import (
	"context"
	"encoding/json"

	"github.com/autonomous-bits/spool/internal/version"
)

func toolVersion() Tool {
	return Tool{
		Name:        "spl_version",
		Description: "Print Spool release version, commit SHA, build date, Go runtime version, and platform as JSON.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
			return version.Get(), nil
		},
	}
}
