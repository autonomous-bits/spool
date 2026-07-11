import { describe, expect, it, vi, afterEach, beforeEach } from 'vitest';
import {
  parseSearchChunksInput,
  SearchChunksValidationError,
  searchChunks,
} from './search-chunks.js';
import { resetStoreCredentialsForTests } from '../store-client.js';

describe('search-chunks tool', () => {
  describe('parseSearchChunksInput', () => {
    it('parses a valid, fully populated request', () => {
      const result = parseSearchChunksInput({
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

    it('parses a valid, minimal (empty) request', () => {
      const result = parseSearchChunksInput({});
      expect(result).toEqual({});
    });

    it('throws when body is not an object', () => {
      expect(() => parseSearchChunksInput('not-an-object')).toThrow(SearchChunksValidationError);
      expect(() => parseSearchChunksInput(null)).toThrow(SearchChunksValidationError);
    });

    it('throws when limit is not a number', () => {
      expect(() => parseSearchChunksInput({ limit: '10' })).toThrow(SearchChunksValidationError);
    });
  });

  describe('searchChunks', () => {
    beforeEach(() => {
      vi.stubEnv('SPOOL_SESSION_TOKEN', 'test-session-token');
      vi.stubEnv('SPOOL_WORKSPACE_ID', 'test-workspace-id');
    });

    afterEach(() => {
      vi.restoreAllMocks();
      vi.unstubAllEnvs();
      resetStoreCredentialsForTests();
    });

    it('forwards the request and returns the store result on success', async () => {
      const mockResult = { chunks: [{ id: 'c-1' }], nextCursor: null };
      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: vi.fn().mockResolvedValue(mockResult),
      });

      const result = await searchChunks({ q: 'test' }, 'http://localhost:3000');

      expect(result).toEqual(mockResult);
      expect(global.fetch).toHaveBeenCalledWith(
        'http://localhost:3000/chunks?q=test',
        expect.objectContaining({
          method: 'GET',
          headers: {
            authorization: `Bearer test-session-token`,
            'x-workspace-id': 'test-workspace-id',
          },
        }),
      );
    });

    it('appends chunkType as type in query parameters', async () => {
      const mockResult = { chunks: [], nextCursor: null };
      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: vi.fn().mockResolvedValue(mockResult),
      });

      await searchChunks({ chunkType: 'feature' }, 'http://localhost:3000');

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
      });

      await expect(searchChunks({}, 'http://localhost:3000')).rejects.toThrow('Store said no');
    });

    it('falls back to a generic error message if store body is malformed', async () => {
      global.fetch = vi.fn().mockResolvedValue({
        ok: false,
        status: 400,
        json: vi.fn().mockRejectedValue(new Error('unparseable')),
      });

      await expect(searchChunks({}, 'http://localhost:3000')).rejects.toThrow(
        'Store rejected search-chunks request (400)',
      );
    });
  });
});
