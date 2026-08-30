package version

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGetVersionInfo(t *testing.T) {
	info := Get()
	if info.Version == "" {
		t.Fatal("expected non-empty version")
	}
	if info.GoVersion == "" {
		t.Fatal("expected non-empty GoVersion")
	}
	if info.Platform == "" {
		t.Fatal("expected non-empty Platform")
	}
	if !strings.Contains(info.Platform, "/") {
		t.Fatalf("expected platform in os/arch format, got %q", info.Platform)
	}

	str := info.String()
	if !strings.HasPrefix(str, "spl version") {
		t.Fatalf("expected string representation to start with 'spl version', got %q", str)
	}

	jsonData, err := info.JSON()
	if err != nil {
		t.Fatalf("JSON marshal error: %v", err)
	}
	var unmarshaled Info
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("JSON unmarshal error: %v", err)
	}
	if unmarshaled.Version != info.Version {
		t.Fatalf("expected version %q, got %q", info.Version, unmarshaled.Version)
	}
}
