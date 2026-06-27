# MCP agent guide

## Purpose

`apps/mcp` is the MCP server for agent-facing Spool interactions. It lets agents discover, manage, and approve idea chunks and relationships through the local harness.

## Suggested structure

- `src/main.ts`: MCP server bootstrap.
- `src/server.ts`: server construction, tool registration, and request handling.
- `src/**`: protocol adapters, tool handlers, schemas, and harness clients.
- `src/*.spec.ts`: unit tests colocated with the code they cover.
- `test/**`: integration tests when behavior spans multiple modules.

## Run tests

From the repository root:

```sh
pnpm test:mcp
```

From `apps/mcp`:

```sh
pnpm test
```

Build before relying on the local MCP server binary:

```sh
pnpm build
```
