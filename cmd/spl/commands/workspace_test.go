package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/autonomous-bits/spool/internal/repository"
	workspacepkg "github.com/autonomous-bits/spool/internal/workspace"
)

func TestWorkspaceInitCreatesWorkspaceAndRegistry(t *testing.T) {
	root := t.TempDir()

	var output bytes.Buffer
	if err := runWorkspaceCommand(root, []string{"init", "alpha"}, &output); err != nil {
		t.Fatalf("run workspace init: %v", err)
	}

	var created workspacepkg.Workspace
	if err := json.Unmarshal(output.Bytes(), &created); err != nil {
		t.Fatalf("decode init output: %v", err)
	}
	if created.Name != "alpha" {
		t.Fatalf("created workspace name = %q, want alpha", created.Name)
	}
	if created.ID == "" {
		t.Fatal("created workspace ID is empty")
	}
	if created.StateDir == "" {
		t.Fatal("created workspace state directory is empty")
	}
	registryPath, err := workspacepkg.RegistryPath(root)
	if err != nil {
		t.Fatalf("registry path: %v", err)
	}
	if _, err := os.Stat(registryPath); err != nil {
		t.Fatalf("stat registry: %v", err)
	}
	registry, err := workspacepkg.LoadRegistry(root)
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	stored, ok := registry.Workspaces[workspacepkg.Name("alpha")]
	if !ok {
		t.Fatal("workspace alpha not found in registry")
	}
	if stored.ID != created.ID {
		t.Fatalf("stored ID = %q, want %q", stored.ID, created.ID)
	}
}

