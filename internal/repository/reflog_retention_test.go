package repository

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository/branch"
)

func TestReflogRetentionInventoryTracksDeletedBranchAndFailsClosed(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	closeTestRepository(t, repo)
	if _, err := repo.CreateBranch("feature", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if _, err := repo.AdvanceBranch("feature"); err != nil {
		t.Fatalf("AdvanceBranch: %v", err)
	}
	if _, err := repo.DeleteBranch("feature"); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	inventory, err := readReflogRetentionInventory(filepath.Join(stateDir, reflogRetentionInventoryFilename))
	if err != nil {
		t.Fatalf("read reflog retention inventory: %v", err)
	}
	if !sameReflogRetentionPaths(inventory, []string{"HEAD", "refs/heads/feature", "refs/heads/main"}) {
		t.Fatalf("inventory = %#v", inventory)
	}

	deleted := filepath.Join(stateDir, "logs", "refs", "heads", "feature")
	if err := os.Remove(deleted); err != nil {
		t.Fatalf("remove tracked historical reflog: %v", err)
	}
	if _, err := repo.ScanRetention(); !errors.Is(err, ErrGCCorrupt) {
		t.Fatalf("ScanRetention error = %v, want ErrGCCorrupt", err)
	}
	if _, err := repo.GC(GCOptions{DryRun: true}); !errors.Is(err, ErrGCCorrupt) {
		t.Fatalf("GC error = %v, want ErrGCCorrupt", err)
	}
	result, err := FsckRepository(stateDir)
	if !errors.Is(err, ErrFsckCorrupt) {
		t.Fatalf("FsckRepository error = %v, want ErrFsckCorrupt", err)
	}
	if !hasFsckDiagnosticAtPath(result, "missing-listed-reflog", "logs/refs/heads/feature") {
		t.Fatalf("fsck result = %#v, want missing listed reflog", result)
	}
}

func TestReflogRetentionInventoryFailureDoesNotAdvanceRefAndReopens(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	before, err := os.ReadFile(filepath.Join(stateDir, reflogRetentionInventoryFilename))
	if err != nil {
		t.Fatalf("read inventory: %v", err)
	}
	repo.writeReflogRetentionInventoryFn = func([]string) error {
		return errors.New("injected inventory write failure")
	}
	if _, err := repo.CreateBranch("feature", branch.Source{Branch: "main"}); err == nil {
		t.Fatal("CreateBranch succeeded despite inventory write failure")
	}
	if _, exists := repo.branches["feature"]; exists {
		t.Fatal("inventory write failure advanced the in-memory branch")
	}
	if _, err := os.Stat(filepath.Join(stateDir, "refs", "heads", "feature")); !os.IsNotExist(err) {
		t.Fatalf("inventory write failure created branch ref: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "logs", "refs", "heads", "feature")); !os.IsNotExist(err) {
		t.Fatalf("inventory write failure left a prepared reflog: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(stateDir, reflogRetentionInventoryFilename))
	if err != nil {
		t.Fatalf("read inventory after failed update: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("inventory changed after failed update: got %q, want %q", after, before)
	}
	repo.writeReflogRetentionInventoryFn = nil
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository after failed inventory update: %v", err)
	}
	closeTestRepository(t, reopened)
	if _, err := reopened.PinBranch("feature"); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("reopened feature = %v, want ErrBranchNotFound", err)
	}
}

func TestReflogRetentionInventoryRecoversRefReplacementFailure(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	err = repo.replaceThenReflogLocked(
		func() error { return errors.New("injected ref replacement failure") },
		"refs/heads/feature", "", repo.branches["main"], "create",
	)
	if err == nil || durableWriteCommitted(err) {
		t.Fatalf("replaceThenReflogLocked error = %v, want uncommitted replacement failure", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "refs", "heads", "feature")); !os.IsNotExist(err) {
		t.Fatalf("failed replacement created ref: %v", err)
	}
	logPath := filepath.Join(stateDir, "logs", "refs", "heads", "feature")
	if data, err := os.ReadFile(logPath); err != nil || len(data) != 0 {
		t.Fatalf("prepared reflog = %q, %v; want valid empty reflog", data, err)
	}
	paths, err := readReflogRetentionInventory(filepath.Join(stateDir, reflogRetentionInventoryFilename))
	if err != nil {
		t.Fatalf("read reflog retention inventory: %v", err)
	}
	if !sameReflogRetentionPaths(paths, []string{"HEAD", "refs/heads/feature", "refs/heads/main"}) {
		t.Fatalf("inventory = %#v", paths)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository after failed ref replacement: %v", err)
	}
	closeTestRepository(t, reopened)
	if _, err := reopened.PinBranch("feature"); !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("reopened feature = %v, want ErrBranchNotFound", err)
	}
	if _, err := reopened.ScanRetention(); err != nil {
		t.Fatalf("ScanRetention after failed ref replacement: %v", err)
	}
	if _, err := reopened.GC(GCOptions{DryRun: true}); err != nil {
		t.Fatalf("GC after failed ref replacement: %v", err)
	}
	if result, err := FsckRepository(stateDir); err != nil || !result.Valid {
		t.Fatalf("FsckRepository after failed ref replacement = %#v, %v", result, err)
	}
}

func TestReflogRetentionInventorySyncWarningCommitsAndReopens(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	repo.writeReflogRetentionInventoryFn = func(paths []string) error {
		data, err := encodeReflogRetentionInventory(paths)
		if err != nil {
			return err
		}
		if err := writeDurableStateFile(filepath.Join(stateDir, reflogRetentionInventoryFilename), data); err != nil {
			return err
		}
		return durableWriteCommittedError{err: errors.New("injected inventory directory sync warning")}
	}
	result, err := repo.CreateBranch("feature", branch.Source{Branch: "main"})
	if err == nil || !durableWriteCommitted(err) || result.Name != "feature" {
		t.Fatalf("CreateBranch = %#v, %v; want committed durability warning", result, err)
	}
	lines, err := readReflogLines(filepath.Join(stateDir, "logs", "refs", "heads", "feature"))
	if err != nil || len(lines) != 1 || lines[0] != [3]string{"", string(repo.branches["main"]), "create"} {
		t.Fatalf("feature reflog = %#v, %v; want one create entry", lines, err)
	}
	repo.writeReflogRetentionInventoryFn = nil
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository after inventory warning: %v", err)
	}
	closeTestRepository(t, reopened)
	if _, err := reopened.PinBranch("feature"); err != nil {
		t.Fatalf("reopened feature: %v", err)
	}
	if _, err := reopened.ScanRetention(); err != nil {
		t.Fatalf("ScanRetention after inventory warning: %v", err)
	}
}

func TestReflogReplacementFailurePreservesPreviousLogAndReopens(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	path := filepath.Join(stateDir, "logs", "refs", "heads", "main")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read main reflog before failure: %v", err)
	}
	previous, err := readReflogLines(path)
	if err != nil {
		t.Fatalf("read main reflog before failure: %v", err)
	}
	repo.writeReflogFn = func(gotPath string, data []byte) error {
		if gotPath != path {
			t.Fatalf("reflog replacement path = %q, want %q", gotPath, path)
		}
		lines, err := parseReflogLines(data)
		if err != nil {
			t.Fatalf("replacement data is not parseable: %v", err)
		}
		if len(lines) != len(previous)+1 {
			t.Fatalf("replacement entries = %#v, want one appended entry", lines)
		}
		return errors.New("injected reflog replacement failure")
	}

	advanced, err := repo.AdvanceBranch("main")
	if advanced == "" || err == nil || !durableWriteCommitted(err) {
		t.Fatalf("AdvanceBranch = %q, %v; want committed durability warning", advanced, err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read main reflog after failure: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("reflog changed after failed replacement: got %q, want %q", after, before)
	}
	if _, err := readReflogLines(path); err != nil {
		t.Fatalf("previous reflog is no longer parseable: %v", err)
	}
	repo.writeReflogFn = nil
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository after failed reflog replacement: %v", err)
	}
	closeTestRepository(t, reopened)
	if got := reopened.branches["main"]; got != advanced {
		t.Fatalf("reopened main head = %q, want %q", got, advanced)
	}
}

