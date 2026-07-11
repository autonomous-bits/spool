import { describe, expect, it, vi, afterEach } from 'vitest';
import {
  parseSearchChunksInput,
  SearchChunksValidationError,
  searchChunks,
} from './search-chunks.js';

describe('search-chunks tool', () => {
  describe('parseSearchChunksInput', () => {
    it('parses a valid, fully populated request', () => {
      const result = parseSearchChunksInput({
        sessionToken: 'token-123',
        workspaceId: 'workspace-123',
        discipline: 'product',
        chunkType: 'feature',
        status: 'draft',
        contextKind: 'permanent',
        branchId: 'branch-123',
        q: 'search query',
        limit: 10,
        cursor: 'cursor-string',
      });
      expect(result).toEqual({
        sessionToken: 'token-123',
        workspaceId: 'workspace-123',
        discipline: 'product',
        chunkType: 'feature',
        status: 'draft',
        contextKind: 'permanent',
        branchId: 'branch-123',
        q: 'search query',
        limit: 10,
        cursor: 'cursor-string',
      });
    });

    it('parses a valid, minimal request', () => {
      const result = parseSearchChunksInput({
        sessionToken: 'token-123',
        workspaceId: 'workspace-123',
      });
      expect(result).toEqual({
        sessionToken: 'token-123',
        workspaceId: 'workspace-123',
      });
    });

    it('throws when body is not an object', () => {
      expect(() => parseSearchChunksInput('not-an-object')).toThrow(SearchChunksValidationError);
      expect(() => parseSearchChunksInput(null)).toThrow(SearchChunksValidationError);
    });

    it('throws when sessionToken is missing or empty', () => {
      expect(() => parseSearchChunksInput({ workspaceId: 'w' })).toThrow(SearchChunksValidationError);
      expect(() => parseSearchChunksInput({ sessionToken: '', workspaceId: 'w' })).toThrow(SearchChunksValidationError);
      expect(() => parseSearchChunksInput({ sessionToken: 123, workspaceId: 'w' })).toThrow(SearchChunksValidationError);
    });

    it('throws when limit is not a number', () => {
      expect(() => parseSearchChunksInput({ sessionToken: 't', workspaceId: 'w', limit: '10' })).toThrow(SearchChunksValidationError);
    });
  });

  describe('searchChunks', () => {
    afterEach(() => {
      vi.restoreAllMocks();
    });

    it('forwards the request and returns the store result on success', async () => {
      const mockResult = { chunks: [{ id: 'c-1' }], nextCursor: null };
      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: vi.fn().mockResolvedValue(mockResult),
      } as unknown as Response);

      const result = await searchChunks(
        { sessionToken: 'token-1', workspaceId: 'w-1', q: 'test' },
        'http://localhost:3000',
      );

      expect(result).toEqual(mockResult);
      expect(global.fetch).toHaveBeenCalledWith(
        'http://localhost:3000/chunks?q=test',
        expect.objectContaining({
          method: 'GET',
          headers: {
            authorization: 'Bearer token-1',
            'x-workspace-id': 'w-1',
          },
        }),
      );
    });

    it('appends chunkType as type in query parameters', async () => {
      const mockResult = { chunks: [], nextCursor: null };
      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: vi.fn().mockResolvedValue(mockResult),
      } as unknown as Response);

      await searchChunks(
        { sessionToken: 'token-1', workspaceId: 'w-1', chunkType: 'feature' },
        'http://localhost:3000',
      );

      expect(global.fetch).toHaveBeenCalledWith(
        'http://localhost:3000/chunks?type=feature',
        expect.anything(),
      );
    });

    it('throws SearchChunksValidationError if the store rejects the request', async () => {
      global.fetch = vi.fn().mockResolvedValue({
        ok: false,
        status: 403,
        json: vi.fn().mockResolvedValue({ message: 'Store said no' }),
      } as unknown as Response);

      await expect(
        searchChunks({ sessionToken: 'token-1', workspaceId: 'w-1' }, 'http://localhost:3000'),
      ).rejects.toThrow('Store said no');
    });

    it('falls back to a generic error message if store body is malformed', async () => {
      global.fetch = vi.fn().mockResolvedValue({
        ok: false,
        status: 400,
        json: vi.fn().mockRejectedValue(new Error('unparseable')),
      } as unknown as Response);

      await expect(
        searchChunks({ sessionToken: 'token-1', workspaceId: 'w-1' }, 'http://localhost:3000'),
      ).rejects.toThrow('Store rejected search-chunks request (400)');
    });
  });
});
