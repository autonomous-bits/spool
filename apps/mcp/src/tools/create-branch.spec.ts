import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  createBranch,
  CreateBranchValidationError,
  parseCreateBranchInput,
  type CreateBranchInput,
  type CreateBranchResult,
} from './create-branch.js';

describe('parseCreateBranchInput', () => {
  it('accepts a well-formed body', () => {
    const body = {
      name: 'feature/foo',
      discipline: 'product',
      stakeholderId: 'stakeholder-1',
      workspaceId: 'workspace-1',
    };

    expect(parseCreateBranchInput(body)).toEqual(body);
  });

  it('rejects a non-object body', () => {
    expect(() => parseCreateBranchInput('nope')).toThrow(CreateBranchValidationError);
    expect(() => parseCreateBranchInput(null)).toThrow(CreateBranchValidationError);
  });

  it('rejects a missing required field', () => {
    const body = {
      discipline: 'product',
      stakeholderId: 'stakeholder-1',
      workspaceId: 'workspace-1',
    };

    expect(() => parseCreateBranchInput(body)).toThrow(/name/);
  });

  it('rejects a blank required field', () => {
    const body = {
      name: '   ',
      discipline: 'product',
      stakeholderId: 'stakeholder-1',
      workspaceId: 'workspace-1',
    };

    expect(() => parseCreateBranchInput(body)).toThrow(/name/);
  });

  it('rejects a missing stakeholderId, never inventing one', () => {
    const body = {
      name: 'feature/foo',
      discipline: 'product',
      workspaceId: 'workspace-1',
    };

    expect(() => parseCreateBranchInput(body)).toThrow(/stakeholderId/);
  });

  it('rejects a missing workspaceId, never inventing one', () => {
    const body = {
      name: 'feature/foo',
      discipline: 'product',
      stakeholderId: 'stakeholder-1',
    };

    expect(() => parseCreateBranchInput(body)).toThrow(/workspaceId/);
  });
});

describe('createBranch', () => {
  const input: CreateBranchInput = {
    name: 'feature/foo',
    discipline: 'product',
    stakeholderId: 'stakeholder-1',
    workspaceId: 'workspace-1',
  };

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('forwards the input to POST {storeUrl}/branches and returns the created branch', async () => {
    const expected: CreateBranchResult = {
      id: 'branch-1',
      name: input.name,
      discipline: input.discipline,
      status: 'draft',
      divergedAt: '2026-01-01T00:00:00.000Z',
      createdAt: '2026-01-01T00:00:00.000Z',
      updatedAt: '2026-01-01T00:00:00.000Z',
      createdByStakeholderId: input.stakeholderId,
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: () => Promise.resolve(expected),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await createBranch(input, 'http://store.test');

    const { workspaceId, ...expectedBody } = input;
    expect(fetchMock).toHaveBeenCalledWith('http://store.test/branches', {
      method: 'POST',
      headers: { 'content-type': 'application/json', 'x-workspace-id': workspaceId },
      body: JSON.stringify(expectedBody),
    });
    expect(result).toEqual(expected);
    expect(result.id).toBe('branch-1');
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

    await expect(createBranch(input, 'http://store.test')).rejects.toMatchObject({
      name: 'CreateBranchValidationError',
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

    await expect(createBranch(input, 'http://store.test')).rejects.toMatchObject({
      name: 'CreateBranchValidationError',
      statusCode: 400,
      message: 'Store rejected create-branch request (400)',
    });
  });
});
