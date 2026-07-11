import { BadRequestException } from '@nestjs/common';
import { describe, expect, it } from 'vitest';
import { parseCreateBranchRequest } from './create-branch-request.dto.js';

function validBody(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    name: 'feature-branch',
    discipline: 'product',
    ...overrides,
  };
}

describe('parseCreateBranchRequest', () => {
  it('parses a valid body', () => {
    const parsed = parseCreateBranchRequest(validBody());

    expect(parsed).toEqual({
      name: 'feature-branch',
      discipline: 'product',
    });
  });

  it('ignores a client-supplied stakeholderId field (authorship is claim-derived)', () => {
    const parsed = parseCreateBranchRequest(validBody({ stakeholderId: 'client-supplied-id' }));

    expect(parsed).not.toHaveProperty('stakeholderId');
  });

  it('rejects a non-object body', () => {
    expect(() => parseCreateBranchRequest('nope')).toThrow(BadRequestException);
    expect(() => parseCreateBranchRequest(null)).toThrow(BadRequestException);
  });

  it.each(['name'])('rejects a missing %s', (field) => {
    const body = validBody({ [field]: undefined });
    expect(() => parseCreateBranchRequest(body)).toThrow(BadRequestException);
  });

  it.each(['name'])('rejects a blank %s', (field) => {
    const body = validBody({ [field]: '   ' });
    expect(() => parseCreateBranchRequest(body)).toThrow(BadRequestException);
  });

  it('rejects an invalid discipline', () => {
    expect(() => parseCreateBranchRequest(validBody({ discipline: 'bogus' }))).toThrow(
      BadRequestException,
    );
  });
});
