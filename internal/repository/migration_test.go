package repository

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/pelletier/go-toml/v2"
)

func createFormatV1Repository(t *testing.T) (string, ObjectID) {
	t.Helper()
	stateDir := t.TempDir()

	// 1. Initialize a repository
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	initialHead := repo.branches["main"]
	initialSnapshot := repo.commits[initialHead].Snapshot
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
	v1EnvData, err := cbor.Marshal(looseObjectEnvelope{Type: "commit", Data: v1CommitData})
	if err != nil {
		t.Fatalf("marshal v1 env: %v", err)
	}
	v1SeedID := objectIDForEncoded("commit", v1CommitData)
	v1SeedPath := filepath.Join(stateDir, "objects", "loose", string(v1SeedID[:2]), string(v1SeedID[2:]))
	_ = os.MkdirAll(filepath.Dir(v1SeedPath), 0o700)
	if err := os.WriteFile(v1SeedPath, v1EnvData, 0o600); err != nil {
		t.Fatalf("write v1 seed commit: %v", err)
	}
	// Delete format 2 seed commit
	_ = os.Remove(filepath.Join(stateDir, "objects", "loose", string(initialHead[:2]), string(initialHead[2:])))

	// Update branch ref to v1SeedID
	refPath := filepath.Join(stateDir, "refs", "heads", "main")
	if err := os.WriteFile(refPath, []byte(string(v1SeedID)+"\n"), 0o600); err != nil {
		t.Fatalf("write ref: %v", err)
	}
	// Update reflog to v1SeedID
	logPath := filepath.Join(stateDir, "logs", "refs", "heads", "main")
	if err := os.WriteFile(logPath, []byte(" "+string(v1SeedID)+" initialize\n"), 0o600); err != nil {
		t.Fatalf("write reflog: %v", err)
	}

	return stateDir, v1SeedID
}

func TestOpenRepositoryInstructsUserToMigrateFormatV1(t *testing.T) {
	stateDir, _ := createFormatV1Repository(t)

	_, err := OpenRepository(stateDir)
	if err == nil {
		t.Fatal("OpenRepository unexpectedly succeeded on format 1 state")
	}

	var migErr *WorkspaceMigrationRequiredError
	if !errors.As(err, &migErr) {
		t.Fatalf("expected WorkspaceMigrationRequiredError, got: %v", err)
	}
	if migErr.FromVersion != 1 || migErr.ToVersion != 2 {
		t.Fatalf("expected from 1 to 2, got: %+v", migErr)
	}

	expectedMsg := "workspace format version 1 cannot be read by this version of Spool (format version 2); run 'spl migrate --from 1 --to 2' to upgrade your workspace"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Fatalf("error = %q, want containing %q", err.Error(), expectedMsg)
	}
}

func TestMigrateRepositoryFormatV1ToV2(t *testing.T) {
	stateDir, _ := createFormatV1Repository(t)

	// Run migration explicitly
	result, err := MigrateRepositoryFormat(stateDir, 1, 2)
	if err != nil {
		t.Fatalf("MigrateRepositoryFormat: %v", err)
	}
	if result.FromVersion != 1 || result.ToVersion != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.BackupPath == "" {
		t.Fatal("expected non-empty BackupPath")
	}
	if _, err := os.Stat(result.BackupPath); err != nil {
		t.Fatalf("backup path stat error: %v", err)
	}

	// Verify config is now version 2
	configPath := filepath.Join(stateDir, "config.toml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var conf map[string]any
	if err := toml.Unmarshal(configData, &conf); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if v, ok := conf["format_version"].(int64); !ok || v != 2 {
		t.Fatalf("expected format_version = 2, got %v", conf["format_version"])
	}

	// Verify repository now opens cleanly
	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository after migration: %v", err)
	}
	defer func() { _ = reopened.Close() }()

	// Verify main branch is intact
	headCommit, ok := reopened.branches["main"]
	if !ok || headCommit == "" {
		t.Fatal("expected main branch to exist")
	}

	// Running migration again should report already at version 2
	if _, err := MigrateRepositoryFormat(stateDir, 1, 2); err == nil {
		t.Fatal("expected error migrating already-migrated repository")
	}
}

