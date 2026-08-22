package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/repository/branch"
	"github.com/autonomous-bits/spool/internal/resolve"
)

func TestCreateBranchCLIAndMCPReturnEquivalentPayloads(t *testing.T) {
	cliTool := resolve.NewResolveTool(repository.NewSeedRepository())
	var output bytes.Buffer
	if err := runBranchCommand([]string{"create", "from-branch", "--from-branch", "main"}, &output, cliTool); err != nil {
		t.Fatalf("run branch CLI: %v", err)
	}
	var cliResult branch.CreateResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}

	mcpTool := resolve.NewResolveTool(repository.NewSeedRepository())
	mcpResult, err := mcpTool.SPLCreateBranch(context.Background(), branch.CreateRequest{
		Name:   "from-branch",
		Source: branch.Source{Branch: "main"},
	})
	if err != nil {
		t.Fatalf("SPLCreateBranch: %v", err)
	}
	if !reflect.DeepEqual(cliResult, mcpResult) {
		t.Fatalf("CLI result %#v does not match MCP result %#v", cliResult, mcpResult)
	}
}

func TestCreateBranchCLIAndMCPSupportCommitSource(t *testing.T) {
	cliTool := resolve.NewResolveTool(repository.NewSeedRepository())
	source, err := cliTool.SPLResolve(context.Background(), resolve.ResolveRequest{
		Selector: resolve.SnapshotSelector{Branch: "main"},
		NodeID:   repository.SeedNodeID,
	})
	if err != nil {
		t.Fatalf("resolve source commit: %v", err)
	}
	var output bytes.Buffer
	if err := runBranchCommand([]string{"create", "from-commit", "--from-commit", source.Snapshot.Commit}, &output, cliTool); err != nil {
		t.Fatalf("run branch CLI: %v", err)
	}
	var cliResult branch.CreateResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}

	mcpTool := resolve.NewResolveTool(repository.NewSeedRepository())
	mcpResult, err := mcpTool.SPLCreateBranch(context.Background(), branch.CreateRequest{
		Name:   "from-commit",
		Source: branch.Source{Commit: source.Snapshot.Commit},
	})
	if err != nil {
		t.Fatalf("SPLCreateBranch: %v", err)
	}
	if !reflect.DeepEqual(cliResult, mcpResult) {
		t.Fatalf("CLI result %#v does not match MCP result %#v", cliResult, mcpResult)
	}
}

func TestCreateBranchCLIRejectsInvalidOrUnresolvedSources(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		want error
	}{
		{name: "neither source", args: []string{"create", "feature"}, want: branch.ErrMissingSource},
		{name: "both sources", args: []string{"create", "feature", "--from-branch", "main", "--from-commit", "missing"}, want: branch.ErrAmbiguousSource},
		{name: "missing source", args: []string{"create", "feature", "--from-branch", "missing"}, want: branch.ErrSourceNotFound},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			err := runBranchCommand(testCase.args, &output, resolve.NewResolveTool(repository.NewSeedRepository()))
			if !errors.Is(err, testCase.want) {
				t.Fatalf("run branch CLI error = %v, want %v", err, testCase.want)
			}
			if output.Len() != 0 {
				t.Fatalf("CLI wrote success output: %q", output.String())
			}
		})
	}
}

func TestCreateBranchCLIRejectsMissingSourceBeforeDuplicateName(t *testing.T) {
	var output bytes.Buffer
	err := runBranchCommand(
		[]string{"create", "main", "--from-branch", "missing"},
		&output,
		resolve.NewResolveTool(repository.NewSeedRepository()),
	)
	if !errors.Is(err, branch.ErrSourceNotFound) {
		t.Fatalf("run branch CLI error = %v, want ErrSourceNotFound", err)
	}
}

func TestCreateBranchCLIRejectsDuplicateName(t *testing.T) {
	var output bytes.Buffer
	err := runBranchCommand(
		[]string{"create", "main", "--from-branch", "main"},
		&output,
		resolve.NewResolveTool(repository.NewSeedRepository()),
	)
	if !errors.Is(err, branch.ErrAlreadyExists) {
		t.Fatalf("run branch CLI error = %v, want ErrAlreadyExists", err)
	}

}

func TestListBranchesCLIAndMCPReturnEquivalentPayloads(t *testing.T) {
	cliRepo := repository.NewSeedRepository()
	if _, err := cliRepo.CreateBranch("zebra", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch zebra: %v", err)
	}

	if _, err := cliRepo.CreateBranch("alpha", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch alpha: %v", err)
	}
	var output bytes.Buffer
	if err := runBranchCommand([]string{"list"}, &output, resolve.NewResolveTool(cliRepo)); err != nil {
		t.Fatalf("run branch CLI: %v", err)
	}
	var cliResult branch.ListResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}

	mcpRepo := repository.NewSeedRepository()
	if _, err := mcpRepo.CreateBranch("zebra", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch zebra: %v", err)
	}
	if _, err := mcpRepo.CreateBranch("alpha", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch alpha: %v", err)
	}
	mcpResult, err := resolve.NewResolveTool(mcpRepo).SPLListBranches(context.Background())
	if err != nil {
		t.Fatalf("SPLListBranches: %v", err)
	}
	if !reflect.DeepEqual(cliResult, mcpResult) {
		t.Fatalf("CLI result %#v does not match MCP result %#v", cliResult, mcpResult)
	}
}

func TestDeleteBranchCLIAndMCPReturnEquivalentPayloads(t *testing.T) {
	cliRepo := repository.NewSeedRepository()
	if _, err := cliRepo.CreateBranch("feature", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	var output bytes.Buffer
	if err := runBranchCommand([]string{"delete", "feature"}, &output, resolve.NewResolveTool(cliRepo)); err != nil {
		t.Fatalf("run branch CLI: %v", err)
	}
	var cliResult branch.DeleteResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}

	mcpRepo := repository.NewSeedRepository()
	if _, err := mcpRepo.CreateBranch("feature", branch.Source{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	mcpResult, err := resolve.NewResolveTool(mcpRepo).SPLDeleteBranch(
		context.Background(), branch.DeleteRequest{Name: "feature"},
	)
	if err != nil {
		t.Fatalf("SPLDeleteBranch: %v", err)
	}
	if !reflect.DeepEqual(cliResult, mcpResult) {
		t.Fatalf("CLI result %#v does not match MCP result %#v", cliResult, mcpResult)
	}
}

func TestDeleteBranchCLIRejectsDefaultAndMissingBranches(t *testing.T) {
	for _, testCase := range []struct {
		name string
		want error
	}{
		{name: "main", want: branch.ErrDefaultProtected},
		{name: "missing", want: branch.ErrNotFound},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			err := runBranchCommand(
				[]string{"delete", testCase.name}, &output, resolve.NewResolveTool(repository.NewSeedRepository()),
			)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("run branch CLI error = %v, want %v", err, testCase.want)
			}

			if output.Len() != 0 {
				t.Fatalf("CLI wrote success output: %q", output.String())
			}
		})
	}
}

func runBranchCommand(args []string, output *bytes.Buffer, tool *resolve.ResolveTool) error {
	command := NewBranchCommand(tool)
	command.SetOut(output)
	command.SetArgs(args)
	return command.Execute()
}
