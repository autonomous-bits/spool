package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
)

const schemaMigratePeopleTOML = `
version = 2

[[node]]
label = "Person"
[[node.property]]
key = "name"
required = true
types = ["string"]
`

func schemaMigratePeopleOperations() []repository.MutationOperation {
	return []repository.MutationOperation{
		{
			Action: "update", Entity: "node", ID: repository.SeedNodeID, Title: "Alice",
			Labels: []string{"Person"}, Properties: map[string]repository.PropertyValue{
				"name": repository.StringPropertyValue("Alice"),
			},
		},
	}
}

func TestSchemaMigrateCLIStagesSchemaAndMutationsTogether(t *testing.T) {
	schemaPath, batchPath := writeSchemaMigrationInputs(t, schemaMigratePeopleTOML, schemaMigratePeopleOperations())

	cliRepo := repository.NewSeedRepository()
	var output bytes.Buffer
	if err := runSchemaCommand([]string{
		"migrate", "--branch", "main", "--schema", schemaPath, "--batch", batchPath,
	}, &output, func() (*repository.Repository, error) {
		return cliRepo, nil
	}); err != nil {
		t.Fatalf("execute schema migrate: %v", err)
	}
	var cliResult repository.StageMutationResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode schema migrate result: %v", err)
	}

	mcpRepo := repository.NewSeedRepository()
	schemaTOML, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	mcpResult, err := mcpRepo.StageSchemaMigration(repository.SchemaMigrationRequest{
		Branch: "main", SchemaTOML: schemaTOML, Operations: schemaMigratePeopleOperations(),
	})
	if err != nil {
		t.Fatalf("StageSchemaMigration: %v", err)
	}
	if !reflect.DeepEqual(cliResult, mcpResult) {
		t.Fatalf("CLI result %#v does not match direct repository result %#v", cliResult, mcpResult)
	}
}

func TestSchemaMigrateCLIRejectsInvalidInputAndPropagatesErrors(t *testing.T) {
	validSchema, validBatch := writeSchemaMigrationInputs(t, schemaMigratePeopleTOML, schemaMigratePeopleOperations())
	invalidSchema, _ := writeSchemaMigrationInputs(t, "version =", schemaMigratePeopleOperations())
	invalidBatch := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(invalidBatch, []byte("["), 0o600); err != nil {
		t.Fatalf("write invalid batch: %v", err)
	}
	providerErr := errors.New("open schema repository")

	for _, testCase := range []struct {
		name     string
		schema   string
		batch    string
		provider func() (*repository.Repository, error)
		want     error
	}{
		{
			name: "invalid TOML", schema: invalidSchema, batch: validBatch,
			provider: func() (*repository.Repository, error) {
				return repository.NewSeedRepository(), nil
			},
			want: repository.ErrInvalidSchemaTOML,
		},
		{
			name: "invalid batch JSON", schema: validSchema, batch: invalidBatch,
			provider: func() (*repository.Repository, error) {
				return repository.NewSeedRepository(), nil
			},
		},
		{
			name: "provider error", schema: validSchema, batch: validBatch,
			provider: func() (*repository.Repository, error) { return nil, providerErr },
			want:     providerErr,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			err := runSchemaCommand([]string{
				"migrate", "--branch", "main", "--schema", testCase.schema, "--batch", testCase.batch,
			}, &output, testCase.provider)
			if testCase.want != nil && !errors.Is(err, testCase.want) {
				t.Fatalf("schema migrate error = %v, want %v", err, testCase.want)
			}
			if testCase.want == nil && err == nil {
				t.Fatal("schema migrate unexpectedly succeeded")
			}
			if output.Len() != 0 {
				t.Fatalf("schema migrate wrote output on error: %q", output.String())
			}
		})
	}
}

func TestSchemaMigrateCLIHelpDescribesAtomicStaging(t *testing.T) {
	var output bytes.Buffer
	if err := runSchemaCommand([]string{"migrate", "--help"}, &output, func() (*repository.Repository, error) {
		return repository.NewSeedRepository(), nil
	}); err != nil {
		t.Fatalf("execute schema migrate help: %v", err)
	}
	for _, text := range []string{
		"spl schema migrate --branch main --schema people.toml --batch people-mutations.json",
		"atomically replace the branch's staged set",
		"--schema",
		"--batch",
	} {
		if !strings.Contains(output.String(), text) {
			t.Errorf("schema migrate help does not contain %q:\n%s", text, output.String())
		}
	}
}

func writeSchemaMigrationInputs(t *testing.T, schema string, operations []repository.MutationOperation) (string, string) {
	t.Helper()
	directory := t.TempDir()
	schemaPath := filepath.Join(directory, "schema.toml")
	if err := os.WriteFile(schemaPath, []byte(schema), 0o600); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	data, err := json.Marshal(operations)
	if err != nil {
		t.Fatalf("marshal operations: %v", err)
	}
	batchPath := filepath.Join(directory, "batch.json")
	if err := os.WriteFile(batchPath, data, 0o600); err != nil {
		t.Fatalf("write batch: %v", err)
	}
	return schemaPath, batchPath
}

func runSchemaCommand(args []string, output *bytes.Buffer, repoProvider func() (*repository.Repository, error)) error {
	command := NewSchemaCommand(repoProvider)
	command.SetOut(output)
	command.SetArgs(args)
	return command.Execute()
}
