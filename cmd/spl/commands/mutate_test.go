package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/ctxgit"
)

func TestMutateCLIUnboundRefused(t *testing.T) {
	var output bytes.Buffer
	command := NewMutateCommand(ctxgit.Options{WorkspaceDir: t.TempDir()})
	command.SetOut(&output)
	ops := filepath.Join(t.TempDir(), "ops.json")
	if err := os.WriteFile(ops, []byte(`[{"action":"add","entity":"node","id":"n1","title":"Nope"}]`), 0o600); err != nil {
		t.Fatalf("write operations: %v", err)
	}
	command.SetArgs([]string{"--operations", ops})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "not bound") {
		t.Fatalf("error = %v, want unbound", err)
	}
	if output.Len() != 0 {
		t.Fatalf("unbound mutate wrote output: %q", output.String())
	}
}

func TestMutateCLIWritesPR(t *testing.T) {
	opts := boundCommandOptions(t)
	ops := filepath.Join(t.TempDir(), "ops.json")
	payload := `[
  {"action":"add","entity":"node","id":"idea-1","title":"Shared idea","labels":["Requirement"]},
  {"action":"add","entity":"node","id":"idea-2","title":"Related idea","labels":["Requirement"]},
  {"action":"add","entity":"edge","id":"idea-2-relates","source":"idea-2","target":"idea-1","type":"RELATES_TO"}
]`
	if err := os.WriteFile(ops, []byte(payload), 0o600); err != nil {
		t.Fatalf("write operations: %v", err)
	}
	var output bytes.Buffer
	command := NewMutateCommand(opts)
	command.SetOut(&output)
	command.SetArgs([]string{"--operations", ops, "--author", "alice", "--message", "Record shared ideas"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute mutate: %v\n%s", err, output.String())
	}
	var result ctxgit.MutateResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode mutate JSON: %v\n%s", err, output.String())
	}
	if result.Operations != 3 {
		t.Fatalf("operations = %d, want 3", result.Operations)
	}
	if !strings.HasPrefix(result.Branch, "spool/mcp/") {
		t.Fatalf("branch = %q, want spool/mcp/ prefix; json=%s", result.Branch, output.String())
	}
	if result.PR.URL == "" {
		t.Fatalf("missing pullRequest: %s", output.String())
	}
	if len(result.Written) == 0 {
		t.Fatalf("missing written summary: %s", output.String())
	}
}

func TestMutateCLIReadsStdin(t *testing.T) {
	opts := boundCommandOptions(t)
	payload := `[{"action":"add","entity":"node","id":"idea-1","title":"Shared idea","labels":["Requirement"]}]`
	var output bytes.Buffer
	command := NewMutateCommand(opts)
	command.SetIn(bytes.NewReader([]byte(payload)))
	command.SetOut(&output)
	command.SetArgs([]string{"--operations", "-", "--message", "Record from stdin"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute mutate stdin: %v\n%s", err, output.String())
	}
	var result ctxgit.MutateResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode mutate JSON: %v\n%s", err, output.String())
	}
	if !strings.HasPrefix(result.Branch, "spool/mcp/") || result.PR.URL == "" {
		t.Fatalf("stdin mutate = %s", output.String())
	}
}

func TestMutateCLIRejectsInvalidOperations(t *testing.T) {
	opts := boundCommandOptions(t)
	ops := filepath.Join(t.TempDir(), "ops.json")
	if err := os.WriteFile(ops, []byte(`{"not":"an array"}`), 0o600); err != nil {
		t.Fatalf("write operations: %v", err)
	}
	var output bytes.Buffer
	command := NewMutateCommand(opts)
	command.SetOut(&output)
	command.SetArgs([]string{"--operations", ops})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "decode mutation operations") {
		t.Fatalf("error = %v, want decode error", err)
	}
}

func TestMutateCLIHasNoVCSAliases(t *testing.T) {
	command := NewMutateCommand(ctxgit.Options{})
	if len(command.Aliases) != 0 {
		t.Fatalf("mutate aliases = %v, want none", command.Aliases)
	}
	for _, name := range []string{"add", "commit", "status", "stage", "write"} {
		if command.Name() == name {
			t.Fatalf("mutate must not be named %q", name)
		}
		for _, alias := range command.Aliases {
			if alias == name {
				t.Fatalf("mutate aliases to %q", name)
			}
		}
	}
}

func TestMutateCLIHelpDescribesGraphWrite(t *testing.T) {
	var output bytes.Buffer
	command := NewMutateCommand(ctxgit.Options{})
	command.SetOut(&output)
	command.SetArgs([]string{"--help"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute mutate help: %v", err)
	}
	help := output.String()
	for _, text := range []string{
		"spl mutate --operations mutations.json --message \"Record requirement\"",
		"--operations -",
		"short-lived branch",
		"schema migrate",
	} {
		if !strings.Contains(help, text) {
			t.Errorf("mutate help does not contain %q:\n%s", text, help)
		}
	}
	for _, name := range []string{"spl add", "spl commit", "spl status", "spl stage", "spl write", "spl branch", "spl switch"} {
		if strings.Contains(help, name) {
			t.Errorf("mutate help must not teach restored VCS wrapper %q:\n%s", name, help)
		}
	}
}
