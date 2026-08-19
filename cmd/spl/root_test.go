package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
)

func TestRootCommandIncludesResolveSubcommand(t *testing.T) {
	command := newRootCommand(&bytes.Buffer{}, resolve.NewResolveTool(repository.NewSeedRepository()))

	found, _, err := command.Find([]string{"resolve"})
	if err != nil {
		t.Fatalf("find resolve command: %v", err)
	}

	if found.Name() != "resolve" {
		t.Fatalf("command = %q, want resolve", found.Name())
	}
}

func TestCommandHelpIncludesExamples(t *testing.T) {
	testCases := []struct {
		path    []string
		example string
	}{
		{[]string{"init", "--help"}, "spl init"},
		{[]string{"add", "--help"}, "spl add --branch main --batch mutations.json"},
		{[]string{"status", "--help"}, "spl status --branch main"},
		{[]string{"commit", "--help"}, "spl commit --branch main"},
		{[]string{"branch", "create", "--help"}, "spl branch create feature --from-branch main"},
		{[]string{"branch", "list", "--help"}, "spl branch list"},
		{[]string{"branch", "delete", "--help"}, "spl branch delete feature"},
		{[]string{"switch", "--help"}, "spl switch feature"},
		{[]string{"resolve", "--help"}, "spl resolve --branch main --node"},
		{[]string{"schema", "migrate", "--help"}, "spl schema migrate --branch main --schema"},
		{[]string{"validate", "--help"}, "spl validate --branch main"},
		{[]string{"diff", "--help"}, "spl diff --base-branch main --target-branch feature"},
		{[]string{"history", "--help"}, "spl history --branch main --entity-id"},
		{[]string{"branches-containing", "--help"}, "spl branches-containing --entity-id"},
	}

	for _, testCase := range testCases {
		t.Run(strings.Join(testCase.path, " "), func(t *testing.T) {
			var output bytes.Buffer
			command := newRootCommand(&output, resolve.NewResolveTool(repository.NewSeedRepository()))
			command.SetArgs(testCase.path)

			if err := command.Execute(); err != nil {
				t.Fatalf("execute help: %v", err)
			}
			if !strings.Contains(output.String(), testCase.example) {
				t.Errorf("help output does not contain %q:\n%s", testCase.example, output.String())
			}
		})
	}
}

func TestRootCommandIncludesSchemaAndValidateSubcommands(t *testing.T) {
	command := newRootCommand(&bytes.Buffer{}, resolve.NewResolveTool(repository.NewSeedRepository()))
	for _, path := range [][]string{{"schema", "migrate"}, {"validate"}} {
		found, _, err := command.Find(path)
		if err != nil {
			t.Fatalf("find %v: %v", path, err)
		}
		if found.Name() != path[len(path)-1] {
			t.Fatalf("command = %q, want %q", found.Name(), path[len(path)-1])
		}
	}
}

func TestRootCommandIncludesDiffSubcommand(t *testing.T) {
	command := newRootCommand(&bytes.Buffer{}, resolve.NewResolveTool(repository.NewSeedRepository()))
	found, _, err := command.Find([]string{"diff"})
	if err != nil {
		t.Fatalf("find diff command: %v", err)
	}

	if found.Name() != "diff" {
		t.Fatalf("command = %q, want diff", found.Name())
	}
}

func TestRootCommandIncludesHistorySubcommands(t *testing.T) {
	command := newRootCommand(&bytes.Buffer{}, resolve.NewResolveTool(repository.NewSeedRepository()))
	for _, path := range [][]string{{"history"}, {"branches-containing"}} {
		found, _, err := command.Find(path)
		if err != nil {
			t.Fatalf("find %v: %v", path, err)
		}
		if found.Name() != path[0] {
			t.Fatalf("command = %q, want %q", found.Name(), path[0])
		}
	}
}

func TestRootCommandIncludesBranchCreateSubcommand(t *testing.T) {
	command := newRootCommand(&bytes.Buffer{}, resolve.NewResolveTool(repository.NewSeedRepository()))

	found, _, err := command.Find([]string{"branch", "create"})
	if err != nil {
		t.Fatalf("find branch create command: %v", err)
	}

	if found.Name() != "create" {
		t.Fatalf("command = %q, want create", found.Name())
	}
}

func TestRootCommandIncludesStatusSubcommand(t *testing.T) {
	command := newRootCommand(&bytes.Buffer{}, resolve.NewResolveTool(repository.NewSeedRepository()))

	found, _, err := command.Find([]string{"status"})
	if err != nil {
		t.Fatalf("find status command: %v", err)
	}

	if found.Name() != "status" {
		t.Fatalf("command = %q, want status", found.Name())
	}
}

func TestRootCommandIncludesCommitSubcommand(t *testing.T) {
	command := newRootCommand(&bytes.Buffer{}, resolve.NewResolveTool(repository.NewSeedRepository()))
	found, _, err := command.Find([]string{"commit"})
	if err != nil {
		t.Fatalf("find commit command: %v", err)
	}
	if found.Name() != "commit" {
		t.Fatalf("command = %q, want commit", found.Name())
	}
}
