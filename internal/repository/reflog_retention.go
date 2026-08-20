package repository

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	reflogRetentionInventoryFilename = "reflog-retention"
	reflogRetentionInventoryHeader   = "spool-reflog-retention-v1"
)

var errReflogRetentionInventoryMissing = errors.New("reflog retention inventory is missing")

// reflogRetentionInventory is intentionally a small canonical text file. Its
// paths are relative to logs/, so moving the repository does not change it.
func encodeReflogRetentionInventory(paths []string) ([]byte, error) {
	canonical, err := canonicalReflogRetentionPaths(paths)
	if err != nil {
		return nil, err
	}
	lines := append([]string{reflogRetentionInventoryHeader}, canonical...)
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

func canonicalReflogRetentionPaths(paths []string) ([]string, error) {
	canonical := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		normalized, err := canonicalReflogRetentionPath(path)
		if err != nil {
			return nil, err
		}
		if normalized != path {
			return nil, fmt.Errorf("reflog retention path %q is not canonical", path)
		}
		if _, duplicate := seen[path]; duplicate {
			return nil, fmt.Errorf("duplicate reflog retention path %q", path)
		}
		seen[path] = struct{}{}
		canonical = append(canonical, path)
	}
	sort.Strings(canonical)
	return canonical, nil
}

func canonicalReflogRetentionPath(path string) (string, error) {
	if path == "HEAD" {
		return path, nil
	}
	const refPrefix = "refs/heads/"
	if !strings.HasPrefix(path, refPrefix) {
		return "", fmt.Errorf("reflog retention path %q is outside the reflog namespace", path)
	}
	branch := strings.TrimPrefix(path, refPrefix)
	if !validRefName(branch) {
		return "", fmt.Errorf("reflog retention path %q has an invalid branch name", path)
	}
	return refPrefix + branch, nil
}

func readReflogRetentionInventory(path string) ([]string, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, errReflogRetentionInventoryMissing
	}
	if err != nil {
		return nil, fmt.Errorf("inspect reflog retention inventory: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("reflog retention inventory is not a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read reflog retention inventory: %w", err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		return nil, errors.New("reflog retention inventory is missing a final newline")
	}
	lines := strings.Split(string(data[:len(data)-1]), "\n")
	if len(lines) == 0 || lines[0] != reflogRetentionInventoryHeader {
		return nil, errors.New("reflog retention inventory has an unsupported format")
	}
	paths := append([]string(nil), lines[1:]...)
	canonical, err := canonicalReflogRetentionPaths(paths)
	if err != nil {
		return nil, err
	}
	if !sameReflogRetentionPaths(paths, canonical) {
		return nil, errors.New("reflog retention inventory paths are not canonically ordered")
	}
	return paths, nil
}

func sameReflogRetentionPaths(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func reflogRetentionPathSet(paths []string) map[string]struct{} {
	set := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		set[path] = struct{}{}
	}
	return set
}

func discoverReflogRetentionPaths(root string) ([]string, error) {
	paths := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if !entry.Type().IsRegular() {
			return fmt.Errorf("reflog %q is not a regular file", relative)
		}
		if _, err := canonicalReflogRetentionPath(relative); err != nil {
			return err
		}
		paths = append(paths, relative)
		return nil
	})
	if os.IsNotExist(err) {
		return nil, errors.New("reflog directory is missing")
	}
	if err != nil {
		return nil, fmt.Errorf("discover reflogs: %w", err)
	}
	return canonicalReflogRetentionPaths(paths)
}

