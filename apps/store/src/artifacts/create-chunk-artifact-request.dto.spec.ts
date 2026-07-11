import { describe, expect, it } from 'vitest';
import { parseCreateChunkArtifactRequest } from './create-chunk-artifact-request.dto.js';

describe('parseCreateChunkArtifactRequest', () => {
  it('parses a valid branchless body', () => {
    const request = parseCreateChunkArtifactRequest({
      artifactId: 'artifact-1',
    });

    expect(request).toEqual({ artifactId: 'artifact-1' });
  });

  it('parses a valid branch-scoped body', () => {
    const request = parseCreateChunkArtifactRequest({
      artifactId: 'artifact-1',
      branchId: 'branch-1',
    });

    expect(request).toEqual({
      artifactId: 'artifact-1',
      branchId: 'branch-1',
    });
  });

  it('throws BadRequestException for a non-object body', () => {
    expect(() => parseCreateChunkArtifactRequest('not an object')).toThrow('JSON object');
  });

  it('throws BadRequestException for a missing artifactId field', () => {
    expect(() => parseCreateChunkArtifactRequest({})).toThrow('artifactId');
  });

  it('throws BadRequestException for a blank branchId when provided', () => {
    expect(() =>
      parseCreateChunkArtifactRequest({
        artifactId: 'artifact-1',
        branchId: '   ',
      }),
    ).toThrow('branchId');
  });
});
