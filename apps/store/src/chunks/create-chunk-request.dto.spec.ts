import { BadRequestException } from '@nestjs/common';
import { describe, expect, it } from 'vitest';
import { parseCreateChunkRequest } from './create-chunk-request.dto.js';

function validBody(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    label: 'ATOMIC-1',
    content: 'A raw captured idea.',
    discipline: 'product',
    chunkType: 'feature',
    contextKind: 'permanent',
    ...overrides,
  };
}

describe('parseCreateChunkRequest', () => {
  it('parses a valid body', () => {
    const parsed = parseCreateChunkRequest(validBody());

    expect(parsed).toEqual({
      label: 'ATOMIC-1',
      content: 'A raw captured idea.',
      discipline: 'product',
      chunkType: 'feature',
      contextKind: 'permanent',
    });
  });

  it('ignores a client-supplied stakeholderId field (authorship is claim-derived)', () => {
    const parsed = parseCreateChunkRequest(validBody({ stakeholderId: 'client-supplied-id' }));

    expect(parsed).not.toHaveProperty('stakeholderId');
  });

  it('rejects a non-object body', () => {
    expect(() => parseCreateChunkRequest('nope')).toThrow(BadRequestException);
    expect(() => parseCreateChunkRequest(null)).toThrow(BadRequestException);
  });

  it.each(['label', 'content'])('rejects a missing %s', (field) => {
    const body = validBody({ [field]: undefined });
    expect(() => parseCreateChunkRequest(body)).toThrow(BadRequestException);
  });

  it.each(['label', 'content'])('rejects a blank %s', (field) => {
    const body = validBody({ [field]: '   ' });
    expect(() => parseCreateChunkRequest(body)).toThrow(BadRequestException);
  });

  it('rejects an invalid discipline', () => {
    expect(() => parseCreateChunkRequest(validBody({ discipline: 'bogus' }))).toThrow(
      BadRequestException,
    );
  });

  it('rejects an invalid chunkType', () => {
    expect(() => parseCreateChunkRequest(validBody({ chunkType: 'bogus' }))).toThrow(
      BadRequestException,
    );
  });

  it('rejects an invalid contextKind', () => {
    expect(() => parseCreateChunkRequest(validBody({ contextKind: 'bogus' }))).toThrow(
      BadRequestException,
    );
  });

  it('parses an optional branchId when present', () => {
    const parsed = parseCreateChunkRequest(
      validBody({ branchId: '00000000-0000-0000-0000-0000000000b1' }),
    );

    expect(parsed.branchId).toBe('00000000-0000-0000-0000-0000000000b1');
  });

  it('omits branchId when absent', () => {
    const parsed = parseCreateChunkRequest(validBody());

    expect(parsed.branchId).toBeUndefined();
  });

  it.each(['', '   '])('rejects a blank branchId %j', (branchId) => {
    expect(() => parseCreateChunkRequest(validBody({ branchId }))).toThrow(BadRequestException);
  });

  it('rejects a non-string branchId', () => {
    expect(() => parseCreateChunkRequest(validBody({ branchId: 42 }))).toThrow(
      BadRequestException,
    );
  });
});
