package commands

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
)

func TestWorkspaceInitAndAttachWriteManifest(t *testing.T) {
	root := t.TempDir()
	checkout := t.TempDir()
	command := NewWorkspaceCommand(func() (string, error) { return root, nil })

	var initOutput bytes.Buffer
	command.SetOut(&initOutput)
	command.SetArgs([]string{"init", "platform"})
	if err := command.Execute(); err != nil {
		t.Fatalf("workspace init: %v", err)
	}
	var initialized struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(initOutput.Bytes(), &initialized); err != nil {
		t.Fatalf("decode init output: %v", err)
	}
	if initialized.ID == "" {
		t.Fatal("workspace init returned an empty ID")
	}

	var attachOutput bytes.Buffer
	command = NewWorkspaceCommand(func() (string, error) { return root, nil })
	command.SetOut(&attachOutput)
	command.SetArgs([]string{"attach", "--workspace", "platform", "--repository-id", "github.com/acme/orders", checkout})
	if err := command.Execute(); err != nil {
		t.Fatalf("workspace attach: %v", err)
	}
	_, manifest, found, err := repository.DiscoverWorkspaceManifest(checkout)
	if err != nil {
		t.Fatalf("discover manifest: %v", err)
	}
	if !found || string(manifest.WorkspaceID) != initialized.ID {
		t.Fatalf("manifest workspace ID = %q, want %q", manifest.WorkspaceID, initialized.ID)
	}
	if manifest.RepositoryID != "github.com/acme/orders" {
		t.Fatalf("manifest repository ID = %q", manifest.RepositoryID)
	}
}

func TestWorkspaceCommandExcludesLegacyLifecycleCommands(t *testing.T) {
	command := NewWorkspaceCommand(func() (string, error) { return t.TempDir(), nil })
	for _, name := range []string{"detach", "list", "current", "use", "unset"} {
		found, _, err := command.Find([]string{name})
		if err == nil && found.Name() == name {
			t.Fatalf("legacy workspace %q command is still registered", name)
		}
	}
}

func TestWorkspaceIdentityAvailableRejectsExistingIdentityAndStateDirectory(t *testing.T) {
	registry := repository.WorkspaceRegistry{
		Workspaces: map[repository.WorkspaceName]repository.Workspace{
			"existing": {ID: "ws_00000001", StateDir: "/workspace/repos/ws_00000001"},
		},
	}
	if workspaceIdentityAvailable(registry, "ws_00000001", "/workspace/repos/ws_00000001") {
		t.Fatal("existing workspace identity was accepted")
	}
	if workspaceIdentityAvailable(registry, "ws_00000002", "/workspace/repos/ws_00000001") {
		t.Fatal("existing workspace state directory was accepted")
	}
	if !workspaceIdentityAvailable(registry, "ws_00000002", "/workspace/repos/ws_00000002") {
		t.Fatal("unique workspace identity was rejected")
	}
}
