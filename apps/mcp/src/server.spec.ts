import { afterEach, describe, expect, it } from 'vitest';
import type { AddressInfo } from 'node:net';
import type { Server } from 'node:http';
import { createMcpHealthResponse, createMcpHttpServer } from './server.js';

describe('MCP HTTP server scaffold', () => {
  let server: Server | undefined;

  afterEach(async () => {
    if (!server?.listening) {
      return;
    }

    await new Promise<void>((resolve, reject) => {
      server?.close((error) => {
        if (error) {
          reject(error);
          return;
        }

        resolve();
      });
    });
  });

  it('builds the default health response', () => {
    expect(createMcpHealthResponse()).toEqual({
      status: 'ok',
      service: 'mcp',
      harnessUrl: 'http://localhost:3000',
    });
  });

  it('responds with MCP health metadata', async () => {
    server = createMcpHttpServer('http://example.test');
    await new Promise<void>((resolve) => server?.listen(0, resolve));

    const { port } = server.address() as AddressInfo;
    const response = await fetch(`http://127.0.0.1:${String(port)}`);

    expect(response.status).toBe(200);
    expect(response.headers.get('content-type')).toContain('application/json');
    await expect(response.json()).resolves.toEqual({
      status: 'ok',
      service: 'mcp',
      harnessUrl: 'http://example.test',
    });
  });
});
