import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  submitVerificationSignal,
  SubmitVerificationSignalValidationError,
  parseSubmitVerificationSignalInput,
  type SubmitVerificationSignalInput,
  type SubmitVerificationSignalResult,
} from './submit-verification-signal.js';

import { resetStoreCredentialsForTests } from '../store-client.js';
describe('parseSubmitVerificationSignalInput', () => {
  const validBody = {
    branchId: 'branch-1',
    verifierName: 'ci-evaluator',
    status: 'pass',
    reason: 'Checks passed.',
  };

  it('accepts a well-formed body', () => {
    expect(parseSubmitVerificationSignalInput(validBody)).toEqual(validBody);
  });

  it('accepts an empty reason when provided', () => {
    expect(parseSubmitVerificationSignalInput({ ...validBody, reason: '' })).toEqual({
      ...validBody,
      reason: '',
    });
  });

  it('rejects a non-object body', () => {
    expect(() => parseSubmitVerificationSignalInput('nope')).toThrow(
      SubmitVerificationSignalValidationError,
    );
    expect(() => parseSubmitVerificationSignalInput(null)).toThrow(
      SubmitVerificationSignalValidationError,
    );
  });

  it.each(['branchId', 'verifierName', 'status'])(
    'rejects a missing or blank %s',
    (field) => {
      const missing: Record<string, unknown> = { ...validBody };
      Reflect.deleteProperty(missing, field);
      expect(() => parseSubmitVerificationSignalInput(missing)).toThrow(new RegExp(field));

      expect(() =>
        parseSubmitVerificationSignalInput({ ...validBody, [field]: '   ' }),
      ).toThrow(new RegExp(field));
    },
  );

  it('does not itself validate the status vocabulary (defers to the store)', () => {
    const body = { ...validBody, status: 'not-a-real-status' };
    expect(parseSubmitVerificationSignalInput(body)).toEqual(body);
  });

  it('rejects a non-string reason when provided', () => {
    expect(() => parseSubmitVerificationSignalInput({ ...validBody, reason: 42 })).toThrow(
      'reason must be a string when provided',
    );
  });
});

describe('submitVerificationSignal', () => {
  const input: SubmitVerificationSignalInput = {
    branchId: 'branch-1',
    verifierName: 'ci-evaluator',
    status: 'pass',
    reason: 'Checks passed.',
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

  it('forwards the input to POST {storeUrl}/branches/:branchId/verification-signals and returns the created signal', async () => {
    const expected: SubmitVerificationSignalResult = {
      id: 'signal-1',
      branchId: input.branchId,
      verifierName: input.verifierName,
      status: 'pass',
      reason: input.reason ?? null,
      createdAt: '2026-01-01T00:00:00.000Z',
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: () => Promise.resolve(expected),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await submitVerificationSignal(input, 'http://store.test');

    expect(fetchMock).toHaveBeenCalledWith(
      'http://store.test/branches/branch-1/verification-signals',
      {
        method: 'POST',
        headers: {
          'content-type': 'application/json',
          authorization: 'Bearer test-session-token',
          'x-workspace-id': 'test-workspace-id',
        },
        body: JSON.stringify({
          verifierName: input.verifierName,
          status: input.status,
          reason: input.reason,
        }),
      },
    );
    expect(result).toEqual(expected);
    expect(result.id).toBe('signal-1');
  });

  it('surfaces store 4xx errors without swallowing them', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 409,
      json: () =>
        Promise.resolve({
          statusCode: 409,
          message: 'Branch branch-1 is not reviewable',
          error: 'Conflict',
        }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(submitVerificationSignal(input, 'http://store.test')).rejects.toMatchObject({
      name: 'SubmitVerificationSignalValidationError',
      statusCode: 409,
      message: 'Branch branch-1 is not reviewable',
    });
  });

  it('surfaces the store 400 invalid-status error without swallowing it', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 400,
      json: () =>
        Promise.resolve({
          statusCode: 400,
          message: 'Invalid status: "not-a-real-status"',
          error: 'Bad Request',
        }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(
      submitVerificationSignal({ ...input, status: 'not-a-real-status' }, 'http://store.test'),
    ).rejects.toMatchObject({
      name: 'SubmitVerificationSignalValidationError',
      statusCode: 400,
      message: 'Invalid status: "not-a-real-status"',
    });
  });

  it('falls back to a generic message when the store error body has no message', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 502,
      json: () => Promise.reject(new Error('not json')),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(submitVerificationSignal(input, 'http://store.test')).rejects.toMatchObject({
      name: 'SubmitVerificationSignalValidationError',
      statusCode: 502,
      message: 'Store rejected submit-verification-signal request (502)',
    });
  });
});
