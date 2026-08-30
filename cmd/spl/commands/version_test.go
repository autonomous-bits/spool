package commands

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/version"
)

func TestVersionCommandOutputsValidJSON(t *testing.T) {
	var output bytes.Buffer
	command := NewVersionCommand()
	command.SetOut(&output)

	if err := command.Execute(); err != nil {
		t.Fatalf("execute version command: %v", err)
	}

	var result version.Info
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode version result: %v, raw output: %s", err, output.String())
	}

	if result.Version == "" {
		t.Fatal("expected non-empty version")
	}
	if result.GoVersion == "" {
		t.Fatal("expected non-empty goVersion")
	}
	if result.Platform == "" {
		t.Fatal("expected non-empty platform")
	}
	if !strings.Contains(result.Platform, "/") {
		t.Fatalf("expected platform in os/arch format, got %q", result.Platform)
	}
}
