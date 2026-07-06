import { randomUUID } from 'node:crypto';
import type { ChunkArtifactAssociationStatus } from './types/vocabulary/chunk-artifact-association-status.js';
import { parseChunkArtifactAssociationStatus } from './types/vocabulary/chunk-artifact-association-status.js';

export interface ChunkArtifactAssociationProps {
  id?: string;
  chunkLabel: string;
  artifactId: string;
  status?: ChunkArtifactAssociationStatus;
  branchId?: string;
  originBranchId?: string;
  createdByStakeholderId: string;
  updatedByStakeholderId?: string;
  createdAt?: Date;
  updatedAt?: Date;
}

function requireNonBlank(value: string, fieldName: string): string {
  if (value.trim().length === 0) {
    throw new TypeError(`ChunkArtifactAssociation ${fieldName} must not be empty or blank`);
  }
  return value;
}

/**
 * ChunkArtifactAssociation entity: links a chunk (by logical label, per the Edge precedent in
 * Meridian IDEA-36/IDEA-37) to an immutable Artifact. Associations are branch-scoped delta rows
 * (Meridian IDEA-32/IDEA-60): a mainline row has branchId/originBranchId undefined, and a
 * branch-scoped row (add or deactivate) carries both set to that branch's id. `status` follows
 * the Edge vocabulary shape for schema parity; this goal's write paths only ever produce 'active'
 * (associate) and 'deactivated' (disassociate) rows — 'superseded' is unused by any G08 write
 * path. Reads never mutate an existing row in place; they resolve the effective association by
 * selecting the most-recently-created row per (chunkLabel, artifactId) scope.
 */
export class ChunkArtifactAssociation {
  readonly id: string;
  readonly chunkLabel: string;
  readonly artifactId: string;
  readonly status: ChunkArtifactAssociationStatus;
  readonly branchId: string | undefined;
  readonly originBranchId: string | undefined;
  readonly createdByStakeholderId: string;
  readonly updatedByStakeholderId: string;
  readonly createdAt: Date;
  readonly updatedAt: Date;

  constructor(props: ChunkArtifactAssociationProps) {
    this.chunkLabel = requireNonBlank(props.chunkLabel, 'chunkLabel');
    this.artifactId = requireNonBlank(props.artifactId, 'artifactId');
    this.status = parseChunkArtifactAssociationStatus(props.status ?? 'active');

    if (props.createdByStakeholderId.trim().length === 0) {
      throw new TypeError(
        'ChunkArtifactAssociation requires a non-blank createdByStakeholderId',
      );
    }

    this.id = props.id ?? randomUUID();
    this.branchId = props.branchId;
    this.originBranchId = props.originBranchId;
    this.createdByStakeholderId = props.createdByStakeholderId;
    this.updatedByStakeholderId = props.updatedByStakeholderId ?? props.createdByStakeholderId;
    this.createdAt = props.createdAt ?? new Date();
    this.updatedAt = props.updatedAt ?? this.createdAt;
  }
}
