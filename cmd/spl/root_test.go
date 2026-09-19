package main

import (
	"bytes"
	"strings"
	"testing"
)

var keepTopLevel = []string{
	"asset", "completion", "context", "filter", "graph", "help", "mcp",
	"merge", "prune", "query-context", "resolve", "schema", "search",
	"search-expand", "validate", "version",
}

var removedTopLevel = []string{
	"init", "workspace", "remote", "push", "pull", "clone", "migrate",
	"fsck", "gc", "cherry-pick", "add", "status", "commit", "branch",
	"switch", "history", "diff", "branches-containing",
}

func TestRootHelpSurfaceKeepOnly(t *testing.T) {
	var output bytes.Buffer
	command := newRootCommand(&output)
	command.SetArgs([]string{"--help"})
	if err := command.Execute(); err != nil {
		t.Fatalf("help: %v", err)
	}
	help := output.String()
	for _, name := range keepTopLevel {
		if !strings.Contains(help, name) {
			t.Errorf("help missing KEEP command %q:\n%s", name, help)
		}
	}
	for _, name := range removedTopLevel {
		// Top-level Available Commands listing: each command starts a help line.
		for _, line := range strings.Split(help, "\n") {
			fields := strings.Fields(line)
			if len(fields) > 0 && fields[0] == name {
				t.Errorf("removed command %q still listed in spl --help: %s", name, line)
			}
		}
	}
}

func TestRemovedCommandsAreNotRegistered(t *testing.T) {
	command := newRootCommand(&bytes.Buffer{})
	for _, name := range removedTopLevel {
		if _, _, err := command.Find([]string{name}); err == nil {
			t.Errorf("removed command %q is still registered", name)
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
}
