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
	batch := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(batch, []byte(`[{"action":"add","entity":"node","id":"n1","title":"Nope"}]`), 0o600); err != nil {
		t.Fatalf("write batch: %v", err)
	}
	command.SetArgs([]string{"--batch", batch})
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
	batch := filepath.Join(t.TempDir(), "batch.json")
	payload := `[
  {"action":"add","entity":"node","id":"idea-1","title":"Shared idea","labels":["Requirement"]},
  {"action":"add","entity":"node","id":"idea-2","title":"Related idea","labels":["Requirement"]},
  {"action":"add","entity":"edge","id":"idea-2-relates","source":"idea-2","target":"idea-1","type":"RELATES_TO"}
]`
	if err := os.WriteFile(batch, []byte(payload), 0o600); err != nil {
		t.Fatalf("write batch: %v", err)
	}
	var output bytes.Buffer
	command := NewMutateCommand(opts)
	command.SetOut(&output)
	command.SetArgs([]string{"--batch", batch, "--author", "alice", "--message", "Record shared ideas"})
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
		t.Fatalf("missing PR: %s", output.String())
	}
}

func TestMutateCLIRejectsInvalidBatch(t *testing.T) {
	opts := boundCommandOptions(t)
	batch := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(batch, []byte(`{"not":"an array"}`), 0o600); err != nil {
		t.Fatalf("write batch: %v", err)
	}
	var output bytes.Buffer
	command := NewMutateCommand(opts)
	command.SetOut(&output)
	command.SetArgs([]string{"--batch", batch})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "decode mutation batch") {
		t.Fatalf("error = %v, want decode error", err)
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
		"spl mutate --batch mutations.json --message \"Record requirement\"",
		"short-lived branch",
		"schema migrate",
	} {
		if !strings.Contains(help, text) {
			t.Errorf("mutate help does not contain %q:\n%s", text, help)
		}
	}
	for _, name := range []string{"spl add", "spl commit", "spl status", "spl branch", "spl switch"} {
		if strings.Contains(help, name) {
			t.Errorf("mutate help must not teach restored VCS wrapper %q:\n%s", name, help)
		}
	}
}
