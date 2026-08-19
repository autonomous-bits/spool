package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
)

func TestNewLoggerWritesStructuredJSON(t *testing.T) {
	var output bytes.Buffer

	newLogger(&output).Error("command failed", "error", errors.New("invalid input"))

	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("decode log entry: %v", err)
	}
	if entry["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR", entry["level"])
	}
	if entry["msg"] != "command failed" {
		t.Errorf("msg = %v, want command failed", entry["msg"])
	}
	if entry["component"] != "spl" {
		t.Errorf("component = %v, want spl", entry["component"])
	}
	if entry["error"] != "invalid input" {
		t.Errorf("error = %v, want invalid input", entry["error"])
	}
}

func TestPersistentToolRetainsCreatedBranchAcrossCommandInstances(t *testing.T) {
	stateDir := t.TempDir()
	initialized, err := repository.InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	if err := initialized.Close(); err != nil {
		t.Fatalf("close initialized repository: %v", err)
	}
	createTool, closeCreate, err := openPersistentTool(stateDir)
	if err != nil {
		t.Fatalf("newPersistentTool for create: %v", err)
	}
	var createOutput bytes.Buffer
	createCommand := newRootCommand(&createOutput, createTool)
	createCommand.SetArgs([]string{"branch", "create", "feature", "--from-branch", "main"})
	if err := createCommand.Execute(); err != nil {
		t.Fatalf("create branch command: %v", err)
	}
	if err := closeCreate(); err != nil {
		t.Fatalf("close created repository: %v", err)
	}

	resolveTool, closeResolve, err := openPersistentTool(stateDir)
	if err != nil {
		t.Fatalf("newPersistentTool for resolve: %v", err)
	}

	t.Cleanup(func() {
		if err := closeResolve(); err != nil {
			t.Errorf("Close resolved repository: %v", err)
		}
	})
	var resolveOutput bytes.Buffer
	resolveCommand := newRootCommand(&resolveOutput, resolveTool)
	resolveCommand.SetArgs([]string{"resolve", "--branch", "feature", "--node", repository.SeedNodeID})
	if err := resolveCommand.Execute(); err != nil {
		t.Fatalf("resolve created branch command: %v", err)
	}
}

func TestPersistentToolRejectsUninitializedRepository(t *testing.T) {
	if _, _, err := openPersistentTool(t.TempDir()); !errors.Is(err, repository.ErrRepositoryNotInitialized) {
		t.Fatalf("openPersistentTool error = %v, want ErrRepositoryNotInitialized", err)
	}
}

func TestInitCommandCreatesDurableMainBranch(t *testing.T) {
	stateDir := t.TempDir()
	var output bytes.Buffer
	var closeRepository func() error
	command := newRootCommandWithLifecycle(
		&output,
		func() (*resolve.ResolveTool, error) {
			repo, err := repository.OpenRepository(stateDir)
			if err != nil {
				return nil, err
			}
			closeRepository = repo.Close
			return resolve.NewResolveTool(repo), nil
		},
		func() (*repository.Repository, error) {
			repo, err := repository.InitializeRepository(stateDir)
			if err == nil {
				closeRepository = repo.Close
			}
			return repo, err
		},
	)
	command.SetArgs([]string{"init"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	if err := closeRepository(); err != nil {
		t.Fatalf("close initialized repository: %v", err)
	}

	var result repository.Initialization
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode init result: %v", err)
	}
	if result != (repository.Initialization{DefaultBranch: "main", ActiveBranch: "main"}) {
		t.Fatalf("init result = %#v", result)
	}

	reopened, err := repository.OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("open initialized repository: %v", err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Errorf("Close reopened repository: %v", err)
		}
	})
	if result, err := reopened.Initialization(); err != nil || result != (repository.Initialization{DefaultBranch: "main", ActiveBranch: "main"}) {
		t.Fatalf("reopened initialization = %#v, %v", result, err)
	}
}

func TestInitCommandRejectsExistingRepositoryWithoutChangingDurableState(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := repository.InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("close initialized repository: %v", err)
	}

	statePath := filepath.Join(stateDir, "config.toml")
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read existing repository state: %v", err)
	}
	var output bytes.Buffer
	command := newRootCommandWithLifecycle(
		&output,
		func() (*resolve.ResolveTool, error) {
			t.Fatal("init command requested a persistent tool")
			return nil, nil
		},
		func() (*repository.Repository, error) {
			return repository.InitializeRepository(stateDir)
		},
	)
	command.SetArgs([]string{"init"})
	if err := command.Execute(); !errors.Is(err, repository.ErrRepositoryAlreadyInitialized) {
		t.Fatalf("execute init error = %v, want ErrRepositoryAlreadyInitialized", err)
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read preserved repository state: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("init command changed existing durable state")
	}
}

func TestRepositoryStateDirFindsRepositoryRootFromSubdirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.work"), []byte("go 1.26\n"), 0o600); err != nil {
		t.Fatalf("write go.work: %v", err)
	}
	subdirectory := filepath.Join(root, "cmd", "spl")
	if err := os.MkdirAll(subdirectory, 0o700); err != nil {
		t.Fatalf("create subdirectory: %v", err)
	}

	stateDir, err := repositoryStateDirFrom(subdirectory)
	if err != nil {
		t.Fatalf("repositoryStateDirFrom: %v", err)
	}
	if want := filepath.Join(root, ".spl"); stateDir != want {
		t.Fatalf("state directory = %q, want %q", stateDir, want)
	}
}
