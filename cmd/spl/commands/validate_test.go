package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
)

func TestValidateCLIReturnsToolSchemaReport(t *testing.T) {
	repo := repository.NewSeedRepository()
	tool := resolve.NewResolveTool(repo)
	var output bytes.Buffer
	if err := runValidateCommand([]string{"--branch", "main"}, &output, func() (*resolve.ResolveTool, error) {
		return tool, nil
	}); err != nil {
		t.Fatalf("execute validate: %v", err)
	}
	var cliResult resolve.SchemaValidationResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode validate result: %v", err)
	}
	toolResult, err := tool.SPLValidateSchema(context.Background(), resolve.SchemaValidationRequest{
		Selector: resolve.SnapshotSelector{Branch: "main"},
	})
	if err != nil {
		t.Fatalf("SPLValidateSchema: %v", err)
	}
	if !reflect.DeepEqual(cliResult, toolResult) {
		t.Fatalf("CLI result %#v does not match tool result %#v", cliResult, toolResult)
	}
	if !cliResult.Valid || cliResult.Schema.Version != repository.BuiltinSchemaVersion ||
		cliResult.Snapshot.Commit == "" || cliResult.Snapshot.Root == "" || cliResult.Schema.Root == "" {
		t.Fatalf("validate result lacks expected metadata: %#v", cliResult)
	}
}

func TestValidateCLIUsesOnlyReachableExplicitCommit(t *testing.T) {
	repo := repository.NewSeedRepository()
	olderCommit, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("pin main: %v", err)
	}
	if _, err := repo.AdvanceBranch("main"); err != nil {
		t.Fatalf("advance main: %v", err)
	}
	if _, err := repo.CreateBranch("feature", repository.BranchSource{Branch: "main"}); err != nil {
		t.Fatalf("create feature: %v", err)
	}
	featureCommit, err := repo.AdvanceBranch("feature")
	if err != nil {
		t.Fatalf("advance feature: %v", err)
	}

	for _, testCase := range []struct {
		name   string
		args   []string
		want   error
		commit string
	}{
		{
			name:   "reachable ancestor",
			args:   []string{"--branch", "main", "--commit", string(olderCommit)},
			commit: string(olderCommit),
		},
		{
			name: "unreachable commit",
			args: []string{"--branch", "main", "--commit", string(featureCommit)},
			want: resolve.ErrUnsupportedCommit,
		},
		{
			name: "empty explicit commit",
			args: []string{"--branch", "main", "--commit", ""},
			want: repository.ErrCommitNotFound,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			err := runValidateCommand(testCase.args, &output, func() (*resolve.ResolveTool, error) {
				return resolve.NewResolveTool(repo), nil
			})
			if testCase.want != nil {
				if !errors.Is(err, testCase.want) {
					t.Fatalf("validate error = %v, want %v", err, testCase.want)
				}
				if output.Len() != 0 {
					t.Fatalf("validate wrote success output: %q", output.String())
				}
				return
			}
			if err != nil {
				t.Fatalf("validate reachable commit: %v", err)
			}
			var result resolve.SchemaValidationResult
			if err := json.Unmarshal(output.Bytes(), &result); err != nil {
				t.Fatalf("decode validate result: %v", err)
			}
			if result.Snapshot.Commit != testCase.commit {
				t.Fatalf("validated commit = %q, want %q", result.Snapshot.Commit, testCase.commit)
			}
		})
	}
}

func TestValidateCLIRequiresBranchAndPropagatesToolError(t *testing.T) {
	providerErr := errors.New("open validation tool")
	for _, testCase := range []struct {
		name     string
		args     []string
		provider func() (*resolve.ResolveTool, error)
		want     error
	}{
		{
			name: "missing branch",
			provider: func() (*resolve.ResolveTool, error) {
				return resolve.NewResolveTool(repository.NewSeedRepository()), nil
			},
		},
		{
			name:     "tool provider",
			args:     []string{"--branch", "main"},
			provider: func() (*resolve.ResolveTool, error) { return nil, providerErr },
			want:     providerErr,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			err := runValidateCommand(testCase.args, &output, testCase.provider)
			if testCase.want != nil {
				if !errors.Is(err, testCase.want) {
					t.Fatalf("validate error = %v, want %v", err, testCase.want)
				}
			} else if err == nil || !strings.Contains(err.Error(), `required flag(s) "branch" not set`) {
				t.Fatalf("validate error = %v, want required branch error", err)
			}
			if output.Len() != 0 {
				t.Fatalf("validate wrote success output: %q", output.String())
			}
		})
	}
}

func TestValidateCLIHelpDescribesSnapshotSelection(t *testing.T) {
	var output bytes.Buffer
	if err := runValidateCommand([]string{"--help"}, &output, func() (*resolve.ResolveTool, error) {
		return resolve.NewResolveTool(repository.NewSeedRepository()), nil
	}); err != nil {
		t.Fatalf("execute validate help: %v", err)
	}
	for _, text := range []string{
		"spl validate --branch main",
		"--commit",
		"immutable graph snapshot",
	} {
		if !strings.Contains(output.String(), text) {
			t.Errorf("validate help does not contain %q:\n%s", text, output.String())
		}
	}
}

func runValidateCommand(args []string, output *bytes.Buffer, toolProvider func() (*resolve.ResolveTool, error)) error {
	command := NewValidateCommand(toolProvider)
	command.SetOut(output)
	command.SetArgs(args)
	return command.Execute()
}
