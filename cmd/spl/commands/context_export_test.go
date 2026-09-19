package commands

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/ctxgit"
)

func TestContextExportHelpNamesMigrateOnce(t *testing.T) {
	var output bytes.Buffer
	command := NewContextExportCommand(ctxgit.Options{WorkspaceDir: t.TempDir()}, false)
	command.SetOut(&output)
	command.SetArgs([]string{"--help"})
	if err := command.Execute(); err != nil {
		t.Fatalf("help: %v", err)
	}
	text := strings.ToLower(output.String())
	for _, needle := range []string{"spl context export", "migrate-once", "overwrite-at-own-risk", "not sync"} {
		if !strings.Contains(text, strings.ToLower(needle)) {
			t.Fatalf("help missing %q:\n%s", needle, output.String())
		}
	}
	if strings.Contains(text, "sync from") {
		t.Fatalf("help must not describe Rack sync:\n%s", output.String())
	}
}

func TestContextExportRefusesUnbound(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	command := NewContextExportCommand(ctxgit.Options{WorkspaceDir: dir}, false)
	command.SetArgs([]string{"--branch", "main"})
	err = command.Execute()
	if err == nil || !errors.Is(err, ctxgit.ErrUnbound) {
		t.Fatalf("export error = %v, want ErrUnbound", err)
	}
}
