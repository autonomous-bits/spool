import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  captureChunk,
  CaptureChunkValidationError,
  parseCaptureChunkInput,
  type CaptureChunkInput,
  type CaptureChunkResult,
} from './capture-chunk.js';

import { resetStoreCredentialsForTests } from '../store-client.js';
describe('parseCaptureChunkInput', () => {
  it('accepts a well-formed body', () => {
    const body = {
      label: 'ATOMIC-1',
      content: 'content',
      discipline: 'product',
      chunkType: 'feature',
      contextKind: 'permanent',
    };

    expect(parseCaptureChunkInput(body)).toEqual(body);
  });

  it('rejects a non-object body', () => {
    expect(() => parseCaptureChunkInput('nope')).toThrow(CaptureChunkValidationError);
    expect(() => parseCaptureChunkInput(null)).toThrow(CaptureChunkValidationError);
  });

  it('rejects a missing required field', () => {
    const body = {
      content: 'content',
      discipline: 'product',
      chunkType: 'feature',
      contextKind: 'permanent',
    };

    expect(() => parseCaptureChunkInput(body)).toThrow(/label/);
  });

  it('rejects a blank required field', () => {
    const body = {
      label: '   ',
      content: 'content',
      discipline: 'product',
      chunkType: 'feature',
      contextKind: 'permanent',
    };

    expect(() => parseCaptureChunkInput(body)).toThrow(/label/);
  });

  it('accepts an optional branchId and forwards it as-is', () => {
    const body = {
      label: 'ATOMIC-1',
      content: 'content',
      discipline: 'product',
      chunkType: 'feature',
      contextKind: 'permanent',
      branchId: 'branch-1',
    };

    expect(parseCaptureChunkInput(body)).toEqual(body);
  });

  it('omits branchId entirely when absent, preserving the branchless path', () => {
    const body = {
      label: 'ATOMIC-1',
      content: 'content',
      discipline: 'product',
      chunkType: 'feature',
      contextKind: 'permanent',
    };

    expect(parseCaptureChunkInput(body)).not.toHaveProperty('branchId');
  });

  it('rejects a blank branchId when present', () => {
    const body = {
      label: 'ATOMIC-1',
      content: 'content',
      discipline: 'product',
      chunkType: 'feature',
      contextKind: 'permanent',
      branchId: '   ',
    };

    expect(() => parseCaptureChunkInput(body)).toThrow(/branchId/);
  });
});

describe('captureChunk', () => {
  const input: CaptureChunkInput = {
    label: 'ATOMIC-1',
    content: 'content',
    discipline: 'product',
    chunkType: 'feature',
    contextKind: 'permanent',
  };

  beforeEach(() => {
    vi.stubEnv('SPOOL_SESSION_TOKEN', 'test-session-token');
    vi.stubEnv('SPOOL_WORKSPACE_ID', 'test-workspace-id');
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.unstubAllEnvs();
    resetStoreCredentialsForTests();
  });

  it('forwards the input to POST {storeUrl}/chunks and returns the created chunk', async () => {
    const expected: CaptureChunkResult = {
      id: 'chunk-1',
      label: input.label,
      content: input.content,
      discipline: input.discipline,
      chunkType: input.chunkType,
      contextKind: input.contextKind,
      status: 'draft',
      createdByStakeholderId: 'stakeholder-1',
      updatedByStakeholderId: 'stakeholder-1',
      createdAt: '2026-01-01T00:00:00.000Z',
      updatedAt: '2026-01-01T00:00:00.000Z',
      branchId: null,
      originBranchId: null,
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: () => Promise.resolve(expected),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await captureChunk(input, 'http://store.test');

    expect(fetchMock).toHaveBeenCalledWith('http://store.test/chunks', {
      method: 'POST',
      headers: {
        'content-type': 'application/json',
        authorization: 'Bearer test-session-token',
        'x-workspace-id': 'test-workspace-id',
      },
      body: JSON.stringify(input),
    });
    expect(result).toEqual(expected);
    expect(result.id).toBe('chunk-1');
  });

  it('forwards an optional branchId as-is and reflects it in the result', async () => {
    const inputWithBranch: CaptureChunkInput = { ...input, branchId: 'branch-1' };
    const expected: CaptureChunkResult = {
      id: 'chunk-2',
      label: inputWithBranch.label,
      content: inputWithBranch.content,
      discipline: inputWithBranch.discipline,
      chunkType: inputWithBranch.chunkType,
      contextKind: inputWithBranch.contextKind,
      status: 'draft',
      createdByStakeholderId: 'stakeholder-1',
      updatedByStakeholderId: 'stakeholder-1',
      createdAt: '2026-01-01T00:00:00.000Z',
      updatedAt: '2026-01-01T00:00:00.000Z',
      branchId: 'branch-1',
      originBranchId: 'branch-1',
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: () => Promise.resolve(expected),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await captureChunk(inputWithBranch, 'http://store.test');

    expect(fetchMock).toHaveBeenCalledWith('http://store.test/chunks', {
      method: 'POST',
      headers: {
        'content-type': 'application/json',
        authorization: 'Bearer test-session-token',
        'x-workspace-id': 'test-workspace-id',
      },
      body: JSON.stringify(inputWithBranch),
    });
    expect(result.branchId).toBe('branch-1');
    expect(result.originBranchId).toBe('branch-1');
  });

  it('surfaces the store 400 validation error without swallowing it', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 400,
      json: () =>
        Promise.resolve({
          statusCode: 400,
          message: 'Invalid discipline: "not-a-discipline"',
          error: 'Bad Request',
        }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(captureChunk(input, 'http://store.test')).rejects.toMatchObject({
      name: 'CaptureChunkValidationError',
      statusCode: 400,
      message: 'Invalid discipline: "not-a-discipline"',
    });
  });

  it('falls back to a generic message when the store error body has no message', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 400,
      json: () => Promise.reject(new Error('not json')),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(captureChunk(input, 'http://store.test')).rejects.toMatchObject({
      name: 'CaptureChunkValidationError',
      statusCode: 400,
      message: 'Store rejected capture-chunk request (400)',
    });
  });
});
