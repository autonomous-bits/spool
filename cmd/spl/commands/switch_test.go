package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/repository/branch"
	"github.com/autonomous-bits/spool/internal/resolve"
)

func TestSwitchCLISelectsExistingInactiveBranch(t *testing.T) {
	repo := repository.NewSeedRepository()
	if _, err := repo.CreateBranch("feature", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	var output bytes.Buffer

	command := NewSwitchCommand(func() (*resolve.ResolveTool, error) {
		return resolve.NewResolveTool(repo), nil
	})
	command.SetOut(&output)
	command.SetArgs([]string{"feature"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute switch command: %v", err)
	}

	var result branch.SwitchResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	if result != (branch.SwitchResult{ActiveBranch: "feature"}) {
		t.Fatalf("result = %#v", result)
	}
	initialization, err := repo.Initialization()
	if err != nil {
		t.Fatalf("Initialization: %v", err)
	}
	if initialization.ActiveBranch != "feature" {
		t.Fatalf("active branch = %q, want feature", initialization.ActiveBranch)
	}
}

func TestSwitchCLIRejectsMissingBranchWithoutWritingSuccessOutput(t *testing.T) {
	repo := repository.NewSeedRepository()
	var output bytes.Buffer

	command := NewSwitchCommand(func() (*resolve.ResolveTool, error) {
		return resolve.NewResolveTool(repo), nil
	})
	command.SetOut(&output)
	command.SetArgs([]string{"missing"})
	if err := command.Execute(); !errors.Is(err, branch.ErrNotFound) {
		t.Fatalf("execute switch command error = %v, want ErrNotFound", err)
	}
	if output.Len() != 0 {
		t.Fatalf("switch command wrote success output: %q", output.String())
	}
	initialization, err := repo.Initialization()
	if err != nil {
		t.Fatalf("Initialization: %v", err)
	}
	if initialization.ActiveBranch != "main" {
		t.Fatalf("active branch = %q, want main", initialization.ActiveBranch)
	}
}

func TestSwitchCLISucceedsWhenBranchIsAlreadyActive(t *testing.T) {
	repo := repository.NewSeedRepository()
	var output bytes.Buffer

	command := NewSwitchCommand(func() (*resolve.ResolveTool, error) {
		return resolve.NewResolveTool(repo), nil
	})
	command.SetOut(&output)
	command.SetArgs([]string{"main"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute switch command: %v", err)
	}

	var result branch.SwitchResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	if result != (branch.SwitchResult{ActiveBranch: "main"}) {
		t.Fatalf("result = %#v", result)
	}
	initialization, err := repo.Initialization()
	if err != nil {
		t.Fatalf("Initialization: %v", err)
	}
	if initialization.ActiveBranch != "main" {
		t.Fatalf("active branch = %q, want main", initialization.ActiveBranch)
	}
}
