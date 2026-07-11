import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { InMemoryTransport } from '@modelcontextprotocol/sdk/inMemory.js';
import type { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { createMcpServer } from './server.js';
import { resetStoreCredentialsForTests } from './store-client.js';

const EXPECTED_TOOL_NAMES = [
  'capture-chunk',
  'create-branch',
  'create-edge',
  'submit-suggestion',
  'submit-verification-signal',
  'upload-artifact',
  'attach-artifact-to-chunk',
  'search-chunks',
  'get-neighbourhood',
] as const;

const captureChunkInput = {
  label: 'ATOMIC-1',
  content: 'content',
  discipline: 'product',
  chunkType: 'feature',
  contextKind: 'permanent',
};

const searchChunksInput = {
  q: 'test query',
};

describe('Spool MCP stdio server (Meridian IDEA-137)', () => {
  const originalFetch = fetch;
  const pairs: { client: Client; server: McpServer }[] = [];

  beforeEach(() => {
    vi.stubEnv('SPOOL_SESSION_TOKEN', 'test-session-token');
    vi.stubEnv('SPOOL_WORKSPACE_ID', 'test-workspace-id');
  });

  afterEach(async () => {
    vi.unstubAllGlobals();
    vi.unstubAllEnvs();
    resetStoreCredentialsForTests();

    await Promise.all(
      pairs.map(async ({ client, server }) => {
        await client.close();
        await server.close();
      }),
    );
    pairs.length = 0;
  });

  /**
   * Connects a fresh `createMcpServer()` to a fresh SDK `Client` over a linked in-memory
   * transport pair, performing the real `initialize` handshake. Registers both for teardown
   * so no test leaves a hanging Vitest worker.
   */
  async function connectClientAndServer(storeUrl = 'http://store.test'): Promise<{
    client: Client;
    server: McpServer;
  }> {
    const server = createMcpServer(storeUrl);
    const client = new Client({ name: 'test-client', version: '0.0.0' });
    const [clientTransport, serverTransport] = InMemoryTransport.createLinkedPair();

    await Promise.all([server.connect(serverTransport), client.connect(clientTransport)]);

    pairs.push({ client, server });
    return { client, server };
  }

  it('completes the initialize handshake over a linked in-memory transport pair', async () => {
    const { client } = await connectClientAndServer();

    expect(client.getServerVersion()).toEqual({ name: 'spool-mcp', version: '0.1.0' });
  });

  it('lists exactly the 9 expected tools, each with a non-empty schema', async () => {
    const { client } = await connectClientAndServer();

    const { tools } = await client.listTools();

    expect(tools.map((tool) => tool.name).sort()).toEqual([...EXPECTED_TOOL_NAMES].sort());
    for (const tool of tools) {
      expect(tool.inputSchema).toBeDefined();
      expect(Object.keys(tool.inputSchema.properties ?? {}).length).toBeGreaterThan(0);
    }
  });

  describe('tools/call capture-chunk (host-held session token, G19)', () => {
    it('succeeds and forwards the created chunk on the success path', async () => {
      const createdChunk = { id: 'chunk-1', ...captureChunkInput, status: 'draft' };
      const fetchSpy = vi.fn(async (url: string, init?: RequestInit) => {
        if (url === 'http://store.test/chunks') {
          return new Response(JSON.stringify(createdChunk), {
            status: 201,
            headers: { 'content-type': 'application/json' },
          });
        }
        return originalFetch(url, init);
      });
      vi.stubGlobal('fetch', fetchSpy);

      const { client } = await connectClientAndServer();
      const result = await client.callTool({ name: 'capture-chunk', arguments: captureChunkInput });

      expect(result.isError).toBeFalsy();
      const content = result.content as { type: string; text: string }[];
      expect(JSON.parse(content[0]?.text ?? '')).toEqual(createdChunk);
      expect(fetchSpy).toHaveBeenCalled();
    });

    it('returns isError: true for a schema-valid but semantically invalid (whitespace-only) field, without calling fetch', async () => {
      const fetchSpy = vi.fn(originalFetch);
      vi.stubGlobal('fetch', fetchSpy);

      const { client } = await connectClientAndServer();
      const result = await client.callTool({
        name: 'capture-chunk',
        arguments: { ...captureChunkInput, label: '   ' },
      });

      expect(result.isError).toBe(true);
      expect(fetchSpy).not.toHaveBeenCalled();
    });
  });

  describe('tools/call search-chunks (host-held session token, G19)', () => {
    it('succeeds and forwards the search result on the success path', async () => {
      const searchResult = { chunks: [], nextCursor: null };
      const fetchSpy = vi.fn(async (url: string, init?: RequestInit) => {
        if (url.startsWith('http://store.test/chunks')) {
          return new Response(JSON.stringify(searchResult), {
            status: 200,
            headers: { 'content-type': 'application/json' },
          });
        }
        return originalFetch(url, init);
      });
      vi.stubGlobal('fetch', fetchSpy);

      const { client } = await connectClientAndServer();
      const result = await client.callTool({ name: 'search-chunks', arguments: searchChunksInput });

      expect(result.isError).toBeFalsy();
      const content = result.content as { type: string; text: string }[];
      expect(JSON.parse(content[0]?.text ?? '')).toEqual(searchResult);
      expect(fetchSpy).toHaveBeenCalled();
    });

    it('returns isError: true for a schema-valid but semantically invalid (whitespace-only) field, without calling fetch', async () => {
      const fetchSpy = vi.fn(originalFetch);
      vi.stubGlobal('fetch', fetchSpy);

      const { client } = await connectClientAndServer();
      const result = await client.callTool({
        name: 'search-chunks',
        arguments: { ...searchChunksInput, q: '   ' },
      });

      expect(result.isError).toBe(true);
      expect(fetchSpy).not.toHaveBeenCalled();
    });
  });
});
