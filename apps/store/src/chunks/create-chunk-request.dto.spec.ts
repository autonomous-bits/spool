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
    stakeholderId: '00000000-0000-0000-0000-000000000001',
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
      stakeholderId: '00000000-0000-0000-0000-000000000001',
    });
  });

  it('rejects a non-object body', () => {
    expect(() => parseCreateChunkRequest('nope')).toThrow(BadRequestException);
    expect(() => parseCreateChunkRequest(null)).toThrow(BadRequestException);
  });

  it.each(['label', 'content', 'stakeholderId'])('rejects a missing %s', (field) => {
    const body = validBody({ [field]: undefined });
    expect(() => parseCreateChunkRequest(body)).toThrow(BadRequestException);
  });

  it.each(['label', 'content', 'stakeholderId'])('rejects a blank %s', (field) => {
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
});
