package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/ctxgit"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPWriteRequiresBind(t *testing.T) {
	ctx := context.Background()
	server := NewSpoolServerWithOptions(ServerOptions{
		WorkspaceDir: func() (string, error) { return t.TempDir(), nil },
	})
	session := connectMCP(t, ctx, server)
	defer func() { _ = session.Close() }()

	for _, tool := range []struct {
		name string
		args map[string]any
	}{
		{name: "spl_prune", args: map[string]any{"dry_run": true}},
		{name: "spl_mutate", args: map[string]any{"operations": []map[string]any{{"action": "add", "entity": "node", "id": "n1", "title": "Nope"}}}},
	} {
		res, err := session.CallTool(ctx, &officialmcp.CallToolParams{
			Name:      tool.name,
			Arguments: tool.args,
		})
		if err != nil {
			t.Fatalf("%s CallTool: %v", tool.name, err)
		}
		if !res.IsError {
			t.Fatalf("unbound %s must fail closed", tool.name)
		}
		if !strings.Contains(toolText(res), "not bound") {
			t.Fatalf("%s error = %s, want unbound message", tool.name, toolText(res))
		}
	}
}

func TestMCPBoundQueryAndSchemaWriteOpensPR(t *testing.T) {
	ctx := context.Background()
	codeRoot, _, cache, recorder := setupMCPBound(t)
	server := NewSpoolServerWithOptions(ServerOptions{
		WorkspaceDir: func() (string, error) { return codeRoot, nil },
		CacheDir:     cache,
		PROpener:     recorder,
		Git:          isolatedMCPGit(),
	})
	session := connectMCP(t, ctx, server)
	defer func() { _ = session.Close() }()

	res, err := session.CallTool(ctx, &officialmcp.CallToolParams{
		Name:      "spl_resolve",
		Arguments: map[string]any{"node": "coderepo"},
	})
	if err != nil || res.IsError {
		t.Fatalf("spl_resolve: err=%v res=%s", err, toolText(res))
	}
	if !strings.Contains(toolText(res), "CodeRepository") && !strings.Contains(toolText(res), "mcp-repo") {
		t.Fatalf("resolve = %s", toolText(res))
	}

	res, err = session.CallTool(ctx, &officialmcp.CallToolParams{
		Name: "spl_schema_migrate",
		Arguments: map[string]any{
			"schema_toml": "version = 1\npermissive = true\n\n# keep-surface schema write\n",
			"message":     "Keep permissive schema",
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("spl_schema_migrate: err=%v res=%s", err, toolText(res))
	}
	text := toolText(res)
	if !strings.Contains(text, `"branch":"spool/mcp/`) {
		t.Fatalf("schema migrate missing short-lived branch: %s", text)
	}
	if !strings.Contains(text, `"url"`) || len(recorder.Requests) != 1 {
		t.Fatalf("schema migrate missing PR: %s requests=%#v", text, recorder.Requests)
	}
}

func TestMCPBoundMutateOpensPR(t *testing.T) {
	ctx := context.Background()
	codeRoot, _, cache, recorder := setupMCPBound(t)
	server := NewSpoolServerWithOptions(ServerOptions{
		WorkspaceDir: func() (string, error) { return codeRoot, nil },
		CacheDir:     cache,
		PROpener:     recorder,
		Git:          isolatedMCPGit(),
	})
	session := connectMCP(t, ctx, server)
	defer func() { _ = session.Close() }()

	res, err := session.CallTool(ctx, &officialmcp.CallToolParams{
		Name: "spl_mutate",
		Arguments: map[string]any{
			"message": "Record shared idea",
			"operations": []map[string]any{
				{"action": "add", "entity": "node", "id": "idea-1", "title": "Shared idea", "labels": []string{"Requirement"}},
			},
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("spl_mutate: err=%v res=%s", err, toolText(res))
	}
	text := toolText(res)
	if !strings.Contains(text, `"branch":"spool/mcp/`) {
		t.Fatalf("mutate missing short-lived branch: %s", text)
	}
	if !strings.Contains(text, `"url"`) || len(recorder.Requests) != 1 {
		t.Fatalf("mutate missing PR: %s requests=%#v", text, recorder.Requests)
	}
	if !strings.Contains(text, `"operations":1`) {
		t.Fatalf("mutate missing operations count: %s", text)
	}
	if !strings.Contains(text, `"written"`) {
		t.Fatalf("mutate missing written summary: %s", text)
	}
	if !strings.Contains(text, `"pullRequest"`) {
		t.Fatalf("mutate missing pullRequest: %s", text)
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
