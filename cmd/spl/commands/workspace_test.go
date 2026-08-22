package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
