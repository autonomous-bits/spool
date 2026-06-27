---
name: structured-logging
description: >
  Structured logging guidance for Spool's NestJS store and plain node:http MCP
  server. Use when adding logs, request correlation, error logs, audit events,
  or redaction for secrets and PII.
metadata:
  version: "1.0"
  compatibility: "Spool apps/store NestJS Logger; apps/mcp stderr JSON lines"
---

# Structured Logging

Apply these rules when adding logs to `apps/store` or `apps/mcp`.

See [structured-logger.md](./examples/structured-logger.md).

## Store logging

- Use `Logger` from `@nestjs/common`.
- Create one logger per class with `new Logger(ClassName.name)`.
- Use `error` for exceptions, `warn` for degraded behavior, `log` for normal operations, and `debug` for local diagnostics.
- Include safe identifiers such as `requestId`, `tenantId`, and `chunkId`.
- NestJS built-in `Logger` is text-oriented. It does not emit true structured JSON
  fields; include correlation IDs in the message or context. If true JSON logs are
  required, add and configure Pino or `nestjs-pino` deliberately.

```typescript
private readonly logger = new Logger(ChunksService.name);
this.logger.log(`Chunk approved chunkId=${chunkId} tenantId=${tenantId}`);
```

## MCP logging

- `apps/mcp` is plain `node:http`; write diagnostics to `process.stderr`.
- Emit one JSON object per line.
- Do not use `console.log()` for server diagnostics because stdout can be protocol-sensitive.

```typescript
process.stderr.write(JSON.stringify({ level: 'info', msg: 'listening', port }) + '\n');
```

## Redaction rules

- Never log passwords, tokens, secrets, API keys, cookies, authorization headers, credentials, or raw auth context.
- Prefer safe IDs over PII.
- Never log raw request bodies by default.
- Never include SQL params in error logs unless explicitly classified as non-sensitive.

## Error logging

- Log the safe operation name and correlation fields.
- Log `err.message` only after narrowing `err instanceof Error`.
- Re-throw after logging unless the error is intentionally converted to a domain result.

```typescript
try {
  await repository.save(chunk);
} catch (err) {
  const reason = err instanceof Error ? err.message : String(err);
  this.logger.error(`Chunk save failed chunkId=${chunkId} reason=${reason}`);
  throw err;
}
```
