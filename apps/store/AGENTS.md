# Store agent guide

## Purpose

`apps/store` is the NestJS knowledge store for Spool. It owns tenant-scoped idea chunks, typed graph relationships, lifecycle state, and generated document projections.

## Suggested structure

> The diagram below is a guide only. The actual files on disk are authoritative — always check the filesystem before assuming a file exists or a path is correct.

```
apps/store/
├── src/
│   ├── main.ts              # application bootstrap
│   ├── app.module.ts        # root NestJS module and feature wiring
│   ├── **/*.ts              # controllers, services, repositories, DTOs, domain modules
│   └── test/
│       └── setup.ts         # shared test setup and helpers
└── test/
    └── *.e2e-spec.ts        # end-to-end and integration tests
```

## Run tests

Tests use **[Vitest](https://vitest.dev)**. Do not use Jest.

From the repository root:

```sh
pnpm test:store
```

From `apps/store`:

```sh
pnpm test          # vitest run (all unit tests)
pnpm test:watch    # vitest watch mode
pnpm test:e2e      # vitest run test/**/*.e2e-spec.ts
pnpm test:coverage # vitest run --coverage
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


