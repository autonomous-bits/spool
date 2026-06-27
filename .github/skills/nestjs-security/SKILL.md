---
name: nestjs-security
description: >
  Security hardening for Spool's Node.js, TypeScript, NestJS, and MCP code.
  Use when adding request handling, validation, auth, SQL access, secrets,
  random identifiers, shutdown paths, or when fixing semgrep security-gate
  findings.
metadata:
  version: "1.0"
  compatibility: "Spool semgrep rules, NestJS 11, Node >=24"
---

# NestJS Security

Apply these rules to `apps/store` and `apps/mcp`. The Security Gate runs `semgrep scan --config .semgrep.yml`; violations block completion.

See [security-bootstrap.md](./examples/security-bootstrap.md).

## Semgrep-enforced rules

- Never call `eval()`.
- Never call `process.exit()` from `apps/**`; set `process.exitCode` and drain resources.
- Never hardcode secrets, tokens, passwords, API keys, or credentials.
- Never concatenate SQL query strings with user data; use parameters.
- Never use `Math.random()` for IDs, tokens, or security decisions.
- Never log sensitive variables through `console.log`.

```typescript
import { randomBytes, randomUUID } from 'node:crypto';

const chunkId = randomUUID();
const nonce = randomBytes(32).toString('hex');
```

## Input validation

- Current dependency-free default: validate untrusted values with explicit guards at
  the boundary before passing them into services.
- If adding NestJS DTO validation, first add `class-validator` and
  `class-transformer` to `apps/store`; then register a global `ValidationPipe`.
- If adding schema-first validation instead, first add `zod` and use it
  consistently for the affected boundary. Do not mix Zod and decorated DTOs in the
  same request path.
- Use DTOs, schemas, or explicit guards for every body, route param, and query
  object.
- For `ValidationPipe`, keep `whitelist: true` and `forbidNonWhitelisted: true`
  enabled, and disable implicit conversion unless the conversion is explicitly
  tested.

```typescript
// Requires: pnpm --filter @spool/store add class-validator class-transformer
app.useGlobalPipes(
  new ValidationPipe({
    whitelist: true,
    forbidNonWhitelisted: true,
    transform: true,
    transformOptions: { enableImplicitConversion: false },
  }),
);
```

## Secrets and configuration

- Read secrets from environment or a secret manager; never store them in source.
- Fail fast at startup when required configuration is absent.
- Keep `config/` for non-secret templates and shared runtime settings only.

## Injection prevention

- Use parameterized SQL queries.
- Treat HTTP input, MCP input, environment variables, and database results as untrusted until validated.
- Avoid user-controlled regular expressions; if unavoidable, bound input length first.

## Shutdown and fatal errors

- In NestJS, use lifecycle hooks to close resources.
- In `apps/mcp`, call `server.close()` and set `process.exitCode`.
- Never mask fatal errors with empty catches or silent returns.
