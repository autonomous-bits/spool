package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/surface"
)

func TestRootHelpSurfaceKeepOnly(t *testing.T) {
	var output bytes.Buffer
	command := newRootCommand(&output)
	command.SetArgs([]string{"--help"})
	if err := command.Execute(); err != nil {
		t.Fatalf("help: %v", err)
	}
	help := output.String()
	listed := listedAvailableCommands(t, help)
	want := map[string]struct{}{}
	for _, name := range surface.KeepCLITopLevel {
		want[name] = struct{}{}
		if _, ok := listed[name]; !ok {
			t.Errorf("help missing KEEP command %q:\n%s", name, help)
		}
	}
	for name := range listed {
		if _, ok := want[name]; !ok {
			t.Errorf("unexpected command %q listed in spl --help", name)
		}
	}
	for _, name := range surface.RemovedCLITopLevel {
		if _, ok := listed[name]; ok {
			t.Errorf("removed command %q still listed in spl --help", name)
		}
	}
}

func TestRemovedCommandsAreNotRegistered(t *testing.T) {
	command := newRootCommand(&bytes.Buffer{})
	for _, name := range surface.RemovedCLITopLevel {
		if _, _, err := command.Find([]string{name}); err == nil {
			t.Errorf("removed command %q is still registered", name)
		}
	}
	for _, cmd := range command.Commands() {
		for _, alias := range cmd.Aliases {
			for _, removed := range surface.RemovedCLITopLevel {
				if alias == removed || cmd.Name() == removed {
					t.Errorf("removed name %q still present as command %q alias %q", removed, cmd.Name(), alias)
				}
			}
		}
	}
}

func TestKeepCommandsAreRegistered(t *testing.T) {
	command := newRootCommand(&bytes.Buffer{})
	for _, path := range [][]string{
		{"context", "init"},
		{"context", "export"},
		{"context", "migrate-once"},
		{"query-context"},
		{"search"},
		{"search-expand"},
		{"filter"},
		{"resolve"},
		{"graph"},
		{"schema", "migrate"},
		{"validate"},
		{"asset", "add"},
		{"asset", "read"},
		{"merge", "preview"},
		{"mutate"},
		{"prune"},
		{"mcp"},
		{"version"},
	} {
		found, _, err := command.Find(path)
		if err != nil {
			t.Fatalf("find %v: %v", path, err)
		}
		if found.Name() != path[len(path)-1] {
			t.Fatalf("command = %q, want %q", found.Name(), path[len(path)-1])
		}
	}
}

func TestCommandHelpIncludesExamples(t *testing.T) {
	testCases := []struct {
		path    []string
		example string
	}{
		{[]string{"context", "init", "--help"}, "spl context init --remote"},
		{[]string{"query-context", "--help"}, "spl query-context --label Task"},
		{[]string{"resolve", "--help"}, "spl resolve --node"},
		{[]string{"schema", "migrate", "--help"}, "spl schema migrate --schema"},
		{[]string{"validate", "--help"}, "spl validate"},
		{[]string{"filter", "--help"}, "spl filter --label Task"},
		{[]string{"search", "--help"}, "spl search --query incident"},
		{[]string{"search-expand", "--help"}, "spl search-expand --query incident"},
		{[]string{"prune", "--help"}, "spl prune"},
		{[]string{"mutate", "--help"}, "spl mutate --operations"},
	}
	for _, testCase := range testCases {
		t.Run(strings.Join(testCase.path, " "), func(t *testing.T) {
			var output bytes.Buffer
			command := newRootCommand(&output)
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

func TestContextNamespaceIsInitExportOnly(t *testing.T) {
	var output bytes.Buffer
	command := newRootCommand(&output)
	command.SetArgs([]string{"context", "--help"})
	if err := command.Execute(); err != nil {
		t.Fatalf("context help: %v", err)
	}
	help := output.String()
	if !strings.Contains(help, "init") || !strings.Contains(help, "export") || !strings.Contains(help, "migrate-once") {
		t.Fatalf("context help missing KEEP subcommands:\n%s", help)
	}
	if strings.Contains(help, "--query") {
		t.Fatalf("context namespace must not be the query verb:\n%s", help)
	}
	parent, _, err := command.Find([]string{"context"})
	if err != nil {
		t.Fatalf("find context: %v", err)
	}
	if parent.Name() != "context" {
		t.Fatalf("context resolved to %q", parent.Name())
	}
	if len(parent.Aliases) != 0 {
		t.Fatalf("context aliases = %v, want none", parent.Aliases)
	}
	for _, child := range parent.Commands() {
		switch child.Name() {
		case "init", "export", "migrate-once", "help":
		default:
			t.Errorf("unexpected context subcommand %q", child.Name())
		}
	}
}

func TestMutateHasNoVCSAliases(t *testing.T) {
	command := newRootCommand(&bytes.Buffer{})
	found, _, err := command.Find([]string{"mutate"})
	if err != nil {
		t.Fatalf("find mutate: %v", err)
	}
	if found.Name() != "mutate" {
		t.Fatalf("mutate resolved to %q", found.Name())
	}
	if len(found.Aliases) != 0 {
		t.Fatalf("mutate aliases = %v, want none", found.Aliases)
	}
	for _, name := range []string{"add", "commit", "status", "stage", "write"} {
		if _, _, findErr := command.Find([]string{name}); findErr == nil {
			t.Errorf("%q must not be registered as a command or mutate alias", name)
		}
	}
}

func TestQueryContextIsBreakingRenameWithNoAlias(t *testing.T) {
	command := newRootCommand(&bytes.Buffer{})
	found, _, err := command.Find([]string{"query-context"})
	if err != nil {
		t.Fatalf("find query-context: %v", err)
	}
	if found.Name() != "query-context" {
		t.Fatalf("query-context resolved to %q", found.Name())
	}
	if len(found.Aliases) != 0 {
		t.Fatalf("query-context aliases = %v, want none (no alias to old context query)", found.Aliases)
	}
	for _, alias := range found.Aliases {
		if alias == "context" {
			t.Fatal("query-context must not alias old context query verb")
		}
	}

	var output bytes.Buffer
	parent := newRootCommand(&output)
	parent.SetArgs([]string{"context", "--query", "incident"})
	err = parent.Execute()
	if err == nil {
		t.Fatal("spl context --query must not run the query verb")
	}
	if strings.Contains(strings.ToLower(err.Error()), "not bound") {
		t.Fatalf("context --query must fail as unknown flag, not as a query: %v", err)
	}
}

func listedAvailableCommands(t *testing.T, help string) map[string]struct{} {
	t.Helper()
	listed := map[string]struct{}{}
	inSection := false
	for _, line := range strings.Split(help, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Available Commands:") {
			inSection = true
			continue
		}
		if inSection && trimmed == "" {
			break
		}
		if !inSection {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		listed[fields[0]] = struct{}{}
	}
	if len(listed) == 0 {
		t.Fatalf("did not parse Available Commands from help:\n%s", help)
	}
	return listed
}
