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
      workspaceId: 'workspace-1',
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
      workspaceId: 'workspace-1',
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
      workspaceId: 'workspace-1',
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

  describe('POST /tools/submit-suggestion', () => {
    const originalFetch = fetch;
    const submitSuggestionInput = {
      label: 'IDEA-1',
      content: 'content',
      discipline: 'product',
      stakeholderId: 'stakeholder-1',
      workspaceId: 'workspace-1',
    };

    async function startServer(): Promise<number> {
      server = createMcpHttpServer('http://harness.test');
      await new Promise<void>((resolve) => server?.listen(0, resolve));
      return (server.address() as AddressInfo).port;
    }

    async function postSubmitSuggestion(port: number, body: unknown): Promise<Response> {
      return originalFetch(`http://127.0.0.1:${String(port)}/tools/submit-suggestion`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(body),
      });
    }

    it('forwards to HARNESS_URL and returns the created suggestion with its id', async () => {
      const createdSuggestion = {
        id: 'suggestion-1',
        ...submitSuggestionInput,
        fromChunkLabel: null,
        toChunkLabel: null,
        relationshipType: null,
        status: 'pending',
        submittedByStakeholderId: submitSuggestionInput.stakeholderId,
        submittedByActorKind: 'delegated',
        decidedByStakeholderId: null,
        decidedAt: null,
      };
      vi.stubGlobal(
        'fetch',
        vi.fn(async (url: string, init?: RequestInit) => {
          if (url === 'http://harness.test/suggestions') {
            return new Response(JSON.stringify(createdSuggestion), {
              status: 201,
              headers: { 'content-type': 'application/json' },
            });
          }
          return originalFetch(url, init);
        }),
      );

      const port = await startServer();
      const response = await postSubmitSuggestion(port, submitSuggestionInput);

      expect(response.status).toBe(201);
      await expect(response.json()).resolves.toEqual(createdSuggestion);
    });

    it('surfaces the store 400 validation error without swallowing it', async () => {
      vi.stubGlobal(
        'fetch',
        vi.fn(async (url: string, init?: RequestInit) => {
          if (url === 'http://harness.test/suggestions') {
            return new Response(
              JSON.stringify({
                statusCode: 400,
                message: 'Unknown stakeholderId: stakeholder-1',
              }),
              { status: 400, headers: { 'content-type': 'application/json' } },
            );
          }
          return originalFetch(url, init);
        }),
      );

      const port = await startServer();
      const response = await postSubmitSuggestion(port, submitSuggestionInput);

      expect(response.status).toBe(400);
      await expect(response.json()).resolves.toEqual({
        message: 'Unknown stakeholderId: stakeholder-1',
      });
    });

    it('rejects a malformed body with 400 before contacting the store', async () => {
      const fetchMock = vi.fn(originalFetch);
      vi.stubGlobal('fetch', fetchMock);

      const port = await startServer();
      const response = await postSubmitSuggestion(port, { discipline: 'product' });

      expect(response.status).toBe(400);
      await expect(response.json()).resolves.toEqual({
        message: 'stakeholderId must be a non-empty string',
      });
      expect(fetchMock).not.toHaveBeenCalled();
    });
  });

  describe('POST /tools/submit-verification-signal', () => {
    const originalFetch = fetch;
    const submitVerificationSignalInput = {
      branchId: 'branch-1',
      verifierName: 'ci-evaluator',
      status: 'pass',
      reason: 'Checks passed.',
      workspaceId: 'workspace-1',
    };

    async function startServer(): Promise<number> {
      server = createMcpHttpServer('http://harness.test');
      await new Promise<void>((resolve) => server?.listen(0, resolve));
      return (server.address() as AddressInfo).port;
    }

    async function postSubmitVerificationSignal(port: number, body: unknown): Promise<Response> {
      return originalFetch(`http://127.0.0.1:${String(port)}/tools/submit-verification-signal`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(body),
      });
    }

    it('forwards to HARNESS_URL and returns the created verification signal with its id', async () => {
      const createdSignal = {
        id: 'signal-1',
        branchId: submitVerificationSignalInput.branchId,
        verifierName: submitVerificationSignalInput.verifierName,
        status: 'pass',
        reason: submitVerificationSignalInput.reason,
        createdAt: '2026-01-01T00:00:00.000Z',
      };
      vi.stubGlobal(
        'fetch',
        vi.fn(async (url: string, init?: RequestInit) => {
          if (url === 'http://harness.test/branches/branch-1/verification-signals') {
            return new Response(JSON.stringify(createdSignal), {
              status: 201,
              headers: { 'content-type': 'application/json' },
            });
          }
          return originalFetch(url, init);
        }),
      );

      const port = await startServer();
      const response = await postSubmitVerificationSignal(port, submitVerificationSignalInput);

      expect(response.status).toBe(201);
      await expect(response.json()).resolves.toEqual(createdSignal);
    });

    it('surfaces the store 4xx validation error without swallowing it', async () => {
      vi.stubGlobal(
        'fetch',
        vi.fn(async (url: string, init?: RequestInit) => {
          if (url === 'http://harness.test/branches/branch-1/verification-signals') {
            return new Response(
              JSON.stringify({
                statusCode: 409,
                message: 'Branch branch-1 is not reviewable',
              }),
              { status: 409, headers: { 'content-type': 'application/json' } },
            );
          }
          return originalFetch(url, init);
        }),
      );

      const port = await startServer();
      const response = await postSubmitVerificationSignal(port, submitVerificationSignalInput);

      expect(response.status).toBe(409);
      await expect(response.json()).resolves.toEqual({
        message: 'Branch branch-1 is not reviewable',
      });
    });

    it('rejects a malformed body with 400 before contacting the store', async () => {
      const fetchMock = vi.fn(originalFetch);
      vi.stubGlobal('fetch', fetchMock);

      const port = await startServer();
      const response = await postSubmitVerificationSignal(port, { verifierName: 'ci-evaluator' });

      expect(response.status).toBe(400);
      await expect(response.json()).resolves.toEqual({
        message: 'branchId must be a non-empty string',
      });
      expect(fetchMock).not.toHaveBeenCalled();
    });
  });

  describe('POST /tools/upload-artifact', () => {
    const originalFetch = fetch;
    const uploadArtifactInput = {
      content: Buffer.from('hello world').toString('base64'),
      mimeType: 'text/plain',
      stakeholderId: 'stakeholder-1',
      workspaceId: 'workspace-1',
    };

    async function startServer(): Promise<number> {
      server = createMcpHttpServer('http://harness.test');
      await new Promise<void>((resolve) => server?.listen(0, resolve));
      return (server.address() as AddressInfo).port;
    }

    async function postUploadArtifact(port: number, body: unknown): Promise<Response> {
      return originalFetch(`http://127.0.0.1:${String(port)}/tools/upload-artifact`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(body),
      });
    }

    it('forwards to HARNESS_URL and returns the created artifact with its id', async () => {
      const createdArtifact = {
        id: 'artifact-1',
        uri: 'file:///artifacts/artifact-1',
        mimeType: uploadArtifactInput.mimeType,
        createdByStakeholderId: uploadArtifactInput.stakeholderId,
        createdAt: '2026-01-01T00:00:00.000Z',
      };
      vi.stubGlobal(
        'fetch',
        vi.fn(async (url: string, init?: RequestInit) => {
          if (url === 'http://harness.test/artifacts') {
            return new Response(JSON.stringify(createdArtifact), {
              status: 201,
              headers: { 'content-type': 'application/json' },
            });
          }
          return originalFetch(url, init);
        }),
      );

      const port = await startServer();
      const response = await postUploadArtifact(port, uploadArtifactInput);

      expect(response.status).toBe(201);
      await expect(response.json()).resolves.toEqual(createdArtifact);
    });

    it('rejects decoded content exceeding the max artifact size with a clear 400, before contacting the store', async () => {
      const fetchMock = vi.fn(originalFetch);
      vi.stubGlobal('fetch', fetchMock);

      const oversized = Buffer.alloc(700_001, 'a').toString('base64');
      const port = await startServer();
      const response = await postUploadArtifact(port, { ...uploadArtifactInput, content: oversized });

      expect(response.status).toBe(400);
      await expect(response.json()).resolves.toEqual({
        message: expect.stringContaining('exceeds the maximum artifact size'),
      });
      expect(fetchMock).not.toHaveBeenCalled();
    });

    it('rejects a malformed body with 400 before contacting the store', async () => {
      const fetchMock = vi.fn(originalFetch);
      vi.stubGlobal('fetch', fetchMock);

      const port = await startServer();
      const response = await postUploadArtifact(port, { mimeType: 'text/plain' });

      expect(response.status).toBe(400);
      await expect(response.json()).resolves.toEqual({
        message: 'content must be a non-empty string',
      });
      expect(fetchMock).not.toHaveBeenCalled();
    });
  });

  describe('POST /tools/attach-artifact-to-chunk', () => {
    const originalFetch = fetch;
    const attachArtifactToChunkInput = {
      chunkLabel: 'IDEA-1',
      artifactId: 'artifact-1',
      stakeholderId: 'stakeholder-1',
      workspaceId: 'workspace-1',
    };

    async function startServer(): Promise<number> {
      server = createMcpHttpServer('http://harness.test');
      await new Promise<void>((resolve) => server?.listen(0, resolve));
      return (server.address() as AddressInfo).port;
    }

    async function postAttachArtifactToChunk(port: number, body: unknown): Promise<Response> {
      return originalFetch(`http://127.0.0.1:${String(port)}/tools/attach-artifact-to-chunk`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(body),
      });
    }

    it('forwards to HARNESS_URL and returns the created association', async () => {
      const createdAssociation = {
        id: 'assoc-1',
        chunkLabel: attachArtifactToChunkInput.chunkLabel,
        artifactId: attachArtifactToChunkInput.artifactId,
        status: 'active',
        branchId: null,
        originBranchId: null,
        createdByStakeholderId: attachArtifactToChunkInput.stakeholderId,
        updatedByStakeholderId: attachArtifactToChunkInput.stakeholderId,
        createdAt: '2026-01-01T00:00:00.000Z',
        updatedAt: '2026-01-01T00:00:00.000Z',
      };
      vi.stubGlobal(
        'fetch',
        vi.fn(async (url: string, init?: RequestInit) => {
          if (url === 'http://harness.test/chunks/IDEA-1/artifacts') {
            return new Response(JSON.stringify(createdAssociation), {
              status: 201,
              headers: { 'content-type': 'application/json' },
            });
          }
          return originalFetch(url, init);
        }),
      );

      const port = await startServer();
      const response = await postAttachArtifactToChunk(port, attachArtifactToChunkInput);

      expect(response.status).toBe(201);
      await expect(response.json()).resolves.toEqual(createdAssociation);
    });

    it('surfaces the store 404 error without swallowing it', async () => {
      vi.stubGlobal(
        'fetch',
        vi.fn(async (url: string, init?: RequestInit) => {
          if (url === 'http://harness.test/chunks/IDEA-1/artifacts') {
            return new Response(
              JSON.stringify({
                statusCode: 404,
                message: 'Chunk with label IDEA-1 not found in this scope',
              }),
              { status: 404, headers: { 'content-type': 'application/json' } },
            );
          }
          return originalFetch(url, init);
        }),
      );

      const port = await startServer();
      const response = await postAttachArtifactToChunk(port, attachArtifactToChunkInput);

      expect(response.status).toBe(404);
      await expect(response.json()).resolves.toEqual({
        message: 'Chunk with label IDEA-1 not found in this scope',
      });
    });

    it('rejects a malformed body with 400 before contacting the store', async () => {
      const fetchMock = vi.fn(originalFetch);
      vi.stubGlobal('fetch', fetchMock);

      const port = await startServer();
      const response = await postAttachArtifactToChunk(port, { chunkLabel: 'IDEA-1' });

      expect(response.status).toBe(400);
      await expect(response.json()).resolves.toEqual({
        message: 'artifactId must be a non-empty string',
      });
      expect(fetchMock).not.toHaveBeenCalled();
    });
  });

  describe('POST /tools/search-chunks', () => {
    const originalFetch = fetch;
    const searchChunksInput = {
      sessionToken: 'token-1',
      workspaceId: 'w-1',
      q: 'test',
    };

    async function startServer(): Promise<number> {
      server = createMcpHttpServer('http://harness.test');
      await new Promise<void>((resolve) => server?.listen(0, resolve));
      return (server.address() as AddressInfo).port;
    }

    async function postSearchChunks(port: number, body: unknown): Promise<Response> {
      return originalFetch(`http://127.0.0.1:${String(port)}/tools/search-chunks`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(body),
      });
    }

    it('returns 200 on success', async () => {
      const mockResult = { chunks: [], nextCursor: null };
      vi.stubGlobal(
        'fetch',
        vi.fn(async (url: string, init?: RequestInit) => {
          if (url === 'http://harness.test/chunks?q=test') {
            return new Response(JSON.stringify(mockResult), {
              status: 200,
              headers: { 'content-type': 'application/json' },
            });
          }
          return originalFetch(url, init);
        }),
      );

      const port = await startServer();
      const response = await postSearchChunks(port, searchChunksInput);

      expect(response.status).toBe(200);
      await expect(response.json()).resolves.toEqual(mockResult);
    });

    it('surfaces 400 validation errors from the store', async () => {
      vi.stubGlobal(
        'fetch',
        vi.fn(async (url: string, init?: RequestInit) => {
          if (url.startsWith('http://harness.test/chunks')) {
            return new Response(JSON.stringify({ message: 'Invalid query' }), {
              status: 400,
              headers: { 'content-type': 'application/json' },
            });
          }
          return originalFetch(url, init);
        }),
      );

      const port = await startServer();
      const response = await postSearchChunks(port, searchChunksInput);

      expect(response.status).toBe(400);
      await expect(response.json()).resolves.toEqual({ message: 'Invalid query' });
    });

    it('returns 400 for malformed JSON', async () => {
      const port = await startServer();
      const response = await originalFetch(`http://127.0.0.1:${String(port)}/tools/search-chunks`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: '{ bad json }',
      });

      expect(response.status).toBe(400);
      await expect(response.json()).resolves.toEqual({ message: 'Request body must be valid JSON' });
    });
  });
});
