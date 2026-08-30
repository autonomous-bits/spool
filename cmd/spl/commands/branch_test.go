package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
)

func TestCreateBranchCLIWithBranchSource(t *testing.T) {
	repo := newTestSeedRepository(t)
	var output bytes.Buffer
	if err := runBranchCommand([]string{"create", "from-branch", "--from-branch", "main"}, &output, repo); err != nil {
		t.Fatalf("run branch CLI: %v", err)
	}
	var cliResult repository.BranchCreateResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	if cliResult.Name != "from-branch" || cliResult.Commit == "" {
		t.Fatalf("unexpected CLI result: %#v", cliResult)
	}
}

func TestCreateBranchCLIWithCommitSource(t *testing.T) {
	repo := newTestSeedRepository(t)
	mainHead, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("pin main branch: %v", err)
	}
	var output bytes.Buffer
	if err := runBranchCommand([]string{"create", "from-commit", "--from-commit", string(mainHead)}, &output, repo); err != nil {
		t.Fatalf("run branch CLI: %v", err)
	}
	var cliResult repository.BranchCreateResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	if cliResult.Name != "from-commit" || cliResult.Commit != string(mainHead) {
		t.Fatalf("unexpected CLI result: %#v, want commit %q", cliResult, mainHead)
	}
}

func TestCreateBranchCLIRejectsInvalidOrUnresolvedSources(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		want error
	}{
		{name: "neither source", args: []string{"create", "feature"}, want: repository.ErrBranchMissingSource},
		{name: "both sources", args: []string{"create", "feature", "--from-branch", "main", "--from-commit", "missing"}, want: repository.ErrBranchAmbiguousSource},
		{name: "missing source", args: []string{"create", "feature", "--from-branch", "missing"}, want: repository.ErrBranchSourceNotFound},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			err := runBranchCommand(testCase.args, &output, newTestSeedRepository(t))
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
		newTestSeedRepository(t),
	)
	if !errors.Is(err, repository.ErrBranchSourceNotFound) {
		t.Fatalf("run branch CLI error = %v, want ErrBranchSourceNotFound", err)
	}
}

func TestCreateBranchCLIRejectsDuplicateName(t *testing.T) {
	var output bytes.Buffer
	err := runBranchCommand(
		[]string{"create", "main", "--from-branch", "main"},
		&output,
		newTestSeedRepository(t),
	)
	if !errors.Is(err, repository.ErrBranchAlreadyExists) {
		t.Fatalf("run branch CLI error = %v, want ErrBranchAlreadyExists", err)
	}
}

func TestListBranchesCLIReturnsSortedBranchNames(t *testing.T) {
	repo := newTestSeedRepository(t)
	if _, err := repo.CreateBranch("zebra", repository.BranchSource{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch zebra: %v", err)
	}
	if _, err := repo.CreateBranch("alpha", repository.BranchSource{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch alpha: %v", err)
	}
	var output bytes.Buffer
	if err := runBranchCommand([]string{"list"}, &output, repo); err != nil {
		t.Fatalf("run branch CLI: %v", err)
	}
	var cliResult repository.BranchListResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	want := []string{"alpha", "main", "zebra"}
	if len(cliResult.Branches) != len(want) || cliResult.Branches[0] != "alpha" || cliResult.Branches[1] != "main" || cliResult.Branches[2] != "zebra" {
		t.Fatalf("branches = %#v, want %#v", cliResult.Branches, want)
	}
}

func TestDeleteBranchCLIDeletesExistingBranch(t *testing.T) {
	repo := newTestSeedRepository(t)
	if _, err := repo.CreateBranch("feature", repository.BranchSource{Branch: "main"}); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	var output bytes.Buffer
	if err := runBranchCommand([]string{"delete", "feature"}, &output, repo); err != nil {
		t.Fatalf("run branch CLI: %v", err)
	}
	var cliResult repository.BranchDeleteResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	if cliResult.Name != "feature" {
		t.Fatalf("result = %#v, want name feature", cliResult)
	}
	if _, err := repo.PinBranch("feature"); !errors.Is(err, repository.ErrBranchNotFound) {
		t.Fatalf("PinBranch after delete error = %v, want ErrBranchNotFound", err)
	}
}

func TestDeleteBranchCLIRejectsDefaultAndMissingBranches(t *testing.T) {
	for _, testCase := range []struct {
		name string
		want error
	}{
		{name: "main", want: repository.ErrDefaultBranchProtected},
		{name: "missing", want: repository.ErrBranchNotFound},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			err := runBranchCommand(
				[]string{"delete", testCase.name}, &output, newTestSeedRepository(t),
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

func runBranchCommand(args []string, output *bytes.Buffer, repo *repository.Repository) error {
	command := NewBranchCommand(func() (*repository.Repository, error) {
		return repo, nil
	})
	command.SetOut(output)
	command.SetArgs(args)
	return command.Execute()
}
