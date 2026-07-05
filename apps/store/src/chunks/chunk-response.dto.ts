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
  label: string;
  content: string;
  discipline: Discipline;
  chunkType: ChunkType;
  contextKind: ContextKind;
  status: ChunkStatus;
  createdByStakeholderId: string;
  updatedByStakeholderId: string;
  createdAt: Date;
  updatedAt: Date;
}

export function toChunkResponse(chunk: Chunk): ChunkResponse {
  return {
    id: chunk.id,
    label: chunk.label,
    content: chunk.content,
    discipline: chunk.discipline,
    chunkType: chunk.chunkType,
    contextKind: chunk.contextKind,
    status: chunk.status,
    createdByStakeholderId: chunk.createdByStakeholderId,
    updatedByStakeholderId: chunk.updatedByStakeholderId,
    createdAt: chunk.createdAt,
    updatedAt: chunk.updatedAt,
  } satisfies ChunkResponse;
}
