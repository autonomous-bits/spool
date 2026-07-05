import type { Pool, QueryResult, QueryResultRow } from 'pg';
import { Inject, Injectable } from '@nestjs/common';
import { Chunk } from '../domain/chunk.js';
import type { ChunkStatus } from '../domain/chunk.js';
import { PG_POOL } from './pg-pool.token.js';

interface ChunkRow extends QueryResultRow {
  id: string;
  label: string;
  content: string;
  discipline: string;
  chunk_type: string;
  context_kind: string;
  status: string;
  created_by_stakeholder_id: string;
  updated_by_stakeholder_id: string;
  created_at: Date;
  updated_at: Date;
}

function toChunk(row: ChunkRow): Chunk {
  return new Chunk({
    id: row.id,
    label: row.label,
    content: row.content,
    discipline: row.discipline as Chunk['discipline'],
    chunkType: row.chunk_type as Chunk['chunkType'],
    contextKind: row.context_kind as Chunk['contextKind'],
    status: row.status as ChunkStatus,
    createdByStakeholderId: row.created_by_stakeholder_id,
    updatedByStakeholderId: row.updated_by_stakeholder_id,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  });
}

/**
 * Postgres-backed repository for the Chunk aggregate (Meridian IDEA-31, amended by IDEA-77's
 * chunk_type/context_kind columns and IDEA-78's branchless/draft capture path). G01 only ever
 * persists branchless drafts: branch_id/origin_branch_id are always NULL and status is always
 * 'draft' on create, per IDEA-78.
 */
@Injectable()
export class ChunkRepository {
  constructor(@Inject(PG_POOL) private readonly pool: Pool) {}

  /**
   * Persists a newly-constructed Chunk as a branchless mainline draft and returns the persisted
   * entity (round-tripped from the database row, not the in-memory instance).
   */
  async create(chunk: Chunk): Promise<Chunk> {
    const result: QueryResult<ChunkRow> = await this.pool.query<ChunkRow>(
      `INSERT INTO chunks (
         id, label, content, discipline, chunk_type, context_kind, status,
         branch_id, origin_branch_id,
         created_by_stakeholder_id, updated_by_stakeholder_id,
         created_at, updated_at
       ) VALUES ($1, $2, $3, $4, $5, $6, $7, NULL, NULL, $8, $9, $10, $11)
       RETURNING *`,
      [
        chunk.id,
        chunk.label,
        chunk.content,
        chunk.discipline,
        chunk.chunkType,
        chunk.contextKind,
        chunk.status,
        chunk.createdByStakeholderId,
        chunk.updatedByStakeholderId,
        chunk.createdAt,
        chunk.updatedAt,
      ],
    );

    const row = result.rows[0];
    if (row === undefined) {
      throw new Error('ChunkRepository.create: INSERT ... RETURNING * produced no row');
    }

    return toChunk(row);
  }

  /**
   * Looks up a chunk by id. Returns `undefined` as an explicit not-found result rather than
   * throwing, so callers can distinguish "not found" from an actual persistence error.
   */
  async findById(id: string): Promise<Chunk | undefined> {
    const result: QueryResult<ChunkRow> = await this.pool.query<ChunkRow>(
      'SELECT * FROM chunks WHERE id = $1',
      [id],
    );

    const row = result.rows[0];
    return row === undefined ? undefined : toChunk(row);
  }
}
