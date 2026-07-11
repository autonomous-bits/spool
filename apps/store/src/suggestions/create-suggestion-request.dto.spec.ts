import { BadRequestException } from '@nestjs/common';
import { describe, expect, it } from 'vitest';
import { parseCreateSuggestionRequest } from './create-suggestion-request.dto.js';

function chunkBody(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    label: 'ATOMIC-1',
    content: 'Some proposed content.',
    discipline: 'product',
    ...overrides,
  };
}

function edgeBody(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    fromChunkLabel: 'ATOMIC-1',
    toChunkLabel: 'ATOMIC-2',
    relationshipType: 'refines',
    discipline: 'product',
    ...overrides,
  };
}

describe('parseCreateSuggestionRequest', () => {
  it('parses a valid chunk-shaped body', () => {
    const parsed = parseCreateSuggestionRequest(chunkBody());

    expect(parsed).toEqual({
      variant: { kind: 'chunk', label: 'ATOMIC-1', content: 'Some proposed content.' },
      discipline: 'product',
    });
  });

  it('parses a valid edge-shaped body', () => {
    const parsed = parseCreateSuggestionRequest(edgeBody());

    expect(parsed).toEqual({
      variant: {
        kind: 'edge',
        fromChunkLabel: 'ATOMIC-1',
        toChunkLabel: 'ATOMIC-2',
        relationshipType: 'refines',
      },
      discipline: 'product',
    });
  });

  it('rejects a non-object body', () => {
    expect(() => parseCreateSuggestionRequest('nope')).toThrow(BadRequestException);
    expect(() => parseCreateSuggestionRequest(null)).toThrow(BadRequestException);
  });

  it('ignores a client-supplied stakeholderId field', () => {
    const parsed = parseCreateSuggestionRequest(
      chunkBody({ stakeholderId: '00000000-0000-0000-0000-000000000001' }),
    );

    expect(parsed).toEqual({
      variant: { kind: 'chunk', label: 'ATOMIC-1', content: 'Some proposed content.' },
      discipline: 'product',
    });
  });

  it('rejects an invalid discipline', () => {
    expect(() => parseCreateSuggestionRequest(chunkBody({ discipline: 'bogus' }))).toThrow(
      BadRequestException,
    );
  });

  it('rejects a body mixing chunk and edge fields', () => {
    const body = { ...chunkBody(), fromChunkLabel: 'ATOMIC-1' };
    expect(() => parseCreateSuggestionRequest(body)).toThrow(BadRequestException);
  });

  it('rejects a body providing neither chunk nor edge fields', () => {
    const body = {
      discipline: 'product',
    };
    expect(() => parseCreateSuggestionRequest(body)).toThrow(BadRequestException);
  });

  it('rejects a partial chunk shape (label only)', () => {
    const body = chunkBody({ content: undefined });
    expect(() => parseCreateSuggestionRequest(body)).toThrow(BadRequestException);
  });

  it('rejects a partial chunk shape (content only)', () => {
    const body = chunkBody({ label: undefined });
    expect(() => parseCreateSuggestionRequest(body)).toThrow(BadRequestException);
  });

  it('rejects a partial edge shape (only fromChunkLabel)', () => {
    const body = {
      fromChunkLabel: 'ATOMIC-1',
      discipline: 'product',
    };
    expect(() => parseCreateSuggestionRequest(body)).toThrow(BadRequestException);
  });

  it('rejects a partial edge shape (missing relationshipType)', () => {
    const body = edgeBody({ relationshipType: undefined });
    expect(() => parseCreateSuggestionRequest(body)).toThrow(BadRequestException);
  });

  it('rejects an invalid relationshipType', () => {
    expect(() => parseCreateSuggestionRequest(edgeBody({ relationshipType: 'bogus' }))).toThrow(
      BadRequestException,
    );
  });

  it('rejects fromChunkLabel === toChunkLabel', () => {
    expect(() =>
      parseCreateSuggestionRequest(edgeBody({ fromChunkLabel: 'SAME', toChunkLabel: 'SAME' })),
    ).toThrow(BadRequestException);
  });

  it.each(['label', 'content'])('rejects a blank %s in a chunk body', (field) => {
    expect(() => parseCreateSuggestionRequest(chunkBody({ [field]: '   ' }))).toThrow(
      BadRequestException,
    );
  });

  it.each(['fromChunkLabel', 'toChunkLabel'])('rejects a blank %s in an edge body', (field) => {
    expect(() => parseCreateSuggestionRequest(edgeBody({ [field]: '   ' }))).toThrow(
      BadRequestException,
    );
  });
});