func TestReflogReplacementRetainsExistingAndNewRoots(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	closeTestRepository(t, repo)
	initial := repo.branches["main"]

	advanced, err := repo.AdvanceBranch("main")
	if err != nil {
		t.Fatalf("AdvanceBranch: %v", err)
	}
	lines, err := readReflogLines(filepath.Join(stateDir, "logs", "refs", "heads", "main"))
	if err != nil {
		t.Fatalf("read main reflog: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("main reflog entries = %#v, want initialization and advance", lines)
	}
	if lines[0] != [3]string{"", string(initial), "initialize"} {
		t.Fatalf("initial reflog entry = %#v", lines[0])
	}
	if lines[1] != [3]string{string(initial), string(advanced), "advance"} {
		t.Fatalf("advance reflog entry = %#v", lines[1])
	}
}

func TestReflogReplacementRejectsMalformedExistingLog(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	path := filepath.Join(stateDir, "logs", "refs", "heads", "main")
	malformed := []byte("not a complete reflog entry")
	if err := os.WriteFile(path, malformed, 0o600); err != nil {
		t.Fatalf("write malformed reflog: %v", err)
	}
	before := repo.branches["main"]

	if _, err := repo.AdvanceBranch("main"); err == nil || durableWriteCommitted(err) {
		t.Fatalf("AdvanceBranch error = %v, want uncommitted malformed reflog failure", err)
	}
	if got := repo.branches["main"]; got != before {
		t.Fatalf("main head = %q, want unchanged %q", got, before)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read malformed reflog: %v", err)
	}
	if string(after) != string(malformed) {
		t.Fatalf("malformed reflog was changed: got %q, want %q", after, malformed)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := OpenRepository(stateDir); err == nil {
		t.Fatal("OpenRepository accepted malformed reflog")
	}
}

func TestOpenRepositoryBootstrapsLegacyReflogInventory(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	configPath := filepath.Join(stateDir, "config.toml")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	legacyConfig := strings.Replace(string(config), "reflog_retention_inventory = true\n", "", 1)
	if err := os.WriteFile(configPath, []byte(legacyConfig), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	if err := os.Remove(filepath.Join(stateDir, reflogRetentionInventoryFilename)); err != nil {
		t.Fatalf("remove inventory: %v", err)
	}

	reopened, err := OpenRepository(stateDir)
	if err != nil {
		t.Fatalf("OpenRepository legacy reflogs: %v", err)
	}
	closeTestRepository(t, reopened)
	paths, err := readReflogRetentionInventory(filepath.Join(stateDir, reflogRetentionInventoryFilename))
	if err != nil {
		t.Fatalf("read bootstrapped inventory: %v", err)
	}
	if !sameReflogRetentionPaths(paths, []string{"HEAD", "refs/heads/main"}) {
		t.Fatalf("bootstrapped paths = %#v", paths)
	}
	config, err = os.ReadFile(configPath)
	if err != nil || !strings.Contains(string(config), "reflog_retention_inventory = true") {
		t.Fatalf("migrated config = %q, %v", config, err)
	}
}

func TestOpenRepositoryRejectsMissingExpectedReflogInventory(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := os.Remove(filepath.Join(stateDir, reflogRetentionInventoryFilename)); err != nil {
		t.Fatalf("remove inventory: %v", err)
	}
	if _, err := OpenRepository(stateDir); !errors.Is(err, errReflogRetentionInventoryMissing) {
		t.Fatalf("OpenRepository missing expected inventory error = %v, want inventory missing", err)
	}
}

func TestReflogRetentionInventoryRejectsMissingMalformedAndUnexpectedLogs(t *testing.T) {
	for _, test := range []struct {
		name     string
		mutate   func(t *testing.T, stateDir string)
		fsckCode string
		scanErr  error
	}{
		{
			name: "missing inventory",
			mutate: func(t *testing.T, stateDir string) {
				t.Helper()
				if err := os.Remove(filepath.Join(stateDir, reflogRetentionInventoryFilename)); err != nil {
					t.Fatalf("remove inventory: %v", err)
				}
			},
			fsckCode: "missing-reflog-retention-inventory",
			scanErr:  ErrGCCorrupt,
		},
		{
			name: "malformed inventory",
			mutate: func(t *testing.T, stateDir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(stateDir, reflogRetentionInventoryFilename), []byte("not an inventory\n"), 0o600); err != nil {
					t.Fatalf("write malformed inventory: %v", err)
				}
			},
			fsckCode: "invalid-reflog-retention-inventory",
			scanErr:  ErrGCCorrupt,
		},
		{
			name: "unexpected reflog",
			mutate: func(t *testing.T, stateDir string) {
				t.Helper()
				mainLog := filepath.Join(stateDir, "logs", "refs", "heads", "main")
				data, err := os.ReadFile(mainLog)
				if err != nil {
					t.Fatalf("read main reflog: %v", err)
				}
				path := filepath.Join(stateDir, "logs", "refs", "heads", "untracked")
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatalf("write unexpected reflog: %v", err)
				}
			},
			fsckCode: "unexpected-reflog",
			scanErr:  ErrGCCorrupt,
		},
		{
			name: "out of scope reflog",
			mutate: func(t *testing.T, stateDir string) {
				t.Helper()
				path := filepath.Join(stateDir, "logs", "outside")
				if err := os.WriteFile(path, []byte("anything\n"), 0o600); err != nil {
					t.Fatalf("write out-of-scope reflog: %v", err)
				}
			},
			fsckCode: "out-of-scope-reflog",
			scanErr:  ErrGCCorrupt,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			stateDir := t.TempDir()
			repo, err := InitializeRepository(stateDir)
			if err != nil {
				t.Fatalf("InitializeRepository: %v", err)
			}
			closeTestRepository(t, repo)
			test.mutate(t, stateDir)
			if _, err := repo.ScanRetention(); !errors.Is(err, test.scanErr) {
				t.Fatalf("ScanRetention error = %v, want %v", err, test.scanErr)
			}
			result, err := FsckRepository(stateDir)
			if !errors.Is(err, ErrFsckCorrupt) || !hasFsckDiagnostic(result, test.fsckCode) {
				t.Fatalf("FsckRepository = %#v, %v; want %s", result, err, test.fsckCode)
			}
		})
	}
}
