# Spool solution map

Spool is a TypeScript/NestJS monorepo for turning stakeholder chat into approved implementation context.

- `apps/store`: main NestJS knowledge store, stores atomic idea chunks and graph edges, allows for branching and lifecycle management.
- `apps/mcp`: MCP server for agent-facing harness interactions. Allows agents discover, managage, and approve idea chunks and relationships.
- `docs/product`: functional roadmap and deliverable specs for the harness.
- `docs/architecture`: system architecture and engineering constraints.

The core domain model is tenant-scoped idea chunks plus typed relationships in Postgres. Documents are generated projections from the graph, not the source of truth. The chunk lifecycle is `draft -> approved -> promoted`.

## Local store runtime

- Run `apps/store` in Docker with Docker Compose.
- Do not run the store directly on the host for local development unless explicitly requested.

## Local MCP configuration

- This repository includes a workspace-level `.mcp.json` for GitHub Copilot CLI and a matching `.vscode/mcp.json` for VS Code compatibility.
- The `spool` MCP server starts with `node apps/mcp/dist/main.js` and reads from the harness via `HARNESS_URL=http://localhost:3000`.
- When MCP access is relevant, prefer the local `spool` server for Spool context.
- If the MCP server binary is missing, build `apps/mcp` before relying on MCP tools.
