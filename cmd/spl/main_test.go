package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/fxamacker/cbor/v2"
	"github.com/pelletier/go-toml/v2"
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
	createRepo, closeCreate, err := openPersistentRepository(stateDir)
	if err != nil {
		t.Fatalf("openPersistentRepository for create: %v", err)
	}
	var createOutput bytes.Buffer
	createCommand := newRootCommand(&createOutput, createRepo)
	createCommand.SetArgs([]string{"branch", "create", "feature", "--from-branch", "main"})
	if err := createCommand.Execute(); err != nil {
		t.Fatalf("create branch command: %v", err)
	}
	if err := closeCreate(); err != nil {
		t.Fatalf("close created repository: %v", err)
	}

	resolveRepo, closeResolve, err := openPersistentRepository(stateDir)
	if err != nil {
		t.Fatalf("openPersistentRepository for resolve: %v", err)
	}

	t.Cleanup(func() {
		if err := closeResolve(); err != nil {
			t.Errorf("Close resolved repository: %v", err)
		}
	})
	var resolveOutput bytes.Buffer
	resolveCommand := newRootCommand(&resolveOutput, resolveRepo)
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
	repoProvider := func() (*repository.Repository, error) {
		repo, err := repository.OpenRepository(stateDir)
		if err != nil {
			return nil, err
		}
		closeRepository = repo.Close
		return repo, nil
	}
	toolProvider := func() (*resolve.ResolveTool, error) {
		repo, err := repoProvider()
		if err != nil {
			return nil, err
		}
		return resolve.NewResolveTool(repo), nil
	}
	initProvider := func() (*repository.Repository, error) {
		repo, err := repository.InitializeRepository(stateDir)
		if err == nil {
			closeRepository = repo.Close
		}
		return repo, err
	}
	command := newRootCommandWithLifecycle(&output, repoProvider, toolProvider, initProvider)
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
	repoProvider := func() (*repository.Repository, error) {
		t.Fatal("init command requested repository provider")
		return nil, nil
	}
	toolProvider := func() (*resolve.ResolveTool, error) {
		t.Fatal("init command requested a persistent tool")
		return nil, nil
	}
	initProvider := func() (*repository.Repository, error) {
		return repository.InitializeRepository(stateDir)
	}
	command := newRootCommandWithLifecycle(&output, repoProvider, toolProvider, initProvider)
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

