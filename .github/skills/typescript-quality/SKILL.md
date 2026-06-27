---
name: typescript-quality
description: >
  TypeScript strict-mode quality rules for Spool's NodeNext ESM monorepo.
  Use when writing or reviewing TypeScript, fixing type/lint failures,
  configuring imports, handling unknown data, or working with strict compiler
  options such as noUncheckedIndexedAccess and exactOptionalPropertyTypes.
metadata:
  version: "1.0"
  compatibility: "Spool: Node >=24, pnpm 11, TypeScript 6, ESM NodeNext"
---

# TypeScript Quality

Apply these rules whenever writing or reviewing TypeScript in Spool. They enforce `tsconfig.base.json` and `eslint.config.mjs` without weakening either file.

See [type-safe-patterns.md](./examples/type-safe-patterns.md) for examples.

## ESM NodeNext imports

- Use `.js` extensions on every relative import, even when importing from `.ts` source.
- Use the `node:` protocol for Node built-ins.
- Use `import type` for imports consumed only as types.

```typescript
import { AppModule } from './app.module.js';
import { randomUUID } from 'node:crypto';
import type { AddressInfo } from 'node:net';
```

## Strict access and optional properties

- Treat indexed values as `T | undefined`; narrow or default before use.
- With `exactOptionalPropertyTypes`, omit optional properties instead of assigning `undefined`.

```typescript
const first = chunks[0];
if (first !== undefined) {
  first.toUpperCase();
}

interface ChunkOptions {
  maxTokens?: number;
}
const options: ChunkOptions = {};
```

## Unknown over any

- Never introduce `any`.
- Accept external data as `unknown` and narrow before property access.
- Use explicit guards at IO boundaries by default. Add Zod only when the task
  intentionally introduces `zod` as a dependency and wires it consistently.

```typescript
function extractId(raw: unknown): string {
  if (typeof raw !== 'object' || raw === null) throw new TypeError('Expected object');
  const value = (raw as Record<string, unknown>)['id'];
  if (typeof value !== 'string') throw new TypeError('Expected string id');
  return value;
}
```

## Shape-checked literals

Use `satisfies` for object literals that must match a type while preserving literal precision.

```typescript
const response = {
  status: 'ok',
  service: 'store',
} satisfies { status: 'ok'; service: 'store' | 'mcp' };
```

## Validation checklist

- [ ] Local imports include `.js`.
- [ ] Node built-ins use `node:`.
- [ ] Type-only imports use `import type`.
- [ ] No new `any`, `as any`, or unguarded external data.
- [ ] Indexed values are narrowed or defaulted.
- [ ] Optional properties are omitted instead of set to `undefined`.
