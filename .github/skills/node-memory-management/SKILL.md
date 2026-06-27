---
name: node-memory-management
description: >
  Node.js memory and resource-lifecycle guidance for Spool. Use when handling
  streams, large datasets, caches, timers, event listeners, shutdown hooks,
  background jobs, heap pressure, OOMs, or leak investigations.
metadata:
  version: "1.0"
  compatibility: "Spool Node >=24, NestJS lifecycle hooks, plain node:http MCP server"
---

# Node Memory Management

Apply these rules when writing stream processing, background jobs, caches, or long-lived resources.

See [stream-pipeline.md](./examples/stream-pipeline.md).

## Streams

- Use `pipeline` from `node:stream/promises`; do not hand-roll `.pipe()` chains.
- Pass `AbortSignal` for cancellable long-running work.
- Avoid buffering large files, HTTP responses, or graph projections into memory.

```typescript
const controller = new AbortController();
const timeoutId = setTimeout(() => controller.abort(), 30_000);
try {
  await pipeline(source, transform, destination, { signal: controller.signal });
} finally {
  clearTimeout(timeoutId);
}
```

## NestJS resource lifecycle

- Use `OnApplicationShutdown` for resources that must be closed on `SIGTERM` or `app.close()`.
- Track `AbortController`, interval, timeout, subscription, and event listener handles.
- Clear every handle in the shutdown hook.

## MCP resource lifecycle

- In `apps/mcp`, keep a reference to the HTTP server returned by `createMcpHttpServer`.
- On `SIGTERM`, call `server.close()` and set `process.exitCode`.
- Never call `process.exit()` in `apps/**`.

## Caches

- Do not use unbounded `Map` caches.
- Use bounded caches for primitive keys.
- Use `WeakMap` for object-keyed caches where object lifetime should control cache lifetime.

```typescript
const cache = new WeakMap<ChunkGraph, DocumentProjection>();
```

## Checklist

- [ ] Large work streams or paginates instead of buffering.
- [ ] Pipelines use `node:stream/promises`.
- [ ] Timers and listeners are cleared.
- [ ] Shutdown hooks close resources.
- [ ] Caches are bounded or weakly keyed.
