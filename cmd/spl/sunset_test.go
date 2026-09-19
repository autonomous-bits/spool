package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultModulesHaveNoRackDependency(t *testing.T) {
	t.Parallel()
	files := []string{"go.mod", "go.sum", filepath.Join("..", "go.mod"), filepath.Join("..", "go.sum")}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := string(data)
		for _, needle := range []string{"spool-rack", "github.com/autonomous-bits/rack"} {
			if strings.Contains(text, needle) {
				t.Errorf("%s must not depend on %q", file, needle)
			}
		}
	}
}

func TestSunsetCopyClosesSoftDoor(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..")
	softDoor := []string{
		"remain for local graph-VCS",
		"remains for local graph-VCS",
		"still exist for local graph-VCS",
		"for local graph-VCS only",
		"local graph-VCS",
	}
	stopList := []string{"Spool-as-VCS", "Rack sync", ".spl", "pack wire-compat"}
	changelog, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("read CHANGELOG.md: %v", err)
	}
	unreleased := string(changelog)
	if idx := strings.Index(unreleased, "## [1."); idx > 0 {
		unreleased = unreleased[:idx]
	}
	for _, item := range stopList {
		if !strings.Contains(unreleased, item) {
			t.Errorf("CHANGELOG Unreleased must name stop-list item %q", item)
		}
	}

	paths := []string{
		filepath.Join(root, "README.md"),
		filepath.Join(root, "docs", "context-bind.md"),
		filepath.Join(root, "docs", "architecture.md"),
		filepath.Join(root, "docs", "context-git-migration.md"),
		filepath.Join(root, ".agents", "skills", "spool", "SKILL.md"),
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(data)
		for _, phrase := range softDoor {
			if strings.Contains(text, phrase) {
				t.Errorf("%s still contains soft-door wording %q", path, phrase)
			}
		}
	}
}
