import { describe, expect, it } from 'vitest';
import { parseCreateArtifactRequest } from './create-artifact-request.dto.js';

describe('parseCreateArtifactRequest', () => {
  it('parses a valid body', () => {
    const request = parseCreateArtifactRequest({
      content: Buffer.from('hello').toString('base64'),
      mimeType: 'text/plain',
      stakeholderId: 'stakeholder-1',
    });

    expect(request).toEqual({
      content: Buffer.from('hello').toString('base64'),
      mimeType: 'text/plain',
      stakeholderId: 'stakeholder-1',
    });
  });

  it('throws BadRequestException for a non-object body', () => {
    expect(() => parseCreateArtifactRequest('not an object')).toThrow('JSON object');
  });

  it('throws BadRequestException for a missing content field', () => {
    expect(() =>
      parseCreateArtifactRequest({ mimeType: 'text/plain', stakeholderId: 'stakeholder-1' }),
    ).toThrow('content');
  });

  it('throws BadRequestException for a missing mimeType field', () => {
    expect(() =>
      parseCreateArtifactRequest({ content: 'aGVsbG8=', stakeholderId: 'stakeholder-1' }),
    ).toThrow('mimeType');
  });

  it('throws BadRequestException for a missing stakeholderId field', () => {
    expect(() =>
      parseCreateArtifactRequest({ content: 'aGVsbG8=', mimeType: 'text/plain' }),
    ).toThrow('stakeholderId');
  });

  it('throws BadRequestException for non-base64 content', () => {
    expect(() =>
      parseCreateArtifactRequest({
        content: 'not base64!!',
        mimeType: 'text/plain',
        stakeholderId: 'stakeholder-1',
      }),
    ).toThrow('base64');
  });
});
