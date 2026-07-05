import { afterEach, describe, expect, it, vi } from 'vitest';
import type { AddressInfo } from 'node:net';
import type { Server } from 'node:http';
import { createMcpHealthResponse, createMcpHttpServer } from './server.js';

describe('MCP HTTP server scaffold', () => {
  let server: Server | undefined;

  afterEach(async () => {
    vi.unstubAllGlobals();

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

  describe('POST /tools/capture-chunk', () => {
    const originalFetch = fetch;
    const captureChunkInput = {
      label: 'ATOMIC-1',
      content: 'content',
      discipline: 'product',
      chunkType: 'feature',
      contextKind: 'permanent',
      stakeholderId: 'stakeholder-1',
    };

    async function startServer(): Promise<number> {
      server = createMcpHttpServer('http://harness.test');
      await new Promise<void>((resolve) => server?.listen(0, resolve));
      return (server.address() as AddressInfo).port;
    }

    async function postCaptureChunk(port: number, body: unknown): Promise<Response> {
      return originalFetch(`http://127.0.0.1:${String(port)}/tools/capture-chunk`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(body),
      });
    }

    it('forwards to HARNESS_URL and returns the created chunk with its id', async () => {
      const createdChunk = { id: 'chunk-1', ...captureChunkInput, status: 'draft' };
      vi.stubGlobal(
        'fetch',
        vi.fn(async (url: string, init?: RequestInit) => {
          if (url === 'http://harness.test/chunks') {
            return new Response(JSON.stringify(createdChunk), {
              status: 201,
              headers: { 'content-type': 'application/json' },
            });
          }
          return originalFetch(url, init);
        }),
      );

      const port = await startServer();
      const response = await postCaptureChunk(port, captureChunkInput);

      expect(response.status).toBe(201);
      await expect(response.json()).resolves.toEqual(createdChunk);
    });

    it('surfaces the store 400 validation error without swallowing it', async () => {
      vi.stubGlobal(
        'fetch',
        vi.fn(async (url: string, init?: RequestInit) => {
          if (url === 'http://harness.test/chunks') {
            return new Response(
              JSON.stringify({ statusCode: 400, message: 'Unknown stakeholderId: stakeholder-1' }),
              { status: 400, headers: { 'content-type': 'application/json' } },
            );
          }
          return originalFetch(url, init);
        }),
      );

      const port = await startServer();
      const response = await postCaptureChunk(port, captureChunkInput);

      expect(response.status).toBe(400);
      await expect(response.json()).resolves.toEqual({
        message: 'Unknown stakeholderId: stakeholder-1',
      });
    });

    it('rejects a malformed body with 400 before contacting the store', async () => {
      const fetchMock = vi.fn(originalFetch);
      vi.stubGlobal('fetch', fetchMock);

      const port = await startServer();
      const response = await postCaptureChunk(port, { content: 'missing label' });

      expect(response.status).toBe(400);
      await expect(response.json()).resolves.toEqual({
        message: 'label must be a non-empty string',
      });
      expect(fetchMock).not.toHaveBeenCalled();
    });
  });

  describe('POST /tools/create-branch', () => {
    const originalFetch = fetch;
    const createBranchInput = {
      name: 'feature/foo',
      discipline: 'product',
      stakeholderId: 'stakeholder-1',
    };

    async function startServer(): Promise<number> {
      server = createMcpHttpServer('http://harness.test');
      await new Promise<void>((resolve) => server?.listen(0, resolve));
      return (server.address() as AddressInfo).port;
    }

    async function postCreateBranch(port: number, body: unknown): Promise<Response> {
      return originalFetch(`http://127.0.0.1:${String(port)}/tools/create-branch`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(body),
      });
    }

    it('forwards to HARNESS_URL and returns the created branch with its id', async () => {
      const createdBranch = { id: 'branch-1', ...createBranchInput, status: 'draft' };
      vi.stubGlobal(
        'fetch',
        vi.fn(async (url: string, init?: RequestInit) => {
          if (url === 'http://harness.test/branches') {
            return new Response(JSON.stringify(createdBranch), {
              status: 201,
              headers: { 'content-type': 'application/json' },
            });
          }
          return originalFetch(url, init);
        }),
      );

      const port = await startServer();
      const response = await postCreateBranch(port, createBranchInput);

      expect(response.status).toBe(201);
      await expect(response.json()).resolves.toEqual(createdBranch);
    });

    it('surfaces the store 400 validation error without swallowing it', async () => {
      vi.stubGlobal(
        'fetch',
        vi.fn(async (url: string, init?: RequestInit) => {
          if (url === 'http://harness.test/branches') {
            return new Response(
              JSON.stringify({ statusCode: 400, message: 'Unknown stakeholderId: stakeholder-1' }),
              { status: 400, headers: { 'content-type': 'application/json' } },
            );
          }
          return originalFetch(url, init);
        }),
      );

      const port = await startServer();
      const response = await postCreateBranch(port, createBranchInput);

      expect(response.status).toBe(400);
      await expect(response.json()).resolves.toEqual({
        message: 'Unknown stakeholderId: stakeholder-1',
      });
    });

    it('rejects a malformed body with 400 before contacting the store', async () => {
      const fetchMock = vi.fn(originalFetch);
      vi.stubGlobal('fetch', fetchMock);

      const port = await startServer();
      const response = await postCreateBranch(port, { discipline: 'product' });

      expect(response.status).toBe(400);
      await expect(response.json()).resolves.toEqual({
        message: 'name must be a non-empty string',
      });
      expect(fetchMock).not.toHaveBeenCalled();
    });
  });

  describe('POST /tools/create-edge', () => {
    const originalFetch = fetch;
    const createEdgeInput = {
      fromChunkLabel: 'IDEA-1',
      toChunkLabel: 'IDEA-2',
      type: 'refines',
      discipline: 'product',
      stakeholderId: 'stakeholder-1',
    };

    async function startServer(): Promise<number> {
      server = createMcpHttpServer('http://harness.test');
      await new Promise<void>((resolve) => server?.listen(0, resolve));
      return (server.address() as AddressInfo).port;
    }

    async function postCreateEdge(port: number, body: unknown): Promise<Response> {
      return originalFetch(`http://127.0.0.1:${String(port)}/tools/create-edge`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(body),
      });
    }

    it('forwards to HARNESS_URL and returns the created edge with its id', async () => {
      const createdEdge = { id: 'edge-1', ...createEdgeInput, status: 'active', supersededByEdgeId: null };
      vi.stubGlobal(
        'fetch',
        vi.fn(async (url: string, init?: RequestInit) => {
          if (url === 'http://harness.test/edges') {
            return new Response(JSON.stringify(createdEdge), {
              status: 201,
              headers: { 'content-type': 'application/json' },
            });
          }
          return originalFetch(url, init);
        }),
      );

      const port = await startServer();
      const response = await postCreateEdge(port, createEdgeInput);

      expect(response.status).toBe(201);
      await expect(response.json()).resolves.toEqual(createdEdge);
    });

    it('surfaces the store 409 conflict error without swallowing it', async () => {
      vi.stubGlobal(
        'fetch',
        vi.fn(async (url: string, init?: RequestInit) => {
          if (url === 'http://harness.test/edges') {
            return new Response(
              JSON.stringify({
                statusCode: 409,
                message: 'An active edge already exists for this from/to/type/branch scope',
              }),
              { status: 409, headers: { 'content-type': 'application/json' } },
            );
          }
          return originalFetch(url, init);
        }),
      );

      const port = await startServer();
      const response = await postCreateEdge(port, createEdgeInput);

      expect(response.status).toBe(409);
      await expect(response.json()).resolves.toEqual({
        message: 'An active edge already exists for this from/to/type/branch scope',
      });
    });

    it('rejects a malformed body with 400 before contacting the store', async () => {
      const fetchMock = vi.fn(originalFetch);
      vi.stubGlobal('fetch', fetchMock);

      const port = await startServer();
      const response = await postCreateEdge(port, { toChunkLabel: 'IDEA-2', discipline: 'product' });

      expect(response.status).toBe(400);
      await expect(response.json()).resolves.toEqual({
        message: 'fromChunkLabel must be a non-empty string',
      });
      expect(fetchMock).not.toHaveBeenCalled();
    });
  });
});
