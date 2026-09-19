package commands

import (
	"os"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/spf13/cobra"
)

func TestLegacySoTCommandsRefuseWhenBound(t *testing.T) {
	root := t.TempDir()
	if _, err := ctxgit.WriteBindFile(root, ctxgit.Bind{
		SolutionID: "demo",
		Remote:     "https://github.com/org/demo-context.git",
	}); err != nil {
		t.Fatalf("WriteBindFile: %v", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	cases := []struct {
		name string
		cmd  *cobra.Command
		args []string
	}{
		{"remote set", NewRemoteCommand(nil), []string{"set", "--endpoint", "https://rack.example.invalid", "--auth-mode", "bearer", "--workspace-id", "x"}},
		{"push", NewPushCommand(nil), []string{"--branch", "main"}},
		{"pull", NewPullCommand(nil), []string{"--branch", "main"}},
		{"clone", NewCloneCommand(), []string{"--endpoint", "https://rack.example.invalid", "--workspace-id", "x"}},
		{"init", NewInitCommand(nil), nil},
		{"workspace init", NewWorkspaceCommandDefault(), []string{"init", "demo"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.cmd.SetArgs(tc.args)
			err := tc.cmd.Execute()
			if err == nil {
				t.Fatal("Execute = nil, want legacy SoT error")
			}
			if !strings.Contains(err.Error(), ctxgit.BindRelPath) || !strings.Contains(err.Error(), "spl context init --remote") {
				t.Fatalf("error = %v, want bind+context git guidance", err)
			}
		})
	}
}
