package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
	statePath := filepath.Join(stateDir, "repository.json")
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
