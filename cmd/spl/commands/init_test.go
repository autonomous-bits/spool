package commands

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
)

func TestInitCommandReturnsActiveMainBranch(t *testing.T) {
	var output bytes.Buffer
	initialized := false
	command := NewInitCommand(func() (*repository.Repository, error) {
		initialized = true
		return repository.NewSeedRepository(), nil
	})
	command.SetOut(&output)

	if err := command.Execute(); err != nil {
		t.Fatalf("execute init command: %v", err)
	}
	if !initialized {
		t.Fatal("init command did not initialize the repository")
	}

	var result repository.Initialization
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode init result: %v", err)
	}
	want := repository.Initialization{DefaultBranch: "main", ActiveBranch: "main"}
	if result != want {
		t.Fatalf("init result = %#v, want %#v", result, want)
	}
}