func validateReflogRetentionInventory(root, inventoryPath string) ([]string, error) {
	paths, err := readReflogRetentionInventory(inventoryPath)
	if err != nil {
		return nil, err
	}
	actual, err := discoverReflogRetentionPaths(root)
	if err != nil {
		return nil, err
	}
	expected := reflogRetentionPathSet(paths)
	for _, path := range actual {
		if _, found := expected[path]; !found {
			return nil, fmt.Errorf("unexpected reflog %q is absent from the retention inventory", path)
		}
	}
	actualSet := reflogRetentionPathSet(actual)
	for _, path := range paths {
		if _, found := actualSet[path]; !found {
			return nil, fmt.Errorf("reflog %q listed by the retention inventory is missing", path)
		}
		if err := validateReflogRetentionFile(filepath.Join(root, filepath.FromSlash(path)), path); err != nil {
			return nil, fmt.Errorf("validate reflog %q: %w", path, err)
		}
	}
	return paths, nil
}

func (r *Repository) reflogRetentionInventoryPath() string {
	return filepath.Join(r.mergeStateDir, reflogRetentionInventoryFilename)
}

func (r *Repository) writeReflogRetentionInventoryLocked(paths []string) error {
	if r.writeReflogRetentionInventoryFn != nil {
		return r.writeReflogRetentionInventoryFn(append([]string(nil), paths...))
	}
	data, err := encodeReflogRetentionInventory(paths)
	if err != nil {
		return err
	}
	return writeDurableStateFile(r.reflogRetentionInventoryPath(), data)
}

func (r *Repository) initializeReflogRetentionInventoryLocked(inventoryExpected bool) error {
	inventoryPath := r.reflogRetentionInventoryPath()
	_, err := readReflogRetentionInventory(inventoryPath)
	switch {
	case err == nil:
		if _, err := validateReflogRetentionInventory(r.reflogDirectory(), inventoryPath); err != nil {
			return err
		}
		if !inventoryExpected {
			return r.writeConfigLocked()
		}
		return nil
	case !errors.Is(err, errReflogRetentionInventoryMissing):
		return err
	case inventoryExpected:
		return err
	}

	paths, err := discoverReflogRetentionPaths(r.reflogDirectory())
	if err != nil {
		return err
	}
	for _, path := range paths {
		if err := validateReflogRetentionFile(filepath.Join(r.reflogDirectory(), filepath.FromSlash(path)), path); err != nil {
			return err
		}
	}
	if err := r.writeReflogRetentionInventoryLocked(paths); err != nil {
		return err
	}
	return r.writeConfigLocked()
}

func (r *Repository) recordReflogRetentionPathLocked(path string, prepared bool) error {
	if r.mergeStateDir == "" {
		return nil
	}
	normalized, err := canonicalReflogRetentionPath(path)
	if err != nil {
		return err
	}
	paths, err := readReflogRetentionInventory(r.reflogRetentionInventoryPath())
	if err != nil {
		return err
	}
	actual, err := discoverReflogRetentionPaths(r.reflogDirectory())
	if err != nil {
		return err
	}
	expected := reflogRetentionPathSet(paths)
	for _, actualPath := range actual {
		if _, found := expected[actualPath]; found {
			continue
		}
		if prepared && actualPath == normalized {
			continue
		}
		return fmt.Errorf("unexpected reflog %q is absent from the retention inventory", actualPath)
	}
	actualSet := reflogRetentionPathSet(actual)
	for _, expectedPath := range paths {
		if _, found := actualSet[expectedPath]; !found {
			return fmt.Errorf("reflog %q listed by the retention inventory is missing", expectedPath)
		}
		if err := validateReflogRetentionFile(filepath.Join(r.reflogDirectory(), filepath.FromSlash(expectedPath)), expectedPath); err != nil {
			return fmt.Errorf("validate reflog %q: %w", expectedPath, err)
		}
	}
	for _, existing := range paths {
		if existing == normalized {
			return nil
		}
	}
	paths = append(paths, normalized)
	return r.writeReflogRetentionInventoryLocked(paths)
}

func validateReflogRetentionFile(path, relative string) error {
	if relative == "HEAD" {
		return validateHeadReflog(path)
	}
	return readObjectReflog(path, &retentionRoots{commits: make(map[ObjectID]struct{}), snapshots: make(map[ObjectID]struct{})})
}
