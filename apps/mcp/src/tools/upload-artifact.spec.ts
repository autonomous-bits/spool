import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  MAX_ARTIFACT_CONTENT_BYTES,
  parseUploadArtifactInput,
  uploadArtifact,
  UploadArtifactValidationError,
  type UploadArtifactInput,
  type UploadArtifactResult,
} from './upload-artifact.js';

import { resetStoreCredentialsForTests } from '../store-client.js';
describe('parseUploadArtifactInput', () => {
  const validBody = {
    content: Buffer.from('hello world').toString('base64'),
    mimeType: 'text/plain',
  };

  it('accepts a well-formed body', () => {
    expect(parseUploadArtifactInput(validBody)).toEqual(validBody);
  });

  it('rejects a non-object body', () => {
    expect(() => parseUploadArtifactInput('nope')).toThrow(UploadArtifactValidationError);
    expect(() => parseUploadArtifactInput(null)).toThrow(UploadArtifactValidationError);
  });

  it.each(['content', 'mimeType'])('rejects a missing %s, never inventing one', (field) => {
    const body: Record<string, unknown> = { ...validBody };
    Reflect.deleteProperty(body, field);
    expect(() => parseUploadArtifactInput(body)).toThrow(new RegExp(field));
  });

  it('rejects a blank required field', () => {
    const body = { ...validBody, mimeType: '   ' };
    expect(() => parseUploadArtifactInput(body)).toThrow(/mimeType/);
  });

  it('rejects malformed (non-base64) content', () => {
    const body = { ...validBody, content: 'not-base64!!' };
    expect(() => parseUploadArtifactInput(body)).toThrow(/base64/);
  });

  it('does not itself validate the mimeType format (defers to the store)', () => {
    const body = { ...validBody, mimeType: 'not-a-real-mime-type' };
    expect(parseUploadArtifactInput(body)).toEqual(body);
  });

  it('rejects decoded content exceeding the max artifact size, without crashing', () => {
    const oversized = Buffer.alloc(MAX_ARTIFACT_CONTENT_BYTES + 1, 'a').toString('base64');
    const body = { ...validBody, content: oversized };
    expect(() => parseUploadArtifactInput(body)).toThrow(UploadArtifactValidationError);
    expect(() => parseUploadArtifactInput(body)).toThrow(/exceeds the maximum artifact size/);
  });

  it('accepts decoded content exactly at the max artifact size', () => {
    const atLimit = Buffer.alloc(MAX_ARTIFACT_CONTENT_BYTES, 'a').toString('base64');
    const body = { ...validBody, content: atLimit };
    expect(() => parseUploadArtifactInput(body)).not.toThrow();
  });
});

describe('uploadArtifact', () => {
  const input: UploadArtifactInput = {
    content: Buffer.from('hello world').toString('base64'),
    mimeType: 'text/plain',
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

  it('forwards the input to POST {storeUrl}/artifacts and returns the created artifact id', async () => {
    const expected: UploadArtifactResult = {
      id: 'artifact-1',
      uri: 'file:///artifacts/artifact-1',
      mimeType: input.mimeType,
      createdByStakeholderId: 'stakeholder-1',
      createdAt: '2026-01-01T00:00:00.000Z',
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: () => Promise.resolve(expected),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await uploadArtifact(input, 'http://store.test');

    expect(fetchMock).toHaveBeenCalledWith('http://store.test/artifacts', {
      method: 'POST',
      headers: {
        'content-type': 'application/json',
        authorization: 'Bearer test-session-token',
        'x-workspace-id': 'test-workspace-id',
      },
      body: JSON.stringify(input),
    });
    expect(result).toEqual(expected);
    expect(result.id).toBe('artifact-1');
  });

  it('surfaces the store 400 validation error without swallowing it', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 400,
      json: () =>
        Promise.resolve({
          statusCode: 400,
          message: 'Unknown stakeholderId: stakeholder-1',
          error: 'Bad Request',
        }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(uploadArtifact(input, 'http://store.test')).rejects.toMatchObject({
      name: 'UploadArtifactValidationError',
      statusCode: 400,
      message: 'Unknown stakeholderId: stakeholder-1',
    });
  });

  it('falls back to a generic message when the store error body has no message', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 502,
      json: () => Promise.reject(new Error('not json')),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(uploadArtifact(input, 'http://store.test')).rejects.toMatchObject({
      name: 'UploadArtifactValidationError',
      statusCode: 502,
      message: 'Store rejected upload-artifact request (502)',
    });
  });
});
