import { describe, expect, it, vi, afterEach, beforeEach } from 'vitest';
import {
  parseGetNeighbourhoodInput,
  GetNeighbourhoodValidationError,
  getNeighbourhood,
} from './get-neighbourhood.js';
import { resetStoreCredentialsForTests } from '../store-client.js';

describe('get-neighbourhood tool', () => {
  describe('parseGetNeighbourhoodInput', () => {
    it('parses a valid, fully populated request', () => {
      const result = parseGetNeighbourhoodInput({
        id: 'chunk-123',
        depth: 2,
        branchId: 'branch-123',
      });
      expect(result).toEqual({
        id: 'chunk-123',
        depth: 2,
        branchId: 'branch-123',
      });
    });

    it('parses a valid, minimal request', () => {
      const result = parseGetNeighbourhoodInput({
        id: 'chunk-123',
      });
      expect(result).toEqual({
        id: 'chunk-123',
      });
    });

    it('throws when body is not an object', () => {
      expect(() => parseGetNeighbourhoodInput('not-an-object')).toThrow(GetNeighbourhoodValidationError);
      expect(() => parseGetNeighbourhoodInput(null)).toThrow(GetNeighbourhoodValidationError);
    });

    it('throws when required fields are missing', () => {
      expect(() => parseGetNeighbourhoodInput({})).toThrow(GetNeighbourhoodValidationError);
      expect(() => parseGetNeighbourhoodInput({ id: '' })).toThrow(GetNeighbourhoodValidationError);
    });

    it('throws when depth is not a number', () => {
      expect(() => parseGetNeighbourhoodInput({ id: 'i', depth: '2' })).toThrow(GetNeighbourhoodValidationError);
    });
  });

  describe('getNeighbourhood', () => {
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
      const mockResult = { chunk: { id: 'chunk-123' }, neighbours: [] };
      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: vi.fn().mockResolvedValue(mockResult),
      });

      const result = await getNeighbourhood(
        { id: 'chunk-123', depth: 2 },
        'http://localhost:3000',
      );

      expect(result).toEqual(mockResult);
      expect(global.fetch).toHaveBeenCalledWith(
        'http://localhost:3000/chunks/chunk-123/neighbourhood?depth=2',
        expect.objectContaining({
          method: 'GET',
          headers: {
            authorization: `Bearer test-session-token`,
            'x-workspace-id': 'test-workspace-id',
          },
        }),
      );
    });

    it('throws GetNeighbourhoodValidationError if the store rejects the request', async () => {
      global.fetch = vi.fn().mockResolvedValue({
        ok: false,
        status: 403,
        json: vi.fn().mockResolvedValue({ message: 'Store said no' }),
      });

      await expect(
        getNeighbourhood({ id: 'chunk-123' }, 'http://localhost:3000'),
      ).rejects.toThrow('Store said no');
    });

    it('falls back to a generic error message if store body is malformed', async () => {
      global.fetch = vi.fn().mockResolvedValue({
        ok: false,
        status: 400,
        json: vi.fn().mockRejectedValue(new Error('unparseable')),
      });

      await expect(
        getNeighbourhood({ id: 'chunk-123' }, 'http://localhost:3000'),
      ).rejects.toThrow('Store rejected get-neighbourhood request (400)');
    });
  });
});
