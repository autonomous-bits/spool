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
