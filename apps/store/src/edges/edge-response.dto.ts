import type { Edge } from '../domain/edge.js';
import type { Discipline } from '../domain/types/vocabulary/discipline.js';
import type { EdgeStatus } from '../domain/types/vocabulary/edge-status.js';
import type { EdgeType } from '../domain/types/vocabulary/edge-type.js';

/**
 * HTTP-facing shape of a persisted Edge, per Meridian IDEA-52/IDEA-34. Kept as an explicit
 * interface (rather than returning the `Edge` domain entity directly) so the API response
 * contract is typed independently of the domain entity's internal shape.
 * `supersededByEdgeId` is always `null` this goal (only 'active' edges are ever created).
 */
export interface EdgeResponse {
  id: string;
  fromChunkLabel: string;
  toChunkLabel: string;
  type: EdgeType;
  status: EdgeStatus;
  discipline: Discipline;
  branchId: string | null;
  originBranchId: string | null;
  supersededByEdgeId: string | null;
  createdByStakeholderId: string;
  updatedByStakeholderId: string;
  createdAt: Date;
  updatedAt: Date;
}

export function toEdgeResponse(edge: Edge): EdgeResponse {
  return {
    id: edge.id,
    fromChunkLabel: edge.fromChunkLabel,
    toChunkLabel: edge.toChunkLabel,
    type: edge.type,
    status: edge.status,
    discipline: edge.discipline,
    branchId: edge.branchId ?? null,
    originBranchId: edge.originBranchId ?? null,
    supersededByEdgeId: edge.supersededByEdgeId ?? null,
    createdByStakeholderId: edge.createdByStakeholderId,
    updatedByStakeholderId: edge.updatedByStakeholderId,
    createdAt: edge.createdAt,
    updatedAt: edge.updatedAt,
  } satisfies EdgeResponse;
}
