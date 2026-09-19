package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/autonomous-bits/spool/internal/repository"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPWriteRequiresBind(t *testing.T) {
	ctx := context.Background()
	server := NewSpoolServerWithOptions(ServerOptions{
		StateDir:     func() (string, error) { return t.TempDir(), nil },
		WorkspaceDir: func() (string, error) { return t.TempDir(), nil },
	})
	session := connectMCP(t, ctx, server)
	defer func() { _ = session.Close() }()

	res, err := session.CallTool(ctx, &officialmcp.CallToolParams{
		Name: "spl_add",
		Arguments: map[string]any{
			"branch": "main",
			"operations": []map[string]any{
				{"action": "add", "entity": "node", "id": "n1", "title": "Nope"},
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("unbound spl_add must fail closed")
	}
	if !strings.Contains(toolText(res), "not bound") {
		t.Fatalf("error = %s, want unbound message", toolText(res))
	}
}

func TestMCPContextExportRequiresBind(t *testing.T) {
	ctx := context.Background()
	server := NewSpoolServerWithOptions(ServerOptions{
		StateDir:     func() (string, error) { return t.TempDir(), nil },
		WorkspaceDir: func() (string, error) { return t.TempDir(), nil },
	})
	session := connectMCP(t, ctx, server)
	defer func() { _ = session.Close() }()

	res, err := session.CallTool(ctx, &officialmcp.CallToolParams{
		Name:      "spl_context_export",
		Arguments: map[string]any{"branch": "main"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("unbound spl_context_export must fail closed")
	}
	if !strings.Contains(toolText(res), "not bound") {
		t.Fatalf("error = %s, want unbound message", toolText(res))
	}
}

func TestMCPContextExportOpensPR(t *testing.T) {
	ctx := context.Background()
	codeRoot, _, cache, recorder := setupMCPBound(t)
	stateDir := filepath.Join(codeRoot, ".spl")
	repo, err := repository.InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{
		Branch: "main",
		Operations: []repository.MutationOperation{
			{Action: "add", Entity: "node", ID: "idea-export", Title: "Exported idea", Labels: []string{"Requirement"}},
		},
	}); err != nil {
		_ = repo.Close()
		t.Fatalf("StageMutationBatch: %v", err)
	}
	if _, err := repo.CommitStagedMutations("main"); err != nil {
		_ = repo.Close()
		t.Fatalf("commit: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("close seed repo: %v", err)
	}

	server := NewSpoolServerWithOptions(ServerOptions{
		StateDir:     func() (string, error) { return stateDir, nil },
		WorkspaceDir: func() (string, error) { return codeRoot, nil },
		CacheDir:     cache,
		PROpener:     recorder,
		Git:          isolatedMCPGit(),
	})
	session := connectMCP(t, ctx, server)
	defer func() { _ = session.Close() }()

	res, err := session.CallTool(ctx, &officialmcp.CallToolParams{
		Name: "spl_context_export",
		Arguments: map[string]any{
			"branch":  "main",
			"author":  "agent",
			"message": "One-shot export",
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("spl_context_export: err=%v res=%s", err, toolText(res))
	}
	text := toolText(res)
	if !strings.Contains(text, `"entrypoint":"export/migrate-once"`) {
		t.Fatalf("missing export entrypoint: %s", text)
	}
	if !strings.Contains(text, `"branch":"spool/mcp/`) || len(recorder.Requests) != 1 {
		t.Fatalf("missing PR path: %s requests=%#v", text, recorder.Requests)
	}
	if !strings.Contains(text, `"kept"`) || !strings.Contains(text, `"skipped"`) {
		t.Fatalf("missing kept/skipped: %s", text)
	}
	if strings.Contains(strings.ToLower(text), "sync from") {
		t.Fatalf("must not describe Rack sync: %s", text)
	}
}

func TestMCPBoundWriteOpensPR(t *testing.T) {
	ctx := context.Background()
	codeRoot, _, cache, recorder := setupMCPBound(t)
	server := NewSpoolServerWithOptions(ServerOptions{
		StateDir:     func() (string, error) { return t.TempDir(), nil },
		WorkspaceDir: func() (string, error) { return codeRoot, nil },
		CacheDir:     cache,
		PROpener:     recorder,
		Git:          isolatedMCPGit(),
	})
	session := connectMCP(t, ctx, server)
	defer func() { _ = session.Close() }()

	res, err := session.CallTool(ctx, &officialmcp.CallToolParams{
		Name: "spl_add",
		Arguments: map[string]any{
			"branch": "main",
			"operations": []map[string]any{
				{"action": "add", "entity": "node", "id": "idea-mcp", "title": "From MCP", "labels": []string{"Requirement"}},
			},
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("spl_add: err=%v res=%s", err, toolText(res))
	}
	res, err = session.CallTool(ctx, &officialmcp.CallToolParams{
		Name: "spl_commit",
		Arguments: map[string]any{
			"branch":  "main",
			"author":  "agent",
			"message": "Add idea from MCP",
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("spl_commit: err=%v res=%s", err, toolText(res))
	}
	text := toolText(res)
	if !strings.Contains(text, `"branch":"spool/mcp/`) {
		t.Fatalf("commit result missing short-lived branch: %s", text)
	}
	if !strings.Contains(text, `"url"`) || len(recorder.Requests) != 1 {
		t.Fatalf("commit result missing PR: %s requests=%#v", text, recorder.Requests)
	}
	res, err = session.CallTool(ctx, &officialmcp.CallToolParams{
		Name:      "spl_resolve",
		Arguments: map[string]any{"branch": "main", "node": "idea-mcp"},
	})
	if err != nil || res.IsError {
		t.Fatalf("spl_resolve: err=%v res=%s", err, toolText(res))
	}
	if !strings.Contains(toolText(res), "From MCP") {
		t.Fatalf("resolve = %s", toolText(res))
	}
}

func TestMCPRefusesRackAndWorkspaceSoT(t *testing.T) {
	ctx := context.Background()
	server := NewSpoolServerWithOptions(ServerOptions{
		StateDir:     func() (string, error) { return t.TempDir(), nil },
		WorkspaceDir: func() (string, error) { return t.TempDir(), nil },
	})
	session := connectMCP(t, ctx, server)
	defer func() { _ = session.Close() }()

	for _, name := range []string{"spl_remote_set", "spl_push", "spl_pull", "spl_clone", "spl_workspace_init"} {
		res, err := session.CallTool(ctx, &officialmcp.CallToolParams{
			Name: name,
			Arguments: map[string]any{
				"endpoint":     "https://rack.example.invalid",
				"auth_mode":    "bearer",
				"workspace_id": "x",
				"branch":       "main",
				"name":         "demo",
			},
		})
		if err != nil {
			t.Fatalf("%s CallTool: %v", name, err)
		}
		if !res.IsError {
			t.Fatalf("%s must refuse Rack/.spl context SoT: %s", name, toolText(res))
		}
		if !strings.Contains(toolText(res), ".spool/context.toml") {
			t.Fatalf("%s error = %s, want bind guidance", name, toolText(res))
		}
	}
}

func setupMCPBound(t *testing.T) (codeRoot, remote, cache string, recorder *ctxgit.RecordingPROpener) {
	t.Helper()
	root := t.TempDir()
	remote = filepath.Join(root, "remote.git")
	git := isolatedMCPGit()
	if _, err := git.Run(context.Background(), root, "init", "--bare", "-b", "main", remote); err != nil {
		t.Fatalf("bare: %v", err)
	}
	codeRoot = filepath.Join(root, "code")
	if err := os.MkdirAll(codeRoot, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cacheRoot := filepath.Join(root, "cache")
	t.Setenv("SPOOL_CONTEXT_CACHE", cacheRoot)
	if _, err := ctxgit.Init(context.Background(), ctxgit.InitRequest{
		WorkspaceDir:    codeRoot,
		Remote:          remote,
		SolutionID:      "mcp-demo",
		ProtectedBranch: "main",
		RepositoryID:    "mcp-repo",
		Author:          "tester <tester@example.com>",
	}, git); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return codeRoot, remote, filepath.Join(cacheRoot, "mcp-demo"), &ctxgit.RecordingPROpener{}
}

func isolatedMCPGit() ctxgit.GitRunner {
	return ctxgit.GitRunner{
		Env: []string{
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_AUTHOR_NAME=spool-test",
			"GIT_AUTHOR_EMAIL=spool-test@example.com",
			"GIT_COMMITTER_NAME=spool-test",
			"GIT_COMMITTER_EMAIL=spool-test@example.com",
		},
	}
}

func connectMCP(t *testing.T, ctx context.Context, server *officialmcp.Server) *officialmcp.ClientSession {
	t.Helper()
	serverTransport, clientTransport := officialmcp.NewInMemoryTransports()
	go func() { _ = server.Run(ctx, serverTransport) }()
	client := officialmcp.NewClient(&officialmcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	return session
}

func toolText(res *officialmcp.CallToolResult) string {
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	raw, _ := json.Marshal(res)
	return string(raw)
}
