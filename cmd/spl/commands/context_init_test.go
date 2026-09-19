package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/ctxgit"
)

func TestContextInitHelp(t *testing.T) {
	var output bytes.Buffer
	command := NewContextInitCommand()
	command.SetOut(&output)
	command.SetArgs([]string{"--help"})
	if err := command.Execute(); err != nil {
		t.Fatalf("help: %v", err)
	}
	if !strings.Contains(output.String(), "spl context init --remote") {
		t.Fatalf("help missing example:\n%s", output.String())
	}
}

func TestContextInitWritesBindAndLayout(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required")
	}
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	if err := exec.Command("git", "init", "--bare", "-b", "main", remote).Run(); err != nil {
		t.Fatalf("bare remote: %v", err)
	}
	code := filepath.Join(root, "code")
	if err := os.MkdirAll(code, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cache := filepath.Join(root, "cache")
	t.Setenv("SPOOL_CONTEXT_CACHE", cache)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(code); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	var output bytes.Buffer
	command := NewContextInitCommand()
	command.SetOut(&output)
	command.SetArgs([]string{"--remote", remote, "--solution-id", "cli-demo", "--repository-id", "cli-repo"})
	if err := command.Execute(); err != nil {
		t.Fatalf("context init: %v\n%s", err, output.String())
	}
	var result ctxgit.InitResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v (%s)", err, output.String())
	}
	if result.BindPath == "" || !result.Pushed || result.SeededNode == "" {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(code, ".spool", "context.toml")); err != nil {
		t.Fatalf("bind file: %v", err)
	}
}
