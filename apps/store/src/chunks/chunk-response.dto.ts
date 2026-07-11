import type { ChunkStatus } from '../domain/chunk.js';
import type { Chunk } from '../domain/chunk.js';
import type { ChunkType } from '../domain/types/vocabulary/chunk-type.js';
import type { ContextKind } from '../domain/types/vocabulary/context-kind.js';
import type { Discipline } from '../domain/types/vocabulary/discipline.js';

/**
 * HTTP-facing shape of a persisted Chunk, per Meridian IDEA-52/IDEA-34. Kept as an explicit
 * interface (rather than returning the `Chunk` domain entity directly) so the API response
 * contract is typed independently of the domain entity's internal shape.
 */
export interface ChunkResponse {
  id: string;
  workspaceId: string;
  label: string;
  content: string;
  discipline: Discipline;
  chunkType: ChunkType;
  contextKind: ContextKind;
  status: ChunkStatus;
  createdByStakeholderId: string;
  updatedByStakeholderId: string;
  branchId: string | null;
  originBranchId: string | null;
  createdAt: Date;
  updatedAt: Date;
}

export interface NeighbourResponse {
  edgeId: string;
  chunkId: string;
  label: string;
  content: string;
  type: string;
  status: string;
  discipline: string;
  contextKind: string;
  direction: 'outgoing' | 'incoming';
  hop: number;
}

export function toChunkResponse(chunk: Chunk): ChunkResponse {
  return {
    id: chunk.id,
    workspaceId: chunk.workspaceId,
    label: chunk.label,
    content: chunk.content,
    discipline: chunk.discipline,
    chunkType: chunk.chunkType,
    contextKind: chunk.contextKind,
    status: chunk.status,
    createdByStakeholderId: chunk.createdByStakeholderId,
    updatedByStakeholderId: chunk.updatedByStakeholderId,
    branchId: chunk.branchId ?? null,
    originBranchId: chunk.originBranchId ?? null,
    createdAt: chunk.createdAt,
    updatedAt: chunk.updatedAt,
  } satisfies ChunkResponse;
}
