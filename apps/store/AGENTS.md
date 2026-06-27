# Store agent guide

## Purpose

`apps/store` is the NestJS knowledge store for Spool. It owns tenant-scoped idea chunks, typed graph relationships, lifecycle state, and generated document projections.

## Suggested structure

- `src/main.ts`: application bootstrap.
- `src/app.module.ts`: root NestJS module and feature wiring.
- `src/**`: controllers, services, repositories, DTOs, and domain modules.
- `src/test/**`: shared test setup and helpers.
- `test/**`: end-to-end and integration tests.

## Run tests

From the repository root:

```sh
pnpm test:store
```

From `apps/store`:

```sh
pnpm test
pnpm test:e2e
```

## Docker runtime

Run the store with Docker Compose from the repository root:

```sh
docker compose up --build spoolstore
```

Use the debug compose file only when debugging:

```sh
docker compose -f compose.debug.yaml up --build spoolstore
```

Do not run the store directly on the host for local development unless explicitly requested.
