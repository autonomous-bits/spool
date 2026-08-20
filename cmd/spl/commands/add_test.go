package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
)

func TestAddCLIAndMCPStageEquivalentBatch(t *testing.T) {
	operations := []repository.MutationOperation{
		{Action: "add", Entity: "node", ID: "node-2", Title: "Second node"},
		{Action: "add", Entity: "edge", ID: "edge-1", Source: repository.SeedNodeID, Target: "node-2"},
	}

	batchPath := filepath.Join(t.TempDir(), "batch.json")
	data, err := json.Marshal(operations)
	if err != nil {
		t.Fatalf("marshal batch: %v", err)
	}
	if err := os.WriteFile(batchPath, data, 0o600); err != nil {
		t.Fatalf("write batch: %v", err)
	}

	cliRepo := repository.NewSeedRepository()
	var output bytes.Buffer
	command := NewAddCommand(func() (*resolve.ResolveTool, error) {
		return resolve.NewResolveTool(cliRepo), nil
	})
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main", "--batch", batchPath})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute add command: %v", err)
	}
	var cliResult repository.StageMutationResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}

	mcpRepo := repository.NewSeedRepository()
	mcpResult, err := resolve.NewResolveTool(mcpRepo).EDGStageMutationBatch(context.Background(), repository.StageMutationRequest{
		Branch: "main", Operations: operations,
	})
	if err != nil {
		t.Fatalf("EDGStageMutationBatch: %v", err)
	}
	if !reflect.DeepEqual(cliResult, mcpResult) {
		t.Fatalf("CLI result %#v does not match MCP result %#v", cliResult, mcpResult)
	}
}

func TestAddCLIHelpDescribesEnrichedMutations(t *testing.T) {
	var output bytes.Buffer
	command := NewAddCommand(func() (*resolve.ResolveTool, error) {
		return resolve.NewResolveTool(repository.NewSeedRepository()), nil
	})
	command.SetOut(&output)
	command.SetArgs([]string{"--help"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute add help: %v", err)
	}
	for _, text := range []string{
		"Node operations may include labels and typed properties",
		"edge operations may include type and typed properties",
		`"kind":"integer"`,
		"built-in v1 schema is permissive",
	} {
		if !strings.Contains(output.String(), text) {
			t.Errorf("add help does not contain %q:\n%s", text, output.String())
		}
	}
}

func TestCLIEnrichedMutationBatchReturnsTypedNodeAndEdgeFields(t *testing.T) {
	repo := repository.NewSeedRepository()
	tool := resolve.NewResolveTool(repo)
	base, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch: %v", err)
	}

	batch := `[
		{
			"action":"add",
			"entity":"node",
			"id":"node-2",
			"title":"Authenticate",
			"labels":["Requirement","Decision","Requirement"],
			"properties":{
				"metadata":{
					"kind":"map",
					"map":{
						"priority":{"kind":"integer","integer":3},
						"tags":{"kind":"list","list":[{"kind":"string","string":"cli"},{"kind":"bool","bool":true}]}
					}
				}
			}
		},
		{
			"action":"add",
			"entity":"edge",
			"id":"edge-1",
			"source":"11111111-1111-4111-8111-111111111111",
			"target":"node-2",
			"type":"DEPENDS_ON",
			"properties":{"weight":{"kind":"float","float":1.5}}
		}
	]`
	batchPath := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(batchPath, []byte(batch), 0o600); err != nil {
		t.Fatalf("write batch: %v", err)
	}

	var addOutput bytes.Buffer
	add := NewAddCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	add.SetOut(&addOutput)
	add.SetArgs([]string{"--branch", "main", "--batch", batchPath})
	if err := add.Execute(); err != nil {
		t.Fatalf("execute add: %v", err)
	}
	var staged repository.StageMutationResult
	if err := json.Unmarshal(addOutput.Bytes(), &staged); err != nil {
		t.Fatalf("decode add result: %v", err)
	}
	if staged.Operations != 2 {
		t.Fatalf("staged operations = %d, want 2", staged.Operations)
	}

	var commitOutput bytes.Buffer
	commit := NewCommitCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	commit.SetOut(&commitOutput)
	commit.SetArgs([]string{"--branch", "main"})
	if err := commit.Execute(); err != nil {
		t.Fatalf("execute commit: %v", err)
	}
	var committed repository.CommitStagedMutationResult
	if err := json.Unmarshal(commitOutput.Bytes(), &committed); err != nil {
		t.Fatalf("decode commit result: %v", err)
	}

	var resolveOutput bytes.Buffer
	if err := runResolveCommand([]string{"--branch", "main", "--node", "node-2"}, &resolveOutput, tool); err != nil {
		t.Fatalf("execute resolve: %v", err)
	}
	var resolved resolve.ResolveResult
	if err := json.Unmarshal(resolveOutput.Bytes(), &resolved); err != nil {
		t.Fatalf("decode resolve result: %v", err)
	}
	wantNode := repository.Node{
		ID: "node-2", Title: "Authenticate", Labels: []string{"Decision", "Requirement"},
		Properties: map[string]repository.PropertyValue{
			"metadata": repository.MapPropertyValue(map[string]repository.PropertyValue{
				"priority": repository.IntegerPropertyValue(3),
				"tags": repository.ListPropertyValue([]repository.PropertyValue{
					repository.StringPropertyValue("cli"), repository.BoolPropertyValue(true),
				}),
			}),
		},
	}
	if !resolved.Node.Equal(wantNode) {
		t.Fatalf("resolved node = %#v, want %#v", resolved.Node, wantNode)
	}

	var diffOutput bytes.Buffer
	diff := NewDiffCommand(func() (*resolve.ResolveTool, error) { return tool, nil })
	diff.SetOut(&diffOutput)
	diff.SetArgs([]string{
		"--base-branch", "main", "--base-commit", string(base),
		"--target-branch", "main", "--target-commit", string(committed.Commit),
		"--edge-id", "edge-1", "--max-rows", "10",
	})
	if err := diff.Execute(); err != nil {
		t.Fatalf("execute diff: %v", err)
	}
	var changes repository.DiffResult
	if err := json.Unmarshal(diffOutput.Bytes(), &changes); err != nil {
		t.Fatalf("decode diff result: %v", err)
	}
	wantEdge := repository.Edge{
		ID: "edge-1", Source: repository.SeedNodeID, Target: "node-2", Type: "DEPENDS_ON",
		Properties: map[string]repository.PropertyValue{"weight": repository.FloatPropertyValue(1.5)},
	}
	var gotEdge *repository.Edge
	for _, change := range changes.Changes {
		if change.ID == wantEdge.ID {
			gotEdge = change.Edge
			break
		}
	}
	if gotEdge == nil || !gotEdge.Equal(wantEdge) {
		t.Fatalf("typed edge diff = %#v, want %#v", changes.Changes, wantEdge)
	}
}