// TestWorkspaceInitInitializesRepositoryState verifies that "workspace init"
// eagerly creates the workspace's backing repository state, so a separate
// "spl init" step is no longer required before the workspace is usable.
func TestWorkspaceInitInitializesRepositoryState(t *testing.T) {
	root := t.TempDir()

	var output bytes.Buffer
	if err := runWorkspaceCommand(root, []string{"init", "alpha"}, &output); err != nil {
		t.Fatalf("run workspace init: %v", err)
	}
	var created workspacepkg.Workspace
	if err := json.Unmarshal(output.Bytes(), &created); err != nil {
		t.Fatalf("decode init output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(created.StateDir, "config.toml")); err != nil {
		t.Fatalf("stat initialized state config: %v", err)
	}
	repo, err := repository.OpenRepository(created.StateDir)
	if err != nil {
		t.Fatalf("open initialized workspace state: %v", err)
	}
	init, err := repo.Initialization()
	if err != nil {
		t.Fatalf("read initialization: %v", err)
	}
	if init.DefaultBranch != "main" {
		t.Fatalf("default branch = %q, want main", init.DefaultBranch)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("close repository: %v", err)
	}
}

// TestWorkspaceInitDoesNotRegisterOnInitializationFailure verifies that a
// failure to initialize the workspace's state directory does not leave a
// dangling workspace registered with no backing repository state: the
// registry entry is only written after initialization succeeds.
func TestWorkspaceInitDoesNotRegisterOnInitializationFailure(t *testing.T) {
	root := t.TempDir()
	// Block directory creation under root/repos so that initializing the
	// new workspace's state directory fails deterministically.
	if err := os.WriteFile(filepath.Join(root, "repos"), []byte("blocked"), 0o600); err != nil {
		t.Fatalf("create blocking file: %v", err)
	}

	var output bytes.Buffer
	err := runWorkspaceCommand(root, []string{"init", "alpha"}, &output)
	if err == nil {
		t.Fatal("workspace init error = nil, want error")
	}
	if output.Len() != 0 {
		t.Fatalf("failed init wrote success output: %q", output.String())
	}
	registry, loadErr := workspacepkg.LoadRegistry(root)
	if loadErr != nil {
		t.Fatalf("load registry: %v", loadErr)
	}
	if _, exists := registry.Workspaces[workspacepkg.Name("alpha")]; exists {
		t.Fatal("registry contains workspace alpha despite initialization failure")
	}
}

func TestWorkspaceInitRejectsDuplicateName(t *testing.T) {
	root := t.TempDir()

	if err := runWorkspaceCommand(root, []string{"init", "alpha"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("first workspace init: %v", err)
	}
	var output bytes.Buffer
	err := runWorkspaceCommand(root, []string{"init", "alpha"}, &output)
	if err == nil {
		t.Fatal("duplicate init error = nil, want error")
	}
	if !strings.Contains(err.Error(), `workspace "alpha" already exists`) {
		t.Fatalf("duplicate init error = %q, want already-exists message", err)
	}
	if output.Len() != 0 {
		t.Fatalf("duplicate init wrote success output: %q", output.String())
	}
}

// TestWorkspaceInitSerializesConcurrentSameNameRequests verifies that
// concurrent "workspace init" calls for the same name cannot both reserve an
// identity and initialize state: identity reservation, state initialization,
// and the registry write all happen under one held registry lock, so exactly
// one call succeeds and the rest fail with an already-exists error rather
// than racing to initialize orphaned or contended state.
func TestWorkspaceInitSerializesConcurrentSameNameRequests(t *testing.T) {
	root := t.TempDir()
	const attempts = 8

	var wg sync.WaitGroup
	results := make([]error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = runWorkspaceCommand(root, []string{"init", "alpha"}, &bytes.Buffer{})
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
			continue
		}
		if !strings.Contains(err.Error(), `workspace "alpha" already exists`) {
			t.Fatalf("unexpected concurrent init error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful concurrent inits = %d, want 1", successes)
	}

	registry, err := workspacepkg.LoadRegistry(root)
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if len(registry.Workspaces) != 1 {
		t.Fatalf("registered workspaces = %d, want 1", len(registry.Workspaces))
	}
	stored := registry.Workspaces[workspacepkg.Name("alpha")]
	repo, err := repository.OpenRepository(stored.StateDir)
	if err != nil {
		t.Fatalf("open registered workspace state: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("close workspace state: %v", err)
	}
}

func TestWorkspaceUseSucceedsAndPersistsPreference(t *testing.T) {
	root := t.TempDir()
	if err := runWorkspaceCommand(root, []string{"init", "alpha"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var output bytes.Buffer
	if err := runWorkspaceCommand(root, []string{"use", "alpha"}, &output); err != nil {
		t.Fatalf("use workspace: %v", err)
	}

	var result struct {
		Slug     string   `json:"slug"`
		Name     string   `json:"name"`
		StateDir string   `json:"stateDir"`
		Paths    []string `json:"paths"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode use output: %v", err)
	}
	if result.Slug != "alpha" {
		t.Fatalf("use output slug = %q, want alpha", result.Slug)
	}
	if result.Name != "alpha" {
		t.Fatalf("use output name = %q, want alpha", result.Name)
	}
	if result.StateDir == "" {
		t.Fatal("use output stateDir is empty")
	}
	if len(result.Paths) != 0 {
		t.Fatalf("use output paths = %#v, want empty", result.Paths)
	}

	current, ok, err := workspacepkg.CurrentWorkspaceName(root)
	if err != nil {
		t.Fatalf("load current workspace preference: %v", err)
	}
	if !ok {
		t.Fatal("current workspace preference not persisted")
	}
	if current != workspacepkg.Name("alpha") {
		t.Fatalf("current workspace preference = %q, want alpha", current)
	}
}

func TestWorkspaceUseRejectsMissingWorkspace(t *testing.T) {
	root := t.TempDir()

	var output bytes.Buffer
	err := runWorkspaceCommand(root, []string{"use", "missing"}, &output)
	if !errors.Is(err, workspacepkg.ErrWorkspaceNotRegistered) {
		t.Fatalf("use missing workspace error = %v, want ErrWorkspaceNotRegistered", err)
	}
	if !strings.Contains(err.Error(), `workspace "missing" is not registered`) {
		t.Fatalf("use missing workspace error = %q, want not-registered message", err)
	}
	if output.Len() != 0 {
		t.Fatalf("use missing workspace wrote success output: %q", output.String())
	}
	current, ok, err := workspacepkg.CurrentWorkspaceName(root)
	if err != nil {
		t.Fatalf("load current workspace preference: %v", err)
	}
	if ok {
		t.Fatalf("current workspace preference = %q, want unset", current)
	}
}

func TestWorkspaceUseRejectsMissingArgsAndInvalidName(t *testing.T) {
	root := t.TempDir()

	var output bytes.Buffer
	err := runWorkspaceCommand(root, []string{"use"}, &output)
	if err == nil {
		t.Fatal("use without args error = nil, want error")
	}
	if output.Len() != 0 {
		t.Fatalf("use without args wrote success output: %q", output.String())
	}

	output.Reset()
	err = runWorkspaceCommand(root, []string{"use", "Alpha"}, &output)
	if !errors.Is(err, workspacepkg.ErrInvalidName) {
		t.Fatalf("use invalid workspace name error = %v, want ErrInvalidName", err)
	}
	if output.Len() != 0 {
		t.Fatalf("use invalid workspace name wrote success output: %q", output.String())
	}
}

func TestWorkspaceUnsetClearsPersistedPreference(t *testing.T) {
	root := t.TempDir()
	if err := runWorkspaceCommand(root, []string{"init", "alpha"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if err := runWorkspaceCommand(root, []string{"use", "alpha"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("use workspace: %v", err)
	}

	var output bytes.Buffer
	if err := runWorkspaceCommand(root, []string{"unset"}, &output); err != nil {
		t.Fatalf("unset workspace: %v", err)
	}

	var result struct {
		Cleared bool `json:"cleared"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode unset output: %v", err)
	}
	if !result.Cleared {
		t.Fatal("unset output cleared = false, want true")
	}

	_, ok, err := workspacepkg.CurrentWorkspaceName(root)
	if err != nil {
		t.Fatalf("load current workspace preference: %v", err)
	}
	if ok {
		t.Fatal("current workspace preference still set after unset")
	}
}

func TestWorkspaceUnsetWithoutPreferenceReportsNotCleared(t *testing.T) {
	root := t.TempDir()

	var output bytes.Buffer
	if err := runWorkspaceCommand(root, []string{"unset"}, &output); err != nil {
		t.Fatalf("unset workspace: %v", err)
	}

	var result struct {
		Cleared bool `json:"cleared"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode unset output: %v", err)
	}
	if result.Cleared {
		t.Fatal("unset output cleared = true, want false when no preference was set")
	}
}

func TestWorkspaceUnsetRecoversFromStalePreference(t *testing.T) {
	root := t.TempDir()
	if err := runWorkspaceCommand(root, []string{"init", "alpha"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if err := runWorkspaceCommand(root, []string{"use", "alpha"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("use workspace: %v", err)
	}
	if err := workspacepkg.UpdateRegistry(root, func(registry *workspacepkg.Registry) error {
		delete(registry.Workspaces, workspacepkg.Name("alpha"))
		return nil
	}); err != nil {
		t.Fatalf("remove workspace from registry: %v", err)
	}

	var output bytes.Buffer
	if err := runWorkspaceCommand(root, []string{"unset"}, &output); err != nil {
		t.Fatalf("unset stale workspace preference: %v", err)
	}

	var result struct {
		Cleared bool `json:"cleared"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode unset output: %v", err)
	}
	if !result.Cleared {
		t.Fatal("unset output cleared = false, want true for a stale preference")
	}
	_, ok, err := workspacepkg.CurrentWorkspaceName(root)
	if err != nil {
		t.Fatalf("load current workspace preference: %v", err)
	}
	if ok {
		t.Fatal("current workspace preference still set after unset")
	}
}

func TestWorkspaceAttachSucceeds(t *testing.T) {
	root := t.TempDir()
	repositoryPath := filepath.Join(root, "repo")
	if err := os.MkdirAll(repositoryPath, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := runWorkspaceCommand(root, []string{"init", "alpha"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var output bytes.Buffer
	if err := runWorkspaceCommand(root, []string{"attach", "--workspace", "alpha", repositoryPath}, &output); err != nil {
		t.Fatalf("attach path: %v", err)
	}

	var result workspacePathResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode attach output: %v", err)
	}
	canonicalPath, err := workspacepkg.CanonicalPath(repositoryPath)
	if err != nil {
		t.Fatalf("canonicalize repository path: %v", err)
	}
	if result.Workspace != "alpha" || result.Attached != canonicalPath {
		t.Fatalf("attach result = %#v, want workspace alpha attached %q", result, canonicalPath)
	}

	registry, err := workspacepkg.LoadRegistry(root)
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if got := registry.Workspaces[workspacepkg.Name("alpha")].Paths; len(got) != 1 || got[0] != canonicalPath {
		t.Fatalf("attached paths = %#v, want [%q]", got, canonicalPath)
	}
}

func TestWorkspaceAttachRejectsAlreadyAttachedAndMissingWorkspace(t *testing.T) {
	root := t.TempDir()
	repositoryPath := filepath.Join(root, "repo")
	if err := os.MkdirAll(repositoryPath, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	for _, name := range []string{"alpha", "beta"} {
		if err := runWorkspaceCommand(root, []string{"init", name}, &bytes.Buffer{}); err != nil {
			t.Fatalf("init workspace %s: %v", name, err)
		}
	}
	if err := runWorkspaceCommand(root, []string{"attach", "--workspace", "alpha", repositoryPath}, &bytes.Buffer{}); err != nil {
		t.Fatalf("attach to alpha: %v", err)
	}

	var output bytes.Buffer
	err := runWorkspaceCommand(root, []string{"attach", "--workspace", "beta", repositoryPath}, &output)
	if !errors.Is(err, workspacepkg.ErrPathAlreadyAttached) {
		t.Fatalf("attach duplicate path error = %v, want ErrPathAlreadyAttached", err)
	}
	if output.Len() != 0 {
		t.Fatalf("duplicate attach wrote success output: %q", output.String())
	}

	output.Reset()
	err = runWorkspaceCommand(root, []string{"attach", "--workspace", "missing", repositoryPath}, &output)
	if !errors.Is(err, workspacepkg.ErrInvalidWorkspace) {
		t.Fatalf("attach missing workspace error = %v, want ErrInvalidWorkspace", err)
	}
	if output.Len() != 0 {
		t.Fatalf("missing workspace attach wrote success output: %q", output.String())
	}
}

func TestWorkspaceDetachSucceedsAndRemovesPath(t *testing.T) {
	root := t.TempDir()
	repositoryPath := filepath.Join(root, "repo")
	if err := os.MkdirAll(repositoryPath, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := runWorkspaceCommand(root, []string{"init", "alpha"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if err := runWorkspaceCommand(root, []string{"attach", "--workspace", "alpha", repositoryPath}, &bytes.Buffer{}); err != nil {
		t.Fatalf("attach workspace: %v", err)
	}

	var output bytes.Buffer
	if err := runWorkspaceCommand(root, []string{"detach", repositoryPath}, &output); err != nil {
		t.Fatalf("detach workspace path: %v", err)
	}

	var result workspacePathResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode detach output: %v", err)
	}
	canonicalPath, err := workspacepkg.CanonicalPath(repositoryPath)
	if err != nil {
		t.Fatalf("canonicalize repository path: %v", err)
	}
	if result.Workspace != "alpha" || result.Detached != canonicalPath {
		t.Fatalf("detach result = %#v, want workspace alpha detached %q", result, canonicalPath)
	}
	registry, err := workspacepkg.LoadRegistry(root)
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if got := registry.Workspaces[workspacepkg.Name("alpha")].Paths; len(got) != 0 {
		t.Fatalf("attached paths after detach = %#v, want empty", got)
	}
}

func TestWorkspaceDetachRejectsMissingPath(t *testing.T) {
	root := t.TempDir()
	repositoryPath := filepath.Join(root, "repo")
	if err := os.MkdirAll(repositoryPath, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	var output bytes.Buffer
	err := runWorkspaceCommand(root, []string{"detach", repositoryPath}, &output)
	if err == nil {
		t.Fatal("detach missing path error = nil, want error")
	}
	if !strings.Contains(err.Error(), "is not attached to any workspace") {
		t.Fatalf("detach missing path error = %q, want not-attached message", err)
	}
	if output.Len() != 0 {
		t.Fatalf("detach missing path wrote success output: %q", output.String())
	}
}

func TestWorkspaceDetachSucceedsAfterAttachedRepositoryIsDeleted(t *testing.T) {
	root := t.TempDir()
	repositoryPath := filepath.Join(root, "repo")
	if err := os.MkdirAll(repositoryPath, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := runWorkspaceCommand(root, []string{"init", "alpha"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if err := runWorkspaceCommand(root, []string{"attach", "--workspace", "alpha", repositoryPath}, &bytes.Buffer{}); err != nil {
		t.Fatalf("attach workspace: %v", err)
	}
	canonicalPath, err := workspacepkg.CanonicalPath(repositoryPath)
	if err != nil {
		t.Fatalf("canonicalize repository path: %v", err)
	}
	if err := os.RemoveAll(repositoryPath); err != nil {
		t.Fatalf("remove attached repository: %v", err)
	}

	var output bytes.Buffer
	if err := runWorkspaceCommand(root, []string{"detach", repositoryPath}, &output); err != nil {
		t.Fatalf("detach path whose repository was deleted: %v", err)
	}
	var result workspacePathResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode detach output: %v", err)
	}
	if result.Workspace != "alpha" || result.Detached != canonicalPath {
		t.Fatalf("detach result = %#v, want workspace alpha detached %q", result, canonicalPath)
	}
	registry, err := workspacepkg.LoadRegistry(root)
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if got := registry.Workspaces[workspacepkg.Name("alpha")].Paths; len(got) != 0 {
		t.Fatalf("attached paths after detach = %#v, want empty", got)
	}
}

func TestWorkspaceListHandlesEmptyRegistryAndSortedOutput(t *testing.T) {
	root := t.TempDir()

	var output bytes.Buffer
	if err := runWorkspaceCommand(root, []string{"list"}, &output); err != nil {
		t.Fatalf("list empty registry: %v", err)
	}
	var empty []workspacepkg.Workspace
	if err := json.Unmarshal(output.Bytes(), &empty); err != nil {
		t.Fatalf("decode empty list output: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty list result = %#v, want empty", empty)
	}

	for _, name := range []string{"zeta", "alpha"} {
		if err := runWorkspaceCommand(root, []string{"init", name}, &bytes.Buffer{}); err != nil {
			t.Fatalf("init workspace %s: %v", name, err)
		}
	}
	alphaPath := filepath.Join(root, "alpha-repo")
	zetaPath := filepath.Join(root, "zeta-repo")
	for _, path := range []string{alphaPath, zetaPath} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir attached repo %q: %v", path, err)
		}
	}
	if err := runWorkspaceCommand(root, []string{"attach", "--workspace", "zeta", zetaPath}, &bytes.Buffer{}); err != nil {
		t.Fatalf("attach zeta path: %v", err)
	}
	if err := runWorkspaceCommand(root, []string{"attach", "--workspace", "alpha", alphaPath}, &bytes.Buffer{}); err != nil {
		t.Fatalf("attach alpha path: %v", err)
	}

	output.Reset()
	if err := runWorkspaceCommand(root, []string{"list"}, &output); err != nil {
		t.Fatalf("list populated registry: %v", err)
	}
	var listed []workspacepkg.Workspace
	if err := json.Unmarshal(output.Bytes(), &listed); err != nil {
		t.Fatalf("decode populated list output: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("listed workspaces count = %d, want 2", len(listed))
	}
	if listed[0].Name != "alpha" || listed[1].Name != "zeta" {
		t.Fatalf("listed workspace order = [%q %q], want [alpha zeta]", listed[0].Name, listed[1].Name)
	}
	alphaCanonical, err := workspacepkg.CanonicalPath(alphaPath)
	if err != nil {
		t.Fatalf("canonicalize alpha path: %v", err)
	}
	zetaCanonical, err := workspacepkg.CanonicalPath(zetaPath)
	if err != nil {
		t.Fatalf("canonicalize zeta path: %v", err)
	}
	if len(listed[0].Paths) != 1 || listed[0].Paths[0] != alphaCanonical {
		t.Fatalf("alpha listed paths = %#v, want [%q]", listed[0].Paths, alphaCanonical)
	}
	if len(listed[1].Paths) != 1 || listed[1].Paths[0] != zetaCanonical {
		t.Fatalf("zeta listed paths = %#v, want [%q]", listed[1].Paths, zetaCanonical)
	}
}

func TestWorkspaceListAndCurrentExposeRegistrySlugSeparatelyFromDisplayName(t *testing.T) {
	root := t.TempDir()
	repositoryPath := filepath.Join(root, "repo")
	if err := os.MkdirAll(repositoryPath, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	canonicalPath, err := workspacepkg.CanonicalPath(repositoryPath)
	if err != nil {
		t.Fatalf("canonicalize repository path: %v", err)
	}
	// Registry keys are stable slugs while Workspace.Name is a separate
	// display name; construct an entry where they diverge, as can already
	// happen for registries not solely produced by "workspace init".
	if err := workspacepkg.UpdateRegistry(root, func(registry *workspacepkg.Registry) error {
		registry.Workspaces[workspacepkg.Name("ecommerce-platform")] = workspacepkg.Workspace{
			ID:        "ws_00000001",
			Name:      "E-Commerce Platform",
			StateDir:  filepath.Join(root, "repos", "ws_00000001"),
			CreatedAt: time.Now().UTC(),
			Paths:     []string{canonicalPath},
		}
		return nil
	}); err != nil {
		t.Fatalf("seed registry: %v", err)
	}

	var listOutput bytes.Buffer
	if err := runWorkspaceCommand(root, []string{"list"}, &listOutput); err != nil {
		t.Fatalf("list: %v", err)
	}
	var listed []struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(listOutput.Bytes(), &listed); err != nil {
		t.Fatalf("decode list output: %v", err)
	}
	if len(listed) != 1 || listed[0].Slug != "ecommerce-platform" || listed[0].Name != "E-Commerce Platform" {
		t.Fatalf("listed = %#v, want slug ecommerce-platform with display name E-Commerce Platform", listed)
	}

	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(repositoryPath); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalWorkingDirectory)
	}()

	var currentOutput bytes.Buffer
	if err := runWorkspaceCommand(root, []string{"current"}, &currentOutput); err != nil {
		t.Fatalf("current: %v", err)
	}
	var current struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(currentOutput.Bytes(), &current); err != nil {
		t.Fatalf("decode current output: %v", err)
	}
	if current.Slug != "ecommerce-platform" || current.Name != "E-Commerce Platform" {
		t.Fatalf("current = %#v, want slug ecommerce-platform with display name E-Commerce Platform", current)
	}
}

func TestWorkspaceCurrentFindsAttachedWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	repositoryPath := filepath.Join(root, "repo")
	if err := os.MkdirAll(repositoryPath, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := runWorkspaceCommand(root, []string{"init", "alpha"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if err := runWorkspaceCommand(root, []string{"attach", "--workspace", "alpha", repositoryPath}, &bytes.Buffer{}); err != nil {
		t.Fatalf("attach workspace: %v", err)
	}

	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(repositoryPath); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalWorkingDirectory)
	}()

	var output bytes.Buffer
	if err := runWorkspaceCommand(root, []string{"current"}, &output); err != nil {
		t.Fatalf("resolve current workspace: %v", err)
	}

	var current workspacepkg.Workspace
	if err := json.Unmarshal(output.Bytes(), &current); err != nil {
		t.Fatalf("decode current output: %v", err)
	}
	if current.Name != "alpha" {
		t.Fatalf("current workspace name = %q, want alpha", current.Name)
	}
}

func TestWorkspaceCurrentRejectsUnattachedWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	repositoryPath := filepath.Join(root, "repo")
	if err := os.MkdirAll(repositoryPath, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(repositoryPath); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalWorkingDirectory)
	}()

	var output bytes.Buffer
	err = runWorkspaceCommand(root, []string{"current"}, &output)
	if !errors.Is(err, workspacepkg.ErrWorkspaceNotFound) {
		t.Fatalf("current missing workspace error = %v, want ErrWorkspaceNotFound", err)
	}
	if !strings.Contains(err.Error(), "no workspace is registered for the current directory") {
		t.Fatalf("current missing workspace error = %q, want user-facing not-found message", err)
	}
	if output.Len() != 0 {
		t.Fatalf("current missing workspace wrote success output: %q", output.String())
	}
}

func runWorkspaceCommand(root string, args []string, output *bytes.Buffer) error {
	command := NewWorkspaceCommand(func() (string, error) {
		return root, nil
	})
	command.SetOut(output)
	command.SetArgs(args)
	return command.Execute()
}
