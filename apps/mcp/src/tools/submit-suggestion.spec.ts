import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  submitSuggestion,
  SubmitSuggestionValidationError,
  parseSubmitSuggestionInput,
  type SubmitSuggestionInput,
  type SubmitSuggestionResult,
} from './submit-suggestion.js';

const STAKEHOLDER_ID = 'stakeholder-1';

import { resetStoreCredentialsForTests } from '../store-client.js';
describe('parseSubmitSuggestionInput', () => {
  const chunkBody = {
    discipline: 'product',
    label: 'IDEA-1',
    content: 'Some proposed content.',
  };

  const edgeBody = {
    discipline: 'product',
    fromChunkLabel: 'IDEA-1',
    toChunkLabel: 'IDEA-2',
    relationshipType: 'refines',
  };

  it('accepts a well-formed chunk-shaped body', () => {
    expect(parseSubmitSuggestionInput(chunkBody)).toEqual(chunkBody);
  });

  it('accepts a well-formed edge-shaped body', () => {
    expect(parseSubmitSuggestionInput(edgeBody)).toEqual(edgeBody);
  });

  it('rejects a non-object body', () => {
    expect(() => parseSubmitSuggestionInput('nope')).toThrow(SubmitSuggestionValidationError);
    expect(() => parseSubmitSuggestionInput(null)).toThrow(SubmitSuggestionValidationError);
  });

  it.each(['discipline'])('rejects a missing %s, never inventing one', (field) => {
    const body: Record<string, unknown> = { ...chunkBody };
    Reflect.deleteProperty(body, field);
    expect(() => parseSubmitSuggestionInput(body)).toThrow(new RegExp(field));
  });

  it('rejects a body mixing chunk and edge fields', () => {
    const body = { ...chunkBody, fromChunkLabel: 'IDEA-1' };
    expect(() => parseSubmitSuggestionInput(body)).toThrow(SubmitSuggestionValidationError);
  });

  it('rejects a body providing neither chunk nor edge fields', () => {
    const body = { discipline: 'product' };
    expect(() => parseSubmitSuggestionInput(body)).toThrow(SubmitSuggestionValidationError);
  });

  it('rejects a partial chunk shape (label only)', () => {
    const body: Record<string, unknown> = { ...chunkBody };
    delete body.content;
    expect(() => parseSubmitSuggestionInput(body)).toThrow(SubmitSuggestionValidationError);
  });

  it('rejects a partial edge shape (only fromChunkLabel)', () => {
    const body = {
      discipline: 'product',
      fromChunkLabel: 'IDEA-1',
    };
    expect(() => parseSubmitSuggestionInput(body)).toThrow(SubmitSuggestionValidationError);
  });

  it('does not itself validate the relationshipType/discipline vocabulary (defers to the store)', () => {
    const body = { ...edgeBody, relationshipType: 'not-a-real-type', discipline: 'not-a-real-discipline' };
    expect(parseSubmitSuggestionInput(body)).toEqual(body);
  });
});

describe('submitSuggestion', () => {
  const input: SubmitSuggestionInput = {
    discipline: 'product',
    label: 'IDEA-1',
    content: 'Some proposed content.',
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

  it('forwards the input to POST {storeUrl}/suggestions and returns the created suggestion', async () => {
    const expected: SubmitSuggestionResult = {
      id: 'suggestion-1',
      label: input.label ?? null,
      content: input.content ?? null,
      fromChunkLabel: null,
      toChunkLabel: null,
      relationshipType: null,
      discipline: input.discipline,
      status: 'pending',
      submittedByStakeholderId: STAKEHOLDER_ID,
      submittedByActorKind: 'delegated',
      decidedByStakeholderId: null,
      decidedAt: null,
      createdAt: '2026-01-01T00:00:00.000Z',
      updatedAt: '2026-01-01T00:00:00.000Z',
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: () => Promise.resolve(expected),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await submitSuggestion(input, 'http://store.test');

    expect(fetchMock).toHaveBeenCalledWith('http://store.test/suggestions', {
      method: 'POST',
      headers: {
        'content-type': 'application/json',
        authorization: 'Bearer test-session-token',
        'x-workspace-id': 'test-workspace-id',
      },
      body: JSON.stringify(input),
    });
    expect(result).toEqual(expected);
    expect(result.id).toBe('suggestion-1');
  });

  it('surfaces the store 400 validation error without swallowing it', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 400,
      json: () =>
        Promise.resolve({
          statusCode: 400,
          message: 'Invalid discipline: "not-a-real-discipline"',
          error: 'Bad Request',
        }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(submitSuggestion(input, 'http://store.test')).rejects.toMatchObject({
      name: 'SubmitSuggestionValidationError',
      statusCode: 400,
      message: 'Invalid discipline: "not-a-real-discipline"',
    });
  });

  it('falls back to a generic message when the store error body has no message', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 502,
      json: () => Promise.reject(new Error('not json')),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(submitSuggestion(input, 'http://store.test')).rejects.toMatchObject({
      name: 'SubmitSuggestionValidationError',
      statusCode: 502,
      message: 'Store rejected submit-suggestion request (502)',
    });
  });
});
