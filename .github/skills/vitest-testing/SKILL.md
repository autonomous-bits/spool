---
name: vitest-testing
description: >
  Vitest testing patterns for Spool's NestJS store and plain node:http MCP
  server. Use when writing unit tests, NestJS integration or e2e tests, MCP
  integration tests, strict mocks, coverage reports, or diagnosing test
  failures.
metadata:
  version: "1.0"
  compatibility: "Spool apps/store: NestJS 11, Vitest 4, TypeScript 6, NodeNext ESM; apps/mcp: plain node:http"
---

# Vitest Testing

Apply these rules when writing or reviewing tests in `apps/store` or `apps/mcp`.

See [store-unit-spec.md](./examples/store-unit-spec.md), [store-e2e-spec.md](./examples/store-e2e-spec.md), and [mcp-integration-spec.md](./examples/mcp-integration-spec.md).

## Imports and globals

- Import every test function explicitly from `vitest`; both projects set `globals: false`.
- Use `import type` for type-only imports from `@nestjs/common` and `node:*`.
- Do not import from `jest`, use `jest.fn()`, or add `@types/jest`.

```typescript
import { afterAll, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { Test, type TestingModule } from '@nestjs/testing';
import type { INestApplication } from '@nestjs/common';
```

## File placement

- Store unit tests: `apps/store/src/**/*.spec.ts` or `apps/store/src/**/*.test.ts`.
- Store e2e tests: `apps/store/test/**/*.e2e-spec.ts`.
- MCP tests: `apps/mcp/src/**/*.spec.ts` or `apps/mcp/src/**/*.test.ts`.

## Store setup file

`apps/store/src/test/setup.ts` runs before every store test. It imports `reflect-metadata` and calls `vi.restoreAllMocks()` in `afterEach`.

- Do not repeat `import 'reflect-metadata'` in individual store specs.
- Do not repeat `vi.restoreAllMocks()` in individual store specs.
- `vi.restoreAllMocks()` restores spies; reset reusable `vi.fn()` stubs with `mockReset()` or `mockClear()` when needed.

## Unit tests — NestJS store

- Use `Test.createTestingModule()`; do not instantiate providers manually.
- Replace dependencies with `useValue` test doubles.
- Type mocks with `Pick<T, K>` and `satisfies`; avoid `any` and `as unknown as T`.
- Use `vi.mocked(mock.method)` or keep the original `vi.fn<T>()` type accessible.
- Create a fresh testing module in `beforeEach`.

```typescript
const repository = {
  findById: vi.fn<ChunksRepository['findById']>(),
  save: vi.fn<ChunksRepository['save']>(),
} satisfies Pick<ChunksRepository, 'findById' | 'save'>;
```

## Integration and e2e tests — NestJS store

- Import the real `AppModule` for app-level tests.
- Override providers that require live infrastructure with `.overrideProvider(...).useValue(...)`.
- Initialise with `await app.init()`; do not call `app.listen()` in e2e tests.
- Use `supertest` against `app.getHttpServer()`.
- Always close with `await app.close()` in `afterAll`.

```typescript
app = moduleRef.createNestApplication();
await app.init();

const response = await request(app.getHttpServer()).get('/health');
expect(response.status).toBe(200);

await app.close();
```

## Integration tests — MCP node:http

- Call `createMcpHttpServer()` and listen on port `0` to get an OS-assigned port.
- Use `AddressInfo` from `node:net` to read the assigned port.
- Use native `fetch`; Node >=24 provides it globally.
- Prefer `127.0.0.1` over `localhost` to avoid IPv4/IPv6 resolver surprises.
- Pass `AbortSignal.timeout(5_000)` to prevent hanging requests.
- Close the server in `afterEach` with a Promise-wrapped `server.close()`.
- Guard close calls with `if (!server?.listening) return`.

```typescript
server = createMcpHttpServer('http://test.local');
await new Promise<void>((resolve) => server?.listen(0, '127.0.0.1', resolve));
const { port } = server.address() as AddressInfo;

const response = await fetch(`http://127.0.0.1:${port}/`, {
  signal: AbortSignal.timeout(5_000),
});
```

## Strict mocking

- Mock only the methods the test uses.
- Prefer `satisfies Pick<T, K>` for hand-written stubs.
- Use `vi.fn<Service['method']>()` when declaring reusable mocks.
- Use `mockResolvedValue` and `mockRejectedValue` for async dependencies.
- Use `vi.spyOn()` only for real object/module methods; store setup restores spies automatically.
- Use `vi.stubEnv()` for environment variables and `vi.unstubAllEnvs()` in cleanup unless config enables `unstubEnvs`.

## Coverage

- Run all tests: `pnpm test`.
- Run store only: `pnpm test:store`.
- Run MCP only: `pnpm test:mcp`.
- Run store e2e only: `pnpm --filter @spool/store test:e2e`.
- Run coverage: `pnpm test:coverage`.
- Root coverage thresholds: lines 80%, functions 80%, branches 75%, statements 80%.
- Coverage excludes test files, `dist`, `main.ts`, and `*.module.ts`; focus tests on controllers, services, repositories, MCP server behavior, and domain transitions.

## Anti-patterns

- No Jest imports, globals, or APIs.
- No `globals: true` assumptions.
- No untyped mocks, `as any`, or `as unknown as T`.
- No live databases, queues, or external services in unit tests.
- No `app.listen()` in NestJS e2e tests.
- No unclosed `INestApplication` or `node:http` servers.
- No module-level server startup; bind servers inside tests or hooks.

## Checklist

- [ ] Test APIs imported explicitly from `vitest`.
- [ ] Local imports use `.js` extensions.
- [ ] Type-only imports use `import type`.
- [ ] Unit tests use `Test.createTestingModule()`.
- [ ] Mocks use `Pick<T>` + `satisfies`.
- [ ] Store specs do not repeat setup-file cleanup.
- [ ] Store e2e tests call `await app.close()`.
- [ ] MCP tests bind port `0` and close the server.
- [ ] No Jest references.
