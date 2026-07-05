import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  captureChunk,
  CaptureChunkValidationError,
  parseCaptureChunkInput,
  type CaptureChunkInput,
  type CaptureChunkResult,
} from './capture-chunk.js';

describe('parseCaptureChunkInput', () => {
  it('accepts a well-formed body', () => {
    const body = {
      label: 'ATOMIC-1',
      content: 'content',
      discipline: 'product',
      chunkType: 'feature',
      contextKind: 'permanent',
      stakeholderId: 'stakeholder-1',
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
      stakeholderId: 'stakeholder-1',
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
      stakeholderId: 'stakeholder-1',
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
      stakeholderId: 'stakeholder-1',
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
      stakeholderId: 'stakeholder-1',
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
      stakeholderId: 'stakeholder-1',
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
    stakeholderId: 'stakeholder-1',
  };

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('forwards the input to POST {harnessUrl}/chunks and returns the created chunk', async () => {
    const expected: CaptureChunkResult = {
      id: 'chunk-1',
      label: input.label,
      content: input.content,
      discipline: input.discipline,
      chunkType: input.chunkType,
      contextKind: input.contextKind,
      status: 'draft',
      createdByStakeholderId: input.stakeholderId,
      updatedByStakeholderId: input.stakeholderId,
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

    const result = await captureChunk(input, 'http://harness.test');

    expect(fetchMock).toHaveBeenCalledWith('http://harness.test/chunks', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
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
      createdByStakeholderId: inputWithBranch.stakeholderId,
      updatedByStakeholderId: inputWithBranch.stakeholderId,
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

    const result = await captureChunk(inputWithBranch, 'http://harness.test');

    expect(fetchMock).toHaveBeenCalledWith('http://harness.test/chunks', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
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

    await expect(captureChunk(input, 'http://harness.test')).rejects.toMatchObject({
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

    await expect(captureChunk(input, 'http://harness.test')).rejects.toMatchObject({
      name: 'CaptureChunkValidationError',
      statusCode: 400,
      message: 'Store rejected capture-chunk request (400)',
    });
  });
});
