import type { ChunkArtifactAssociation } from '../domain/chunk-artifact-association.js';
import type { ChunkArtifactAssociationStatus } from '../domain/types/vocabulary/chunk-artifact-association-status.js';

/**
 * HTTP-facing shape of a persisted ChunkArtifactAssociation, per Meridian IDEA-52/IDEA-34. Kept
 * as an explicit interface (rather than returning the domain entity directly) so the API response
 * contract is typed independently of the domain entity's internal shape.
 */
export interface ChunkArtifactResponse {
  id: string;
  chunkLabel: string;
  artifactId: string;
  status: ChunkArtifactAssociationStatus;
  branchId: string | null;
  originBranchId: string | null;
  createdByStakeholderId: string;
  updatedByStakeholderId: string;
  createdAt: Date;
  updatedAt: Date;
}

export function toChunkArtifactResponse(
  association: ChunkArtifactAssociation,
): ChunkArtifactResponse {
  return {
    id: association.id,
    chunkLabel: association.chunkLabel,
    artifactId: association.artifactId,
    status: association.status,
    branchId: association.branchId ?? null,
    originBranchId: association.originBranchId ?? null,
    createdByStakeholderId: association.createdByStakeholderId,
    updatedByStakeholderId: association.updatedByStakeholderId,
    createdAt: association.createdAt,
    updatedAt: association.updatedAt,
  } satisfies ChunkArtifactResponse;
}
