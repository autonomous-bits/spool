import type { Pool, PoolClient, QueryResult, QueryResultRow } from 'pg';
import { ConflictException, Inject, Injectable } from '@nestjs/common';
import { Chunk } from '../domain/chunk.js';
import type { ChunkStatus } from '../domain/chunk.js';
import { PG_POOL } from './pg-pool.token.js';

interface ChunkRow extends QueryResultRow {
  id: string;
  workspace_id: string;
  label: string;
  content: string;
  discipline: string;
  chunk_type: string;
  context_kind: string;
  status: string;
  branch_id: string | null;
  origin_branch_id: string | null;
  created_by_stakeholder_id: string;
  updated_by_stakeholder_id: string;
  created_at: Date;
  updated_at: Date;
}

function toChunk(row: ChunkRow): Chunk {
  return new Chunk({
    id: row.id,
    workspaceId: row.workspace_id,
    label: row.label,
    content: row.content,
    discipline: row.discipline as Chunk['discipline'],
    chunkType: row.chunk_type as Chunk['chunkType'],
    contextKind: row.context_kind as Chunk['contextKind'],
    status: row.status as ChunkStatus,
    ...(row.branch_id === null ? {} : { branchId: row.branch_id }),
    ...(row.origin_branch_id === null ? {} : { originBranchId: row.origin_branch_id }),
    createdByStakeholderId: row.created_by_stakeholder_id,
    updatedByStakeholderId: row.updated_by_stakeholder_id,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  });
}

type Queryable = Pick<Pool, 'query'>;

function createNonDraftBranchConflict(branchId: string): ConflictException {
  return new ConflictException(`Branch ${branchId} is not in draft status`);
}

async function insertChunk(queryable: Queryable, chunk: Chunk): Promise<Chunk> {
  const result: QueryResult<ChunkRow> = await queryable.query<ChunkRow>(
    `INSERT INTO chunks (
       id, workspace_id, label, content, discipline, chunk_type, context_kind, status,
       branch_id, origin_branch_id,
       created_by_stakeholder_id, updated_by_stakeholder_id,
       created_at, updated_at
     ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
     RETURNING *`,
    [
      chunk.id,
      chunk.workspaceId,
      chunk.label,
      chunk.content,
      chunk.discipline,
      chunk.chunkType,
      chunk.contextKind,
      chunk.status,
      chunk.branchId ?? null,
      chunk.originBranchId ?? null,
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

async function assertDraftBranchLock(client: PoolClient, branchId: string): Promise<void> {
  const result: QueryResult<{ status: string } & QueryResultRow> = await client.query(
    'SELECT status FROM branches WHERE id = $1 FOR UPDATE',
    [branchId],
  );

  const row = result.rows[0];
  if (row?.status !== 'draft') {
    throw createNonDraftBranchConflict(branchId);
  }
}

/**
 * Postgres-backed repository for the Chunk aggregate (Meridian IDEA-31, amended by IDEA-77's
 * chunk_type/context_kind columns and IDEA-78's branchless/draft capture path). branch_id and
 * origin_branch_id are NULL for a branchless capture (G01) or both set to the same branch's id
 * when the chunk is attached to a draft branch at creation time (G02). G04 hardens branch-scoped
 * writes by re-checking the branch's draft status under a row lock inside the insert transaction.
 */
@Injectable()
export class ChunkRepository {
  constructor(@Inject(PG_POOL) private readonly pool: Pool) {}

  /**
   * Persists a newly-constructed Chunk and returns the persisted entity (round-tripped from the
   * database row, not the in-memory instance).
   */
  async create(chunk: Chunk): Promise<Chunk> {
    if (chunk.branchId === undefined) {
      return insertChunk(this.pool, chunk);
    }

    const client = await this.pool.connect();
    let transactionOpen = false;

    try {
      await client.query('BEGIN');
      transactionOpen = true;

      await assertDraftBranchLock(client, chunk.branchId);
      await client.query('UPDATE branches SET updated_at = clock_timestamp() WHERE id = $1', [
        chunk.branchId,
      ]);

      const created = await insertChunk(client, chunk);

      await client.query('COMMIT');
      transactionOpen = false;
      return created;
    } catch (error) {
      if (transactionOpen) {
        await client.query('ROLLBACK');
      }
      throw error;
    } finally {
      client.release();
    }
  }

  /**
   * Looks up a chunk by id, scoped to a workspace (Meridian IDEA-98/IDEA-100, G11 SG4). A
   * cross-workspace id is indistinguishable from "does not exist" (returns `undefined`, not a
   * scope violation) so a lookup can never leak whether an id exists in another workspace.
   */
  async findById(id: string, workspaceId: string): Promise<Chunk | undefined> {
    const result: QueryResult<ChunkRow> = await this.pool.query<ChunkRow>(
      'SELECT * FROM chunks WHERE id = $1 AND workspace_id = $2',
      [id, workspaceId],
    );

    const row = result.rows[0];
    return row === undefined ? undefined : toChunk(row);
  }

  /**
   * Looks up a chunk by its logical label, scoped to a single branch (or to the branchless-draft
   * scope when `branchId` is omitted). This is a narrow existence-check for edge-endpoint
   * validation (Meridian IDEA-36/IDEA-37); it is not a generic branch-overlay/UNION read, which
   * remains out of scope until a future goal. Returns `undefined` as an explicit not-found result
   * rather than throwing.
   *
   * `workspaceId` is optional (G11 SG4/SG5): every caller (`ChunksService`, `EdgesService`,
   * `ArtifactsService`) now passes it, adding an `AND workspace_id = $n` filter so labels only
   * collide within the same workspace. It stays optional at the type level only to avoid
   * breaking any as-yet-unwritten caller; there is no remaining first-party caller that omits it.
   */
  async findByLabel(
    label: string,
    branchId: string | undefined,
    workspaceId?: string,
  ): Promise<Chunk | undefined> {
    const conditions = ['label = $1'];
    const params: unknown[] = [label];

    if (branchId === undefined) {
      conditions.push('branch_id IS NULL');
    } else {
      params.push(branchId);
      conditions.push(`branch_id = $${params.length}`);
    }

    if (workspaceId !== undefined) {
      params.push(workspaceId);
      conditions.push(`workspace_id = $${params.length}`);
    }

    const result: QueryResult<ChunkRow> = await this.pool.query<ChunkRow>(
      `SELECT * FROM chunks WHERE ${conditions.join(' AND ')}`,
      params,
    );

    const row = result.rows[0];
    return row === undefined ? undefined : toChunk(row);
  }
}
