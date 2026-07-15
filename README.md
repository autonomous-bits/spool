# Spool

Spool turns stakeholder chat into approved implementation context. It captures conversations as
atomic idea "chunks," links them with typed relationships, and carries them through a
`draft -> approved -> promoted` lifecycle so agents and humans can collaborate on a shared,
versioned knowledge graph instead of scattered documents.

## Architecture

Spool is a TypeScript/NestJS monorepo managed with pnpm workspaces:

| Path         | Description                                                                                     |
| ------------ | ------------------------------------------------------------------------------------------------ |
| `apps/store` | NestJS knowledge store API. Owns tenant-scoped idea chunks, typed graph edges, branches, and lifecycle management, backed by Postgres. |
| `apps/mcp`   | MCP (Model Context Protocol) server for agent-facing interactions — lets agents discover, manage, and approve chunks and relationships. |
| `tools/`     | Shared scripts, codegen utilities, and CLI helpers. Not application code — do not import from here in `apps/`. |
| `config/`    | Shared environment configuration templates consumed by apps and Docker Compose. Not for secrets — use local `.env` files. |
| `docs/`      | Product roadmap, engineering constitution, and goal specifications.                              |

See `docs/constitution.md` for the engineering principles that govern changes to this repository.

## Requirements

- Node.js >= 24
- pnpm (`packageManager: pnpm@11.0.0`, see `package.json`)
- Docker and Docker Compose (for running the store locally)

## Getting started

Install dependencies from the repository root:

```sh
pnpm install
```

### Running the store

The store API is developed and tested through containers, not run directly on the host. Create a
local, untracked `.env` at the repo root with GitHub OAuth App credentials (`GITHUB_CLIENT_ID`,
`GITHUB_CLIENT_SECRET`) — `compose.yaml` reads these for the containerized `spoolstore` service.
For host-side test runs against the compose Postgres service, see `config/store.env.example`.

```sh
pnpm --filter store build
docker compose up --build spoolstore
```

`docker compose` copies the host's pre-built `apps/store/dist/` into the image, so always rebuild
locally (`pnpm --filter store build` or `pnpm build`) before `docker compose up --build`.

The store is available at `http://localhost:3002` once Postgres and its stub dependencies are
healthy.

### Running the MCP server

Build the MCP server, then point an MCP-compatible client (such as this repo's `.mcp.json`) at
`apps/mcp/dist/main.js` with `SPOOL_STORE_URL` set to the running store:

```sh
pnpm --filter mcp build
```

## Development

Common workspace-wide commands, run from the repository root:

```sh
pnpm build       # build all workspaces
pnpm typecheck   # typecheck all workspaces
pnpm test        # run all tests (Vitest)
pnpm test:store  # run apps/store tests only
pnpm test:mcp    # run apps/mcp tests only
pnpm lint        # lint all workspaces
pnpm format      # format with Prettier
```

Each app also has its own `AGENTS.md` (`apps/store/AGENTS.md`, `apps/mcp/AGENTS.md`) with more
detail on structure and testing conventions. Tests use **Vitest**, not Jest.

## Documentation

- `docs/constitution.md` — engineering principles and quality gates
- `docs/architecture.md` — architecture documents

## License

Copyright (C) 2026 autonomous-bits

Licensed under the [GNU Affero General Public License v3.0](LICENSE) (AGPL-3.0-only) — see
`NOTICE` for the project's copyright/permission notice and `package.json`. Unlike plain GPLv3,
the AGPL requires anyone who runs a modified version of Spool as a network service to make that
modified source available to the service's users, closing the "SaaS loophole."
