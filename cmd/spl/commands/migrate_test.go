package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
)

func TestMigrateCLIRequiresFlags(t *testing.T) {
	command := NewMigrateCommand(func(from, to int) (*repository.MigrationResult, error) {
		return &repository.MigrationResult{FromVersion: from, ToVersion: to, BackupPath: "/tmp/backup"}, nil
	})
	command.SetOut(&bytes.Buffer{})
	command.SetArgs([]string{})
	err := command.Execute()
	if err == nil {
		t.Fatal("expected error when flags are omitted")
	}
	if !strings.Contains(err.Error(), "both --from and --to flags are required") {
		t.Fatalf("error = %q, want containing required flag notice", err.Error())
	}
}

func TestMigrateCLISuccess(t *testing.T) {
	var output bytes.Buffer
	command := NewMigrateCommand(func(from, to int) (*repository.MigrationResult, error) {
		if from != 1 || to != 2 {
			return nil, errors.New("invalid versions")
		}
		return &repository.MigrationResult{
			FromVersion: from,
			ToVersion:   to,
			BackupPath:  "/tmp/state.v1.backup-12345",
		}, nil
	})
	command.SetOut(&output)
	command.SetArgs([]string{"--from", "1", "--to", "2"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute migrate: %v", err)
	}

	var result repository.MigrationResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON output: %v", err)
	}
	if result.FromVersion != 1 || result.ToVersion != 2 || result.BackupPath != "/tmp/state.v1.backup-12345" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestMigrateCLISurfacesProviderError(t *testing.T) {
	command := NewMigrateCommand(func(from, to int) (*repository.MigrationResult, error) {
		return nil, errors.New("cannot migrate: disk full")
	})
	command.SetOut(&bytes.Buffer{})
	command.SetArgs([]string{"--from", "1", "--to", "2"})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "cannot migrate: disk full") {
		t.Fatalf("expected error containing 'cannot migrate: disk full', got %v", err)
	}
}