func TestAddCLIAcceptsExistingNodeUpdatesAndDeletes(t *testing.T) {
	for _, operation := range []repository.MutationOperation{
		{Action: "update", Entity: "node", ID: repository.SeedNodeID, Title: "Updated seed"},
		{Action: "delete", Entity: "node", ID: repository.SeedNodeID},
	} {
		t.Run(operation.Action, func(t *testing.T) {
			batchPath := filepath.Join(t.TempDir(), "batch.json")
			data, err := json.Marshal([]repository.MutationOperation{operation})
			if err != nil {
				t.Fatalf("marshal batch: %v", err)
			}
			if err := os.WriteFile(batchPath, data, 0o600); err != nil {
				t.Fatalf("write batch: %v", err)
			}
			command := NewAddCommand(func() (*resolve.ResolveTool, error) {
				return resolve.NewResolveTool(repository.NewSeedRepository()), nil
			})
			command.SetOut(&bytes.Buffer{})
			command.SetArgs([]string{"--branch", "main", "--batch", batchPath})
			if err := command.Execute(); err != nil {
				t.Fatalf("execute add command: %v", err)
			}
		})
	}
}

func TestAddCLIRejectsInvalidBatchesWithoutReplacingStagedSet(t *testing.T) {
	stateDir := t.TempDir()
	repo, err := repository.InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}

	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Errorf("Close repository: %v", err)
		}
	})
	run := func(operations []repository.MutationOperation) error {
		t.Helper()
		batchPath := filepath.Join(t.TempDir(), "batch.json")
		data, err := json.Marshal(operations)
		if err != nil {
			t.Fatalf("marshal batch: %v", err)
		}
		if err := os.WriteFile(batchPath, data, 0o600); err != nil {
			t.Fatalf("write batch: %v", err)
		}
		command := NewAddCommand(func() (*resolve.ResolveTool, error) {
			return resolve.NewResolveTool(repo), nil
		})
		command.SetOut(&bytes.Buffer{})
		command.SetArgs([]string{"--branch", "main", "--batch", batchPath})
		return command.Execute()
	}
	valid := []repository.MutationOperation{{Action: "add", Entity: "node", ID: "node-2", Title: "Second node"}}
	if err := run(valid); err != nil {
		t.Fatalf("stage valid batch: %v", err)
	}
	statePath := filepath.Join(stateDir, "staged", "main.json")
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read staged state: %v", err)
	}

	for _, testCase := range []struct {
		name       string
		operations []repository.MutationOperation
		want       error
	}{
		{
			name:       "generic invalid",
			operations: []repository.MutationOperation{{Action: "update", Entity: "node", ID: "missing"}},
			want:       repository.ErrInvalidMutationBatch,
		},
		{
			name:       "missing endpoint masks generic invalid",
			operations: []repository.MutationOperation{{Action: "update", Entity: "node", ID: "missing"}, {Action: "add", Entity: "edge", ID: "edge-1", Source: repository.SeedNodeID, Target: "missing"}},
			want:       repository.ErrMissingEdgeEndpoint,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if err := run(testCase.operations); !errors.Is(err, testCase.want) {
				t.Fatalf("add command error = %v, want %v", err, testCase.want)
			}
			after, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatalf("read state after rejection: %v", err)
			}
			if !bytes.Equal(after, before) {
				t.Fatal("rejected add command changed persisted staged state")
			}
		})
	}
}
