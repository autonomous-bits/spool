package commands

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

func TestFsckCLIWritesValidJSONReport(t *testing.T) {
	var output bytes.Buffer
	command := NewFsckCommand(func() (*resolve.FsckTool, error) {
		return resolve.NewFsckTool(repository.NewSeedRepository()), nil
	})
	command.SetOut(&output)
	if err := command.Execute(); err != nil {
		t.Fatalf("execute fsck: %v", err)
	}
	var result repository.FsckResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode fsck result: %v", err)
	}
	if !result.Valid || len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want valid report", result)
	}
}

func TestFsckCLIWritesCorruptReportAndReturnsError(t *testing.T) {
	var output bytes.Buffer
	stateDir := t.TempDir()
	repo, err := repository.InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	head, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "objects", "loose", string(head[:2]), string(head[2:])), []byte("bad"), 0o600); err != nil {
		t.Fatalf("corrupt fixture: %v", err)
	}
	command := NewFsckCommand(func() (*resolve.FsckTool, error) {
		return resolve.NewPersistentFsckTool(stateDir), nil
	})
	command.SetOut(&output)
	err = command.Execute()
	if !errors.Is(err, repository.ErrFsckCorrupt) {
		t.Fatalf("execute fsck error = %v, want ErrFsckCorrupt", err)
	}
	var result repository.FsckResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode fsck result: %v", err)
	}
	if result.Valid || len(result.Diagnostics) == 0 {
		t.Fatalf("result = %#v, want corrupt report", result)
	}
}

func TestFsckCLISurfacesPackedStorage(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := repository.InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	if _, err := repo.GC(repository.GCOptions{}); err != nil {
		t.Fatalf("GC: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	var output bytes.Buffer
	command := NewFsckCommand(func() (*resolve.FsckTool, error) {
		return resolve.NewPersistentFsckTool(stateDir), nil
	})
	command.SetOut(&output)
	if err := command.Execute(); err != nil {
		t.Fatalf("execute fsck: %v", err)
	}
	var result repository.FsckResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode fsck result: %v", err)
	}
	if !result.Valid || len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want valid packed report", result)
	}
}
