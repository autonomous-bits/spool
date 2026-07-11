# MCP agent guide

## Purpose

`apps/mcp` is the MCP server for agent-facing Spool interactions. It lets agents discover, manage, and approve idea chunks and relationships through the local harness.

## Suggested structure

> The diagram below is a guide only. The actual files on disk are authoritative — always check the filesystem before assuming a file exists or a path is correct.

```
apps/mcp/
├── src/
│   ├── main.ts              # MCP server bootstrap
│   ├── server.ts            # server construction, tool registration, request handling
│   ├── *.spec.ts            # unit tests colocated with the code they cover
│   └── **/*.ts              # protocol adapters, tool handlers, schemas, harness clients
└── test/
    └── *.spec.ts            # integration tests when behavior spans multiple modules
```

## Run tests

Tests use **[Vitest](https://vitest.dev)**. Do not use Jest.

From the repository root:

```sh
pnpm test:mcp
```

From `apps/mcp`:

```sh
pnpm test          # vitest run (all unit tests)
pnpm test:watch    # vitest watch mode
pnpm test:coverage # vitest run --coverage
```

Build before relying on the local MCP server binary:

```sh
pnpm build
```

## Client configuration (stdio transport)

`apps/mcp` is a real stdio JSON-RPC MCP server (Meridian IDEA-137): `main.ts` connects a
`McpServer` to a `StdioServerTransport` over the process's stdin/stdout. There is no HTTP surface
(no `GET /health`) — a client spawns `node dist/main.js` as a child process and speaks MCP over
its stdio streams.

Both the workspace-level `.mcp.json` (GitHub Copilot CLI) and `.vscode/mcp.json` (VS Code) at the
repo root register a `spool` server entry with an explicit `cwd` pointing at the compiled output,
since the entrypoint is resolved relative to the spawned process's working directory:

```json
{
  "type": "stdio",
  "command": "node",
  "args": ["main.js"],
  "cwd": "/absolute/path/to/spool/apps/mcp/dist",
  "env": { "SPOOL_STORE_URL": "http://localhost:3000" }
}
```

Run `pnpm --filter mcp build` (or `pnpm build` from the repo root) before starting a client that
spawns this entrypoint, or it will run stale compiled code.
