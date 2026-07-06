import { describe, expect, it } from 'vitest';
import {
  isChunkArtifactAssociationStatus,
  parseChunkArtifactAssociationStatus,
} from './chunk-artifact-association-status.js';

const VALID_STATUSES = ['active', 'superseded', 'deactivated'] as const;

describe('ChunkArtifactAssociationStatus', () => {
  it.each(VALID_STATUSES)('accepts valid status %j', (status) => {
    expect(isChunkArtifactAssociationStatus(status)).toBe(true);
    expect(parseChunkArtifactAssociationStatus(status)).toBe(status);
  });

  it('rejects an unknown status', () => {
    expect(isChunkArtifactAssociationStatus('archived')).toBe(false);
    expect(() => parseChunkArtifactAssociationStatus('archived')).toThrow(TypeError);
  });

  it('rejects a non-string value', () => {
    expect(isChunkArtifactAssociationStatus(42)).toBe(false);
    expect(() => parseChunkArtifactAssociationStatus(null)).toThrow(TypeError);
  });
});
