import { describe, expect, it } from 'vitest';
import { parseCreateArtifactRequest } from './create-artifact-request.dto.js';

describe('parseCreateArtifactRequest', () => {
  it('parses a valid body', () => {
    const request = parseCreateArtifactRequest({
      content: Buffer.from('hello').toString('base64'),
      mimeType: 'text/plain',
    });

    expect(request).toEqual({
      content: Buffer.from('hello').toString('base64'),
      mimeType: 'text/plain',
    });
  });

  it('throws BadRequestException for a non-object body', () => {
    expect(() => parseCreateArtifactRequest('not an object')).toThrow('JSON object');
  });

  it('throws BadRequestException for a missing content field', () => {
    expect(() => parseCreateArtifactRequest({ mimeType: 'text/plain' })).toThrow('content');
  });

  it('throws BadRequestException for a missing mimeType field', () => {
    expect(() => parseCreateArtifactRequest({ content: 'aGVsbG8=' })).toThrow('mimeType');
  });

  it('throws BadRequestException for non-base64 content', () => {
    expect(() =>
      parseCreateArtifactRequest({
        content: 'not base64!!',
        mimeType: 'text/plain',
      }),
    ).toThrow('base64');
  });
});
