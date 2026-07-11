import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  createEdge,
  CreateEdgeValidationError,
  parseCreateEdgeInput,
  type CreateEdgeInput,
  type CreateEdgeResult,
} from './create-edge.js';

describe('parseCreateEdgeInput', () => {
  const validBody = {
    fromChunkLabel: 'IDEA-1',
    toChunkLabel: 'IDEA-2',
    type: 'refines',
    discipline: 'product',
    stakeholderId: 'stakeholder-1',
    workspaceId: 'workspace-1',
  };

  it('accepts a well-formed body without branchId', () => {
    expect(parseCreateEdgeInput(validBody)).toEqual(validBody);
  });

  it('accepts a well-formed body with branchId', () => {
    const body = { ...validBody, branchId: 'branch-1' };
    expect(parseCreateEdgeInput(body)).toEqual(body);
  });

  it('rejects a non-object body', () => {
    expect(() => parseCreateEdgeInput('nope')).toThrow(CreateEdgeValidationError);
    expect(() => parseCreateEdgeInput(null)).toThrow(CreateEdgeValidationError);
  });

  it.each(['fromChunkLabel', 'toChunkLabel', 'type', 'discipline', 'stakeholderId', 'workspaceId'])(
    'rejects a missing %s, never inventing one',
    (field) => {
      const body: Record<string, unknown> = { ...validBody };
      Reflect.deleteProperty(body, field);
      expect(() => parseCreateEdgeInput(body)).toThrow(new RegExp(field));
    },
  );

  it('rejects a blank required field', () => {
    const body = { ...validBody, fromChunkLabel: '   ' };
    expect(() => parseCreateEdgeInput(body)).toThrow(/fromChunkLabel/);
  });

  it('rejects a blank branchId when provided', () => {
    const body = { ...validBody, branchId: '   ' };
    expect(() => parseCreateEdgeInput(body)).toThrow(/branchId/);
  });

  it('does not itself validate the type/discipline vocabulary (defers to the store)', () => {
    const body = { ...validBody, type: 'not-a-real-type', discipline: 'not-a-real-discipline' };
    expect(parseCreateEdgeInput(body)).toEqual(body);
  });
});

describe('createEdge', () => {
  const input: CreateEdgeInput = {
    fromChunkLabel: 'IDEA-1',
    toChunkLabel: 'IDEA-2',
    type: 'refines',
    discipline: 'product',
    stakeholderId: 'stakeholder-1',
    workspaceId: 'workspace-1',
  };

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('forwards the input to POST {harnessUrl}/edges and returns the created edge', async () => {
    const expected: CreateEdgeResult = {
      id: 'edge-1',
      fromChunkLabel: input.fromChunkLabel,
      toChunkLabel: input.toChunkLabel,
      type: input.type,
      status: 'active',
      discipline: input.discipline,
      branchId: null,
      originBranchId: null,
      supersededByEdgeId: null,
      createdByStakeholderId: input.stakeholderId,
      updatedByStakeholderId: input.stakeholderId,
      createdAt: '2026-01-01T00:00:00.000Z',
      updatedAt: '2026-01-01T00:00:00.000Z',
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: () => Promise.resolve(expected),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await createEdge(input, 'http://harness.test');

    const { workspaceId, ...expectedBody } = input;
    expect(fetchMock).toHaveBeenCalledWith('http://harness.test/edges', {
      method: 'POST',
      headers: { 'content-type': 'application/json', 'x-workspace-id': workspaceId },
      body: JSON.stringify(expectedBody),
    });
    expect(result).toEqual(expected);
    expect(result.id).toBe('edge-1');
  });

  it('surfaces the store 400 validation error without swallowing it', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 400,
      json: () =>
        Promise.resolve({
          statusCode: 400,
          message: 'Invalid type: "not-a-real-type"',
          error: 'Bad Request',
        }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(createEdge(input, 'http://harness.test')).rejects.toMatchObject({
      name: 'CreateEdgeValidationError',
      statusCode: 400,
      message: 'Invalid type: "not-a-real-type"',
    });
  });

  it('surfaces the store 409 conflict error without swallowing it', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 409,
      json: () =>
        Promise.resolve({
          statusCode: 409,
          message: 'An active edge already exists for this from/to/type/branch scope',
          error: 'Conflict',
        }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(createEdge(input, 'http://harness.test')).rejects.toMatchObject({
      name: 'CreateEdgeValidationError',
      statusCode: 409,
    });
  });

  it('falls back to a generic message when the store error body has no message', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 404,
      json: () => Promise.reject(new Error('not json')),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(createEdge(input, 'http://harness.test')).rejects.toMatchObject({
      name: 'CreateEdgeValidationError',
      statusCode: 404,
      message: 'Store rejected create-edge request (404)',
    });
  });
});
