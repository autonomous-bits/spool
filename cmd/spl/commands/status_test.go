package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

func TestStatusCLIAndMCPReportEquivalentSharedStagingDelta(t *testing.T) {
	operations := []repository.MutationOperation{
		{Action: "add", Entity: "node", ID: "node-2", Title: "Second node"},
	}
	cliRepo := repository.NewSeedRepository()
	if _, err := cliRepo.StageMutationBatch(repository.StageMutationRequest{Branch: "main", Operations: operations}); err != nil {
		t.Fatalf("stage CLI repository batch: %v", err)
	}
	var output bytes.Buffer
	command := NewStatusCommand(func() (*resolve.ResolveTool, error) {
		return resolve.NewResolveTool(cliRepo), nil
	})
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute status command: %v", err)
	}
	var cliResult repository.BranchStagingStatus
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}

	mcpRepo := repository.NewSeedRepository()
	if _, err := mcpRepo.StageMutationBatch(repository.StageMutationRequest{Branch: "main", Operations: operations}); err != nil {
		t.Fatalf("stage MCP repository batch: %v", err)
	}
	mcpResult, err := resolve.NewResolveTool(mcpRepo).EDGBranchStagingStatus(context.Background(), "main")
	if err != nil {
		t.Fatalf("EDGBranchStagingStatus: %v", err)
	}
	if !reflect.DeepEqual(cliResult, mcpResult) {
		t.Fatalf("CLI result %#v does not match MCP result %#v", cliResult, mcpResult)
	}
}

func TestStatusCLIReportsEmptyDeltaAndRejectsMissingBranch(t *testing.T) {
	repo := repository.NewSeedRepository()
	newCommand := func() (*bytes.Buffer, *cobra.Command) {
		var output bytes.Buffer
		command := NewStatusCommand(func() (*resolve.ResolveTool, error) {
			return resolve.NewResolveTool(repo), nil
		})
		command.SetOut(&output)
		return &output, command
	}

	output, command := newCommand()
	command.SetArgs([]string{"--branch", "main"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute empty status command: %v", err)
	}
	var status repository.BranchStagingStatus
	if err := json.Unmarshal(output.Bytes(), &status); err != nil {
		t.Fatalf("decode empty status: %v", err)
	}
	if status != (repository.BranchStagingStatus{Branch: "main"}) {
		t.Fatalf("status = %#v", status)
	}

	_, command = newCommand()
	command.SetArgs([]string{"--branch", "missing"})
	if err := command.Execute(); !errors.Is(err, repository.ErrBranchNotFound) {
		t.Fatalf("status command error = %v, want ErrBranchNotFound", err)
	}
}