func TestMigrateRepositoryFormatDetectsCycles(t *testing.T) {
	stateDir, v1SeedID := createFormatV1Repository(t)

	// Create a cyclic commit B where B has parent v1SeedID, and alter v1SeedID to have parent B
	cycleRawB := map[int]any{
		1: string(v1SeedID),
		2: []any{string(v1SeedID)},
		3: "commit B",
		4: "spl-local",
		5: 0,
	}
	cycleDataB, err := cbor.Marshal(cycleRawB)
	if err != nil {
		t.Fatalf("marshal commit B: %v", err)
	}
	envDataB, err := cbor.Marshal(looseObjectEnvelope{Type: "commit", Data: cycleDataB})
	if err != nil {
		t.Fatalf("marshal env B: %v", err)
	}
	bID := objectIDForEncoded("commit", cycleDataB)
	bPath := filepath.Join(stateDir, "objects", "loose", string(bID[:2]), string(bID[2:]))
	_ = os.MkdirAll(filepath.Dir(bPath), 0o700)
	if err := os.WriteFile(bPath, envDataB, 0o600); err != nil {
		t.Fatalf("write commit B: %v", err)
	}

	// Make v1SeedID point back to bID to form a cycle: v1SeedID -> bID -> v1SeedID
	cycleRawSeed := map[int]any{
		1: string(v1SeedID),
		2: []any{string(bID)},
		3: "seed with cycle",
		4: "spl-local",
		5: 0,
	}
	cycleDataSeed, err := cbor.Marshal(cycleRawSeed)
	if err != nil {
		t.Fatalf("marshal cyclic seed: %v", err)
	}
	envDataSeed, err := cbor.Marshal(looseObjectEnvelope{Type: "commit", Data: cycleDataSeed})
	if err != nil {
		t.Fatalf("marshal env seed: %v", err)
	}
	seedPath := filepath.Join(stateDir, "objects", "loose", string(v1SeedID[:2]), string(v1SeedID[2:]))
	if err := os.WriteFile(seedPath, envDataSeed, 0o600); err != nil {
		t.Fatalf("write cyclic seed: %v", err)
	}

	_, err = MigrateRepositoryFormat(stateDir, 1, 2)
	if err == nil {
		t.Fatal("expected error on cyclic commit graph")
	}
	if !strings.Contains(err.Error(), "cyclic commit history detected") {
		t.Fatalf("error = %q, want containing 'cyclic commit history detected'", err.Error())
	}
}

func TestMigrateRepositoryFormatRemapsRemoteBranchTracking(t *testing.T) {
	stateDir, v1SeedID := createFormatV1Repository(t)

	// Add remote branch tracking pointing to v1SeedID in config.toml
	configPath := filepath.Join(stateDir, "config.toml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var conf map[string]any
	if err := toml.Unmarshal(configData, &conf); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	conf["remote_branches"] = map[string]any{
		"main": map[string]any{
			"remote_branch":      "main",
			"remote_head_commit": string(v1SeedID),
		},
	}
	v1ConfigData, err := toml.Marshal(conf)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(configPath, v1ConfigData, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Run migration
	if _, err := MigrateRepositoryFormat(stateDir, 1, 2); err != nil {
		t.Fatalf("MigrateRepositoryFormat: %v", err)
	}

	// Verify remote tracking was remapped
	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository after migration: %v", err)
	}
	defer func() { _ = reopened.Close() }()

	tracking, ok, err := reopened.RemoteBranchTracking("main")
	if err != nil {
		t.Fatalf("RemoteBranchTracking error: %v", err)
	}
	if !ok {
		t.Fatal("expected remote tracking entry for main")
	}
	newHead := reopened.branches["main"]
	if tracking.RemoteHeadCommit != string(newHead) {
		t.Fatalf("remote head commit = %q, want %q", tracking.RemoteHeadCommit, newHead)
	}
}

func TestMigrateRepositoryFormatRejectsInvalidVersions(t *testing.T) {
	stateDir := t.TempDir()

	if _, err := MigrateRepositoryFormat(stateDir, 2, 1); err == nil {
		t.Fatal("expected error on downgrade migration 2 to 1")
	}
	if _, err := MigrateRepositoryFormat(stateDir, 1, 3); err == nil {
		t.Fatal("expected error on migration 1 to 3")
	}
}

func TestFsckRepositoryReportsUnsupportedFormatVersion(t *testing.T) {
	stateDir, _ := createFormatV1Repository(t)

	res, err := FsckRepository(stateDir)
	if err == nil {
		t.Fatal("expected FsckRepository to return error on corrupt state")
	}
	if !errors.Is(err, ErrFsckCorrupt) {
		t.Fatalf("FsckRepository error = %v, want ErrFsckCorrupt", err)
	}
	if res.Valid {
		t.Fatal("expected FsckRepository to report invalid on format 1 state")
	}

	foundDiagnostic := false
	for _, diag := range res.Diagnostics {
		if diag.Code == "unsupported-format-version" {
			foundDiagnostic = true
			if !strings.Contains(diag.Detail, "spl migrate --from 1 --to 2") {
				t.Fatalf("diagnostic detail = %q, want containing 'spl migrate --from 1 --to 2'", diag.Detail)
			}
			break
		}
	}
	if !foundDiagnostic {
		t.Fatalf("expected unsupported-format-version diagnostic, got: %+v", res.Diagnostics)
	}
}
