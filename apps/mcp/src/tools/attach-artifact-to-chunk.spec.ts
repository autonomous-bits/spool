import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  attachArtifactToChunk,
  AttachArtifactToChunkValidationError,
  parseAttachArtifactToChunkInput,
  type AttachArtifactToChunkInput,
  type AttachArtifactToChunkResult,
} from './attach-artifact-to-chunk.js';

import { resetStoreCredentialsForTests } from '../store-client.js';
describe('parseAttachArtifactToChunkInput', () => {
  const validBody = {
    chunkLabel: 'IDEA-1',
    artifactId: 'artifact-1',
  };

  it('accepts a well-formed body without branchId', () => {
    expect(parseAttachArtifactToChunkInput(validBody)).toEqual(validBody);
  });

  it('accepts a well-formed body with branchId', () => {
    const body = { ...validBody, branchId: 'branch-1' };
    expect(parseAttachArtifactToChunkInput(body)).toEqual(body);
  });

  it('rejects a non-object body', () => {
    expect(() => parseAttachArtifactToChunkInput('nope')).toThrow(
      AttachArtifactToChunkValidationError,
    );
    expect(() => parseAttachArtifactToChunkInput(null)).toThrow(
      AttachArtifactToChunkValidationError,
    );
  });

  it.each(['chunkLabel', 'artifactId'])('rejects a missing %s, never inventing one', (field) => {
    const body: Record<string, unknown> = { ...validBody };
    Reflect.deleteProperty(body, field);
    expect(() => parseAttachArtifactToChunkInput(body)).toThrow(new RegExp(field));
  });

  it('rejects a blank required field', () => {
    const body = { ...validBody, artifactId: '   ' };
    expect(() => parseAttachArtifactToChunkInput(body)).toThrow(/artifactId/);
  });

  it('rejects a blank branchId when provided', () => {
    const body = { ...validBody, branchId: '   ' };
    expect(() => parseAttachArtifactToChunkInput(body)).toThrow(/branchId/);
  });
});

describe('attachArtifactToChunk', () => {
  const input: AttachArtifactToChunkInput = {
    chunkLabel: 'IDEA-1',
    artifactId: 'artifact-1',
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

  it('forwards the input to POST {storeUrl}/chunks/:label/artifacts and returns the created association', async () => {
    const expected: AttachArtifactToChunkResult = {
      id: 'assoc-1',
      chunkLabel: input.chunkLabel,
      artifactId: input.artifactId,
      status: 'active',
      branchId: null,
      originBranchId: null,
      createdByStakeholderId: 'stakeholder-1',
      updatedByStakeholderId: 'stakeholder-1',
      createdAt: '2026-01-01T00:00:00.000Z',
      updatedAt: '2026-01-01T00:00:00.000Z',
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: () => Promise.resolve(expected),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await attachArtifactToChunk(input, 'http://store.test');

    expect(fetchMock).toHaveBeenCalledWith('http://store.test/chunks/IDEA-1/artifacts', {
      method: 'POST',
      headers: {
        'content-type': 'application/json',
        authorization: 'Bearer test-session-token',
        'x-workspace-id': 'test-workspace-id',
      },
      body: JSON.stringify({ artifactId: input.artifactId }),
    });
    expect(result).toEqual(expected);
    expect(result.id).toBe('assoc-1');
  });

  it('forwards branchId when provided and URL-encodes the chunk label', async () => {
    const withBranch: AttachArtifactToChunkInput = {
      ...input,
      chunkLabel: 'IDEA/1',
      branchId: 'branch-1',
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: () =>
        Promise.resolve({
          id: 'assoc-2',
          chunkLabel: withBranch.chunkLabel,
          artifactId: withBranch.artifactId,
          status: 'active',
          branchId: withBranch.branchId,
          originBranchId: withBranch.branchId,
          createdByStakeholderId: 'stakeholder-1',
          updatedByStakeholderId: 'stakeholder-1',
          createdAt: '2026-01-01T00:00:00.000Z',
          updatedAt: '2026-01-01T00:00:00.000Z',
        }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await attachArtifactToChunk(withBranch, 'http://store.test');

    expect(fetchMock).toHaveBeenCalledWith('http://store.test/chunks/IDEA%2F1/artifacts', {
      method: 'POST',
      headers: {
        'content-type': 'application/json',
        authorization: 'Bearer test-session-token',
        'x-workspace-id': 'test-workspace-id',
      },
      body: JSON.stringify({
        artifactId: withBranch.artifactId,
        branchId: withBranch.branchId,
      }),
    });
  });

  it('surfaces the store 404 error without swallowing it', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 404,
      json: () =>
        Promise.resolve({
          statusCode: 404,
          message: 'Chunk with label IDEA-1 not found in this scope',
          error: 'Not Found',
        }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(attachArtifactToChunk(input, 'http://store.test')).rejects.toMatchObject({
      name: 'AttachArtifactToChunkValidationError',
      statusCode: 404,
      message: 'Chunk with label IDEA-1 not found in this scope',
    });
  });

  it('falls back to a generic message when the store error body has no message', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 502,
      json: () => Promise.reject(new Error('not json')),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(attachArtifactToChunk(input, 'http://store.test')).rejects.toMatchObject({
      name: 'AttachArtifactToChunkValidationError',
      statusCode: 502,
      message: 'Store rejected attach-artifact-to-chunk request (502)',
    });
  });
});
