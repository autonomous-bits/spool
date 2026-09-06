package graphcontract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const manifestFormatVersion = 1

// fixtureSet describes one versioned, directory-scoped conformance fixture
// set listed in testdata/MANIFEST.json.
type fixtureSet struct {
	Name          string `json:"name"`
	Directory     string `json:"directory"`
	FormatVersion int    `json:"format_version"`
	Description   string `json:"description"`
}

type fixtureManifest struct {
	FormatVersion int          `json:"format_version"`
	Package       string       `json:"package"`
	Description   string       `json:"description"`
	FixtureSets   []fixtureSet `json:"fixture_sets"`
}

// TestFixtureManifestListsEveryFixtureSet proves testdata/MANIFEST.json is
// the single authoritative, language-agnostic index a cross-repository
// conformance runner (Rack) can use to discover every fixture set: every
// listed directory must exist and contain at least one JSON fixture.
func TestFixtureManifestListsEveryFixtureSet(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "MANIFEST.json"))
	if err != nil {
		t.Fatalf("read fixture manifest: %v", err)
	}
	var manifest fixtureManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode fixture manifest: %v", err)
	}
	if manifest.FormatVersion != manifestFormatVersion {
		t.Fatalf("manifest format version = %d, want %d", manifest.FormatVersion, manifestFormatVersion)
	}
	if len(manifest.FixtureSets) == 0 {
		t.Fatal("fixture manifest lists no fixture sets")
	}
	seen := make(map[string]bool, len(manifest.FixtureSets))
	for _, set := range manifest.FixtureSets {
		if seen[set.Name] {
			t.Fatalf("fixture set %q listed more than once", set.Name)
		}
		seen[set.Name] = true
		if set.FormatVersion == 0 {
			t.Fatalf("fixture set %q: format_version is required", set.Name)
		}
		paths, err := filepath.Glob(filepath.Join(set.Directory, "*.json"))
		if err != nil {
			t.Fatalf("fixture set %q: list fixtures: %v", set.Name, err)
		}
		if len(paths) == 0 {
			t.Fatalf("fixture set %q: directory %s has no JSON fixtures", set.Name, set.Directory)
		}
	}
}