func TestMigrateCommandFlowsEndToEnd(t *testing.T) {
	stateDir := t.TempDir()

	// 1. Initialize a repository
	repo, err := repository.InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	initialHead, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}
	record, err := repo.PinnedSnapshotRecord(initialHead)
	if err != nil {
		t.Fatalf("PinnedSnapshotRecord: %v", err)
	}
	initialSnapshot := record.Snapshot
	if err := repo.Close(); err != nil {
		t.Fatalf("Close repository: %v", err)
	}

	// 2. Synthesize format_version 1 state
	configPath := filepath.Join(stateDir, "config.toml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var conf map[string]any
	if err := toml.Unmarshal(configData, &conf); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	conf["format_version"] = 1
	v1ConfigData, err := toml.Marshal(conf)
	if err != nil {
		t.Fatalf("marshal v1 config: %v", err)
	}
	if err := os.WriteFile(configPath, v1ConfigData, 0o600); err != nil {
		t.Fatalf("write v1 config: %v", err)
	}

	// Synthesize format 1 seed commit (Parents: nil, CBOR 0x02 0xf6)
	rawMap := map[int]any{
		1: string(initialSnapshot),
		2: nil,
		3: "seed resolve snapshot",
		4: "spl-local",
		5: 0,
	}
	v1CommitData, err := cbor.Marshal(rawMap)
	if err != nil {
		t.Fatalf("marshal v1 raw commit: %v", err)
	}
	v1EnvData, err := cbor.Marshal(struct {
		Type string `cbor:"1,keyasint"`
		Data []byte `cbor:"2,keyasint"`
	}{Type: "commit", Data: v1CommitData})
	if err != nil {
		t.Fatalf("marshal v1 env: %v", err)
	}
	v1SeedID := repository.ObjectIDForEncoded("commit", v1CommitData)
	v1SeedPath := filepath.Join(stateDir, "objects", "loose", string(v1SeedID[:2]), string(v1SeedID[2:]))
	_ = os.MkdirAll(filepath.Dir(v1SeedPath), 0o700)
	if err := os.WriteFile(v1SeedPath, v1EnvData, 0o600); err != nil {
		t.Fatalf("write v1 seed commit: %v", err)
	}
	_ = os.Remove(filepath.Join(stateDir, "objects", "loose", string(initialHead[:2]), string(initialHead[2:])))
	refPath := filepath.Join(stateDir, "refs", "heads", "main")
	if err := os.WriteFile(refPath, []byte(string(v1SeedID)+"\n"), 0o600); err != nil {
		t.Fatalf("write ref: %v", err)
	}
	logPath := filepath.Join(stateDir, "logs", "refs", "heads", "main")
	if err := os.WriteFile(logPath, []byte(" "+string(v1SeedID)+" initialize\n"), 0o600); err != nil {
		t.Fatalf("write reflog: %v", err)
	}

	// 3. Verify status fails with migration instruction
	var statusOut bytes.Buffer
	statusCmd, closeStatus := bootstrapRootCommand(&statusOut, stateDir)
	statusCmd.SetArgs([]string{"status", "--branch", "main"})
	err = statusCmd.Execute()
	_ = closeStatus()
	if err == nil {
		t.Fatal("expected status to fail on format 1 state")
	}
	expectedMsg := "workspace format version 1 cannot be read by this version of Spool (format version 2); run 'spl migrate --from 1 --to 2' to upgrade your workspace"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Fatalf("error = %q, want containing %q", err.Error(), expectedMsg)
	}

	// 4. Run spl migrate --from 1 --to 2
	var migrateOut bytes.Buffer
	migrateCmd, closeMigrate := bootstrapRootCommand(&migrateOut, stateDir)
	migrateCmd.SetArgs([]string{"migrate", "--from", "1", "--to", "2"})
	if err := migrateCmd.Execute(); err != nil {
		t.Fatalf("migrate command failed: %v", err)
	}
	_ = closeMigrate()

	var migResult repository.MigrationResult
	if err := json.Unmarshal(migrateOut.Bytes(), &migResult); err != nil {
		t.Fatalf("decode migration output: %v", err)
	}
	if migResult.FromVersion != 1 || migResult.ToVersion != 2 {
		t.Fatalf("unexpected migration result: %+v", migResult)
	}
	if _, err := os.Stat(migResult.BackupPath); err != nil {
		t.Fatalf("backup directory not found: %v", err)
	}

	// 5. Verify status now succeeds cleanly
	statusOut.Reset()
	statusCmd2, closeStatus2 := bootstrapRootCommand(&statusOut, stateDir)
	statusCmd2.SetArgs([]string{"status", "--branch", "main"})
	if err := statusCmd2.Execute(); err != nil {
		t.Fatalf("status command after migration failed: %v", err)
	}
	_ = closeStatus2()

	// 6. Verify fsck succeeds cleanly
	var fsckOut bytes.Buffer
	fsckCmd, closeFsck := bootstrapRootCommand(&fsckOut, stateDir)
	fsckCmd.SetArgs([]string{"fsck"})
	if err := fsckCmd.Execute(); err != nil {
		t.Fatalf("fsck command after migration failed: %v", err)
	}
	_ = closeFsck()
	var fsckResult repository.FsckResult
	if err := json.Unmarshal(fsckOut.Bytes(), &fsckResult); err != nil {
		t.Fatalf("decode fsck output: %v", err)
	}
	if !fsckResult.Valid {
		t.Fatalf("fsck reported invalid repository: %+v", fsckResult.Diagnostics)
	}
}
