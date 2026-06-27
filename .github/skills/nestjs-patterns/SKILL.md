---
name: nestjs-patterns
description: >
  NestJS architectural patterns for Spool's apps/store service. Use when
  scaffolding modules, controllers, providers, tests, lifecycle hooks, or
  feature boundaries in the NestJS 11 store app.
metadata:
  version: "1.0"
  compatibility: "Spool apps/store: NestJS 11, Vitest 4, TypeScript 6, NodeNext ESM"
---

# NestJS Patterns

Apply these rules when adding or modifying `apps/store`. The store uses NestJS 11, explicit Vitest imports, SWC for decorators, and NodeNext ESM imports.

See [feature-module.md](./examples/feature-module.md) and [feature-module-spec.md](./examples/feature-module-spec.md).

## Module boundaries

- Create one feature module per domain concept.
- Keep `AppModule` thin; it imports feature modules and global infrastructure only.
- Export only providers other modules actually consume.
- Never import application code from `tools/`.

```typescript
@Module({
  controllers: [ChunksController],
  providers: [ChunksService],
  exports: [ChunksService],
})
export class ChunksModule {}
```

## Bootstrap requirements

- Keep `import 'reflect-metadata';` as the first import in `apps/store/src/main.ts`.
- Run the store locally with Docker Compose, not directly on the host.
- Use `.js` extensions in local imports.

```typescript
import 'reflect-metadata';
import { NestFactory } from '@nestjs/core';
import { AppModule } from './app.module.js';
```

## Controllers and services

- Type controller responses with interfaces or DTO types; never return `any` or untyped `object`.
- Put business logic in services, not controllers.
- Use constructor injection; do not manually instantiate providers.
- Use Nest exceptions such as `NotFoundException` for HTTP-safe errors.

## Testing

- Use Vitest, never Jest.
- Import test functions explicitly from `vitest`; globals are disabled.
- Use `Test.createTestingModule()` for unit tests.
- Close `INestApplication` in `afterAll` for e2e tests.
- Do not repeat `vi.restoreAllMocks()` in every file; `apps/store/src/test/setup.ts` handles it.

```typescript
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Test, type TestingModule } from '@nestjs/testing';
```

## Checklist

- [ ] Feature module registered in `AppModule`.
- [ ] Local imports use `.js`.
- [ ] Type-only imports use `import type`.
- [ ] Controller responses are typed.
- [ ] Tests use Vitest imports and `@nestjs/testing`.
- [ ] Store runtime assumptions preserve Docker Compose usage.
