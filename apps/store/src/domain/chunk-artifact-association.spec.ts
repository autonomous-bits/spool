import { describe, expect, it } from 'vitest';
import {
  ChunkArtifactAssociation,
  type ChunkArtifactAssociationProps,
} from './chunk-artifact-association.js';

const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';

function validProps(
  overrides: Partial<ChunkArtifactAssociationProps> = {},
): ChunkArtifactAssociationProps {
  return {
    workspaceId: WORKSPACE_ID,
    chunkLabel: 'ATOMIC-1',
    artifactId: '00000000-0000-0000-0000-0000000000a1',
    createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
    ...overrides,
  };
}

describe('ChunkArtifactAssociation', () => {
  it('constructs a mainline association with defaulted status, id, and updatedByStakeholderId', () => {
    const association = new ChunkArtifactAssociation(validProps());

    expect(association.chunkLabel).toBe('ATOMIC-1');
    expect(association.artifactId).toBe('00000000-0000-0000-0000-0000000000a1');
    expect(association.status).toBe('active');
    expect(association.createdByStakeholderId).toBe('00000000-0000-0000-0000-000000000001');
    expect(association.updatedByStakeholderId).toBe('00000000-0000-0000-0000-000000000001');
    expect(association.id).toBeTruthy();
    expect(association.branchId).toBeUndefined();
    expect(association.originBranchId).toBeUndefined();
  });

  it('constructs a branch-scoped deactivation with branchId and originBranchId set', () => {
    const association = new ChunkArtifactAssociation(
      validProps({
        status: 'deactivated',
        branchId: '00000000-0000-0000-0000-0000000000b1',
        originBranchId: '00000000-0000-0000-0000-0000000000b1',
      }),
    );

    expect(association.status).toBe('deactivated');
    expect(association.branchId).toBe('00000000-0000-0000-0000-0000000000b1');
    expect(association.originBranchId).toBe('00000000-0000-0000-0000-0000000000b1');
  });

  it.each(['', '   '])('rejects blank chunkLabel %j', (chunkLabel) => {
    expect(() => new ChunkArtifactAssociation(validProps({ chunkLabel }))).toThrow(TypeError);
  });

  it.each(['', '   '])('rejects blank artifactId %j', (artifactId) => {
    expect(() => new ChunkArtifactAssociation(validProps({ artifactId }))).toThrow(TypeError);
  });

  it.each(['', '   '])('rejects blank workspaceId %j', (workspaceId) => {
    expect(() => new ChunkArtifactAssociation(validProps({ workspaceId }))).toThrow(TypeError);
  });

  it('rejects an invalid status', () => {
    expect(
      () =>
        new ChunkArtifactAssociation(
          validProps({
            status: 'archived' as unknown as ChunkArtifactAssociationProps['status'],
          }),
        ),
    ).toThrow(TypeError);
  });

  it('requires a non-blank createdByStakeholderId', () => {
    expect(() => new ChunkArtifactAssociation(validProps({ createdByStakeholderId: '' }))).toThrow(
      TypeError,
    );
    expect(
      () => new ChunkArtifactAssociation(validProps({ createdByStakeholderId: '   ' })),
    ).toThrow(TypeError);
  });
});
