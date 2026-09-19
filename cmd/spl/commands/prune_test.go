package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/ctxgit"
)

func TestPruneCLIUnboundRefused(t *testing.T) {
	var output bytes.Buffer
	command := NewPruneCommand(ctxgit.Options{WorkspaceDir: t.TempDir()})
	command.SetOut(&output)
	command.SetArgs([]string{"--dry-run"})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "not bound") {
		t.Fatalf("error = %v, want unbound", err)
	}
}

func TestPruneCLIDryRunEmitsJSON(t *testing.T) {
	opts := boundCommandOptions(t)
	var output bytes.Buffer
	command := NewPruneCommand(opts)
	command.SetOut(&output)
	command.SetArgs([]string{"--dry-run"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute prune --dry-run: %v", err)
	}
	var result ctxgit.PruneResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode prune JSON: %v", err)
	}
	if !result.DryRun {
		t.Fatal("expected result.DryRun = true")
	}
}

func TestQueryContextCLIRequiresSeedSelector(t *testing.T) {
	opts := boundCommandOptions(t)
	for _, testCase := range []struct {
		name string
		args []string
		want string
	}{
		{name: "missing selector", args: nil, want: "provide --query or at least one typed filter"},
		{name: "mixed selectors", args: []string{"--query", "evidence", "--label", "Seed"}, want: "--query cannot be combined with typed filter flags"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			command := NewQueryContextCommand(opts)
			command.SetArgs(testCase.args)
			if err := command.Execute(); err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("query-context error = %v, want %q", err, testCase.want)
			}
		})
	}
}

func TestQueryCommandsBoundJSON(t *testing.T) {
	opts := boundCommandOptions(t)
	session, err := ctxgit.Start(t.Context(), opts)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	_ = session

	var graphOut bytes.Buffer
	graph := NewGraphCommand(opts)
	graph.SetOut(&graphOut)
	if err := graph.Execute(); err != nil {
		t.Fatalf("graph: %v", err)
	}
	if !bytes.Contains(graphOut.Bytes(), []byte(`"nodes"`)) {
		t.Fatalf("graph JSON = %s", graphOut.String())
	}

	var resolveOut bytes.Buffer
	resolveCmd := NewResolveCommand(opts)
	resolveCmd.SetOut(&resolveOut)
	resolveCmd.SetArgs([]string{"--node", "coderepo"})
	if err := resolveCmd.Execute(); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !bytes.Contains(resolveOut.Bytes(), []byte("cli-repo")) {
		t.Fatalf("resolve JSON = %s", resolveOut.String())
	}

	var searchOut bytes.Buffer
	search := NewSearchCommand(opts)
	search.SetOut(&searchOut)
	search.SetArgs([]string{"--query", "cli-repo"})
	if err := search.Execute(); err != nil {
		t.Fatalf("search: %v", err)
	}

	var filterOut bytes.Buffer
	filter := NewFilterCommand(opts)
	filter.SetOut(&filterOut)
	filter.SetArgs([]string{"--label", "CodeRepository"})
	if err := filter.Execute(); err != nil {
		t.Fatalf("filter: %v", err)
	}

	var qcOut bytes.Buffer
	qc := NewQueryContextCommand(opts)
	qc.SetOut(&qcOut)
	qc.SetArgs([]string{"--label", "CodeRepository", "--direction", "both"})
	if err := qc.Execute(); err != nil {
		t.Fatalf("query-context: %v", err)
	}

	var validateOut bytes.Buffer
	validate := NewValidateCommand(opts)
	validate.SetOut(&validateOut)
	if err := validate.Execute(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestSchemaMigrateCLIWritesPR(t *testing.T) {
	opts := boundCommandOptions(t)
	schemaPath := t.TempDir() + "/schema.toml"
	if err := os.WriteFile(schemaPath, []byte("version = 1\npermissive = true\n\n# keep-surface schema write\n"), 0o600); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	var output bytes.Buffer
	command := NewSchemaCommand(opts)
	command.SetOut(&output)
	command.SetArgs([]string{"migrate", "--schema", schemaPath, "--message", "Keep schema"})
	if err := command.Execute(); err != nil {
		t.Fatalf("schema migrate: %v", err)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"branch":"spool/mcp/`)) {
		t.Fatalf("schema migrate JSON = %s", output.String())
	}
}

func TestAssetAddAndReadCLI(t *testing.T) {
	opts := boundCommandOptions(t)
	content := []byte("# notes\n")
	tempFile := t.TempDir() + "/notes.md"
	if err := os.WriteFile(tempFile, content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var addOut bytes.Buffer
	add := NewAssetCommand(opts)
	add.SetOut(&addOut)
	add.SetArgs([]string{"add", "--file", tempFile, "--title", "Notes", "--id", "notes"})
	if err := add.Execute(); err != nil {
		t.Fatalf("asset add: %v\n%s", err, addOut.String())
	}
	if !bytes.Contains(addOut.Bytes(), []byte(`"hash"`)) {
		t.Fatalf("asset add JSON = %s", addOut.String())
	}
}

func TestMergeCLIPreview(t *testing.T) {
	opts := boundCommandOptions(t)
	var output bytes.Buffer
	command := NewMergeCommand(opts)
	command.SetOut(&output)
	command.SetArgs([]string{"preview", "--source", "main", "--target", "main"})
	if err := command.Execute(); err != nil {
		t.Fatalf("merge preview: %v", err)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"clean"`)) {
		t.Fatalf("preview JSON = %s", output.String())
	}
}
