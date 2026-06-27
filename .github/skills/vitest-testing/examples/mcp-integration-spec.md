# MCP Integration Spec Example

Integration test for the `apps/mcp` plain `node:http` server. Demonstrates port-0 binding, native `fetch`, route assertions, request timeout, and guaranteed teardown.

File location: `apps/mcp/src/<feature>.spec.ts`

```typescript
import type { AddressInfo } from 'node:net';
import type { Server } from 'node:http';
import { afterEach, describe, expect, it } from 'vitest';
import { createMcpHttpServer } from './server.js';

describe('MCP HTTP server', () => {
  let server: Server | undefined;

  afterEach(async () => {
    if (!server?.listening) return;

    await new Promise<void>((resolve, reject) => {
      server?.close((error) => (error ? reject(error) : resolve()));
    });
  });

  it('responds with MCP health metadata', async () => {
    server = createMcpHttpServer('http://store.internal:3000');
    await new Promise<void>((resolve) => server?.listen(0, '127.0.0.1', resolve));

    const { port } = server.address() as AddressInfo;
    const response = await fetch(`http://127.0.0.1:${port}/`, {
      signal: AbortSignal.timeout(5_000),
    });

    expect(response.status).toBe(200);
    expect(response.headers.get('content-type')).toContain('application/json');
    await expect(response.json()).resolves.toEqual({
      status: 'ok',
      service: 'mcp',
      harnessUrl: 'http://store.internal:3000',
    });
  });

  it('does not start listening when only constructing the server', () => {
    server = createMcpHttpServer('http://test.local');

    expect(server.listening).toBe(false);
  });
});
```
