package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
)

func TestAssetAddCLI(t *testing.T) {
	repo := newTestSeedRepository(t)

	content := []byte("# System Architecture\n\n```mermaid\nflowchart TD\n  A --> B\n```")
	tempFile := filepath.Join(t.TempDir(), "arch.md")
	if err := os.WriteFile(tempFile, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var output bytes.Buffer
	cmd := NewAssetCommand(func() (*repository.Repository, error) {
		return repo, nil
	})
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"add", "--branch", "main", "--file", tempFile, "--title", "Architecture Spec", "--id", "custom-arch-id"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute spl asset add: %v", err)
	}

	var payload struct {
		Node     string `json:"node"`
		AssetURI string `json:"assetUri"`
		Hash     string `json:"hash"`
		Size     int64  `json:"size"`
		MIMEType string `json:"mimeType"`
		Branch   string `json:"branch"`
		Staged   bool   `json:"staged"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal JSON output %q: %v", output.String(), err)
	}

	if payload.Node != "custom-arch-id" {
		t.Errorf("expected node ID custom-arch-id, got %s", payload.Node)
	}
	if !strings.HasPrefix(payload.AssetURI, "spool://assets/") {
		t.Errorf("expected spool://assets/ URI prefix, got %s", payload.AssetURI)
	}
	if payload.Size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), payload.Size)
	}
	if payload.MIMEType != "text/markdown; charset=utf-8" {
		t.Errorf("expected text/markdown, got %s", payload.MIMEType)
	}
	if payload.Branch != "main" {
		t.Errorf("expected branch main, got %s", payload.Branch)
	}
	if !payload.Staged {
		t.Errorf("expected staged true")
	}

	// Verify staging status has 1 operation
	status, err := repo.BranchStagingStatus("main")
	if err != nil {
		t.Fatalf("BranchStagingStatus: %v", err)
	}
	if status.Operations != 1 {
		t.Errorf("expected 1 staged operation, got %d", status.Operations)
	}
}

func TestAssetReadCLI(t *testing.T) {
	repo := newTestSeedRepository(t)

	content := []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"><circle r=\"50\"/></svg>")
	tempFile := filepath.Join(t.TempDir(), "diagram.svg")
	if err := os.WriteFile(tempFile, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// 1. Add asset
	var addOut bytes.Buffer
	addCmd := NewAssetCommand(func() (*repository.Repository, error) {
		return repo, nil
	})
	addCmd.SetOut(&addOut)
	addCmd.SetArgs([]string{"add", "--branch", "main", "--file", tempFile})
	if err := addCmd.Execute(); err != nil {
		t.Fatalf("asset add: %v", err)
	}

	var addPayload repository.AssetAddResult
	if err := json.Unmarshal(addOut.Bytes(), &addPayload); err != nil {
		t.Fatalf("Unmarshal add result: %v", err)
	}

	// 2. Read via --locator
	var readOutLocator bytes.Buffer
	readCmd1 := NewAssetCommand(func() (*repository.Repository, error) {
		return repo, nil
	})
	readCmd1.SetOut(&readOutLocator)
	readCmd1.SetArgs([]string{"read", "--locator", addPayload.AssetURI})
	if err := readCmd1.Execute(); err != nil {
		t.Fatalf("asset read --locator: %v", err)
	}
	if !bytes.Equal(readOutLocator.Bytes(), content) {
		t.Errorf("read --locator content mismatch: got %q, want %q", readOutLocator.Bytes(), content)
	}

	// 3. Read via --node
	var readOutNode bytes.Buffer
	readCmd2 := NewAssetCommand(func() (*repository.Repository, error) {
		return repo, nil
	})
	readCmd2.SetOut(&readOutNode)
	readCmd2.SetArgs([]string{"read", "--node", addPayload.Node, "--branch", "main"})
	if err := readCmd2.Execute(); err != nil {
		t.Fatalf("asset read --node: %v", err)
	}
	if !bytes.Equal(readOutNode.Bytes(), content) {
		t.Errorf("read --node content mismatch: got %q, want %q", readOutNode.Bytes(), content)
	}

	// 4. Read via positional argument (node ID)
	var readOutPos bytes.Buffer
	readCmd3 := NewAssetCommand(func() (*repository.Repository, error) {
		return repo, nil
	})
	readCmd3.SetOut(&readOutPos)
	readCmd3.SetArgs([]string{"read", addPayload.Node, "--branch", "main"})
	if err := readCmd3.Execute(); err != nil {
		t.Fatalf("asset read positional node: %v", err)
	}
	if !bytes.Equal(readOutPos.Bytes(), content) {
		t.Errorf("read positional node content mismatch: got %q, want %q", readOutPos.Bytes(), content)
	}

	// 5. Read via positional argument (locator URI)
	var readOutPosURI bytes.Buffer
	readCmd4 := NewAssetCommand(func() (*repository.Repository, error) {
		return repo, nil
	})
	readCmd4.SetOut(&readOutPosURI)
	readCmd4.SetArgs([]string{"read", addPayload.AssetURI})
	if err := readCmd4.Execute(); err != nil {
		t.Fatalf("asset read positional URI: %v", err)
	}
	if !bytes.Equal(readOutPosURI.Bytes(), content) {
		t.Errorf("read positional URI content mismatch: got %q, want %q", readOutPosURI.Bytes(), content)
	}
}

func TestAssetCLIErrors(t *testing.T) {
	repo := newTestSeedRepository(t)

	// Missing --file
	cmd1 := NewAssetCommand(func() (*repository.Repository, error) {
		return repo, nil
	})
	cmd1.SetArgs([]string{"add", "--branch", "main"})
	if err := cmd1.Execute(); err == nil {
		t.Errorf("expected error for missing --file flag")
	}

	// Missing --branch
	tempFile := filepath.Join(t.TempDir(), "dummy.txt")
	_ = os.WriteFile(tempFile, []byte("hello"), 0o644)
	cmd2 := NewAssetCommand(func() (*repository.Repository, error) {
		return repo, nil
	})
	cmd2.SetArgs([]string{"add", "--file", tempFile})
	if err := cmd2.Execute(); err == nil {
		t.Errorf("expected error for missing --branch flag")
	}

	// Non-existent file
	cmd3 := NewAssetCommand(func() (*repository.Repository, error) {
		return repo, nil
	})
	cmd3.SetArgs([]string{"add", "--branch", "main", "--file", "/nonexistent/path/file.txt"})
	if err := cmd3.Execute(); err == nil {
		t.Errorf("expected error for non-existent file")
	}

	// Read missing argument
	cmd4 := NewAssetCommand(func() (*repository.Repository, error) {
		return repo, nil
	})
	cmd4.SetArgs([]string{"read"})
	if err := cmd4.Execute(); err == nil {
		t.Errorf("expected error for missing read target")
	}

	// Read non-existent node
	cmd5 := NewAssetCommand(func() (*repository.Repository, error) {
		return repo, nil
	})
	cmd5.SetArgs([]string{"read", "--node", "unknown-node-id", "--branch", "main"})
	if err := cmd5.Execute(); err == nil {
		t.Errorf("expected error for unknown node ID")
	}
}
