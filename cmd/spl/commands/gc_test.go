package commands

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
)

func TestGCCLIWritesCompleteJSONReport(t *testing.T) {
	repo, err := repository.InitializeRepository(t.TempDir())
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Errorf("close repository: %v", err)
		}
	})

	var output bytes.Buffer
	command := NewGCCommand(func() (*repository.Repository, error) {
		return repo, nil
	})
	command.SetOut(&output)
	command.SetArgs([]string{"--dry-run"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute gc: %v", err)
	}

	var result repository.GCResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode gc result: %v", err)
	}
	if result.Roots == 0 || result.ReachableObjects == 0 || result.PackedObjects == 0 ||
		result.RetainedUnreachableObjects != 0 || result.PrunedLooseObjects != 0 ||
		result.RetiredPacks != 0 || result.ReclaimedBytes == 0 {
		t.Fatalf("result = %#v, want complete dry-run report", result)
	}
}
