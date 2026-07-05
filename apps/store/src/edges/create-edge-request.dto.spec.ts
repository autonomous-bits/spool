import { BadRequestException } from '@nestjs/common';
import { describe, expect, it } from 'vitest';
import { parseCreateEdgeRequest } from './create-edge-request.dto.js';

function validBody(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    fromChunkLabel: 'ATOMIC-1',
    toChunkLabel: 'ATOMIC-2',
    type: 'refines',
    discipline: 'product',
    stakeholderId: '00000000-0000-0000-0000-000000000001',
    ...overrides,
  };
}

describe('parseCreateEdgeRequest', () => {
  it('parses a valid body', () => {
    const parsed = parseCreateEdgeRequest(validBody());

    expect(parsed).toEqual({
      fromChunkLabel: 'ATOMIC-1',
      toChunkLabel: 'ATOMIC-2',
      type: 'refines',
      discipline: 'product',
      stakeholderId: '00000000-0000-0000-0000-000000000001',
    });
  });

  it('rejects a non-object body', () => {
    expect(() => parseCreateEdgeRequest('nope')).toThrow(BadRequestException);
    expect(() => parseCreateEdgeRequest(null)).toThrow(BadRequestException);
  });

  it.each(['fromChunkLabel', 'toChunkLabel', 'stakeholderId'])('rejects a missing %s', (field) => {
    const body = validBody({ [field]: undefined });
    expect(() => parseCreateEdgeRequest(body)).toThrow(BadRequestException);
  });

  it.each(['fromChunkLabel', 'toChunkLabel', 'stakeholderId'])('rejects a blank %s', (field) => {
    const body = validBody({ [field]: '   ' });
    expect(() => parseCreateEdgeRequest(body)).toThrow(BadRequestException);
  });

  it('rejects fromChunkLabel === toChunkLabel', () => {
    expect(() =>
      parseCreateEdgeRequest(validBody({ fromChunkLabel: 'SAME', toChunkLabel: 'SAME' })),
    ).toThrow(BadRequestException);
  });

  it('rejects an invalid type', () => {
    expect(() => parseCreateEdgeRequest(validBody({ type: 'bogus' }))).toThrow(
      BadRequestException,
    );
  });

  it('rejects an invalid discipline', () => {
    expect(() => parseCreateEdgeRequest(validBody({ discipline: 'bogus' }))).toThrow(
      BadRequestException,
    );
  });

  it('parses an optional branchId when present', () => {
    const parsed = parseCreateEdgeRequest(
      validBody({ branchId: '00000000-0000-0000-0000-0000000000b1' }),
    );

    expect(parsed.branchId).toBe('00000000-0000-0000-0000-0000000000b1');
  });

  it('omits branchId when absent', () => {
    const parsed = parseCreateEdgeRequest(validBody());

    expect(parsed.branchId).toBeUndefined();
  });

  it.each(['', '   '])('rejects a blank branchId %j', (branchId) => {
    expect(() => parseCreateEdgeRequest(validBody({ branchId }))).toThrow(BadRequestException);
  });

  it('rejects a non-string branchId', () => {
    expect(() => parseCreateEdgeRequest(validBody({ branchId: 42 }))).toThrow(
      BadRequestException,
    );
  });
});
