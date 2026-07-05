import type { Pool, PoolClient, QueryResult, QueryResultRow } from 'pg';
import { ConflictException, Inject, Injectable } from '@nestjs/common';
import { Edge } from '../domain/edge.js';
import { PG_POOL } from './pg-pool.token.js';

interface EdgeRow extends QueryResultRow {
  id: string;
  from_chunk_label: string;
  to_chunk_label: string;
  type: string;
  status: string;
  discipline: string;
  branch_id: string | null;
  origin_branch_id: string | null;
  superseded_by_edge_id: string | null;
  created_by_stakeholder_id: string;
  updated_by_stakeholder_id: string;
  created_at: Date;
  updated_at: Date;
}

function toEdge(row: EdgeRow): Edge {
  return new Edge({
    id: row.id,
    fromChunkLabel: row.from_chunk_label,
    toChunkLabel: row.to_chunk_label,
    type: row.type as Edge['type'],
    status: row.status as Edge['status'],
    discipline: row.discipline as Edge['discipline'],
    ...(row.branch_id === null ? {} : { branchId: row.branch_id }),
    ...(row.origin_branch_id === null ? {} : { originBranchId: row.origin_branch_id }),
    ...(row.superseded_by_edge_id === null ? {} : { supersededByEdgeId: row.superseded_by_edge_id }),
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

async function insertEdge(queryable: Queryable, edge: Edge): Promise<Edge> {
  const result: QueryResult<EdgeRow> = await queryable.query<EdgeRow>(
    `INSERT INTO edges (
       id, from_chunk_label, to_chunk_label, type, status, discipline,
       branch_id, origin_branch_id, superseded_by_edge_id,
       created_by_stakeholder_id, updated_by_stakeholder_id,
       created_at, updated_at
     ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
     RETURNING *`,
    [
      edge.id,
      edge.fromChunkLabel,
      edge.toChunkLabel,
      edge.type,
      edge.status,
      edge.discipline,
      edge.branchId ?? null,
      edge.originBranchId ?? null,
      edge.supersededByEdgeId ?? null,
      edge.createdByStakeholderId,
      edge.updatedByStakeholderId,
      edge.createdAt,
      edge.updatedAt,
    ],
  );

  const row = result.rows[0];
  if (row === undefined) {
    throw new Error('EdgeRepository.create: INSERT ... RETURNING * produced no row');
  }

  return toEdge(row);
}

async function assertDraftBranchLock(client: PoolClient, branchId: string): Promise<void> {
  const result: QueryResult<{ status: string } & QueryResultRow> = await client.query(
    'SELECT status FROM branches WHERE id = $1 FOR UPDATE',
    [branchId],
  );

  const row = result.rows[0];
  if (row === undefined || row.status !== 'draft') {
    throw createNonDraftBranchConflict(branchId);
  }
}

/**
 * Postgres-backed repository for the Edge aggregate (Meridian IDEA-31/IDEA-32/IDEA-33/IDEA-44).
 * G03 only ever creates 'active' edges; supersededByEdgeId is always NULL on create. No generic
 * list/overlay-read method is exposed this goal (Meridian IDEA-33's branch-overlay UNION read is
 * out of scope until a future goal), so only create/findById are provided. G04 hardens
 * branch-scoped writes by re-checking the branch's draft status under a row lock inside the insert
 * transaction.
 */
@Injectable()
export class EdgeRepository {
  constructor(@Inject(PG_POOL) private readonly pool: Pool) {}

  /**
   * Persists a newly-constructed Edge and returns the persisted entity (round-tripped from the
   * database row, not the in-memory instance).
   */
  async create(edge: Edge): Promise<Edge> {
    if (edge.branchId === undefined) {
      return insertEdge(this.pool, edge);
    }

    const client = await this.pool.connect();
    let transactionOpen = false;

    try {
      await client.query('BEGIN');
      transactionOpen = true;

      await assertDraftBranchLock(client, edge.branchId);
      await client.query('UPDATE branches SET updated_at = clock_timestamp() WHERE id = $1', [
        edge.branchId,
      ]);

      const created = await insertEdge(client, edge);

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
   * Looks up an edge by id. Returns `undefined` as an explicit not-found result rather than
   * throwing, so callers can distinguish "not found" from an actual persistence error.
   */
  async findById(id: string): Promise<Edge | undefined> {
    const result: QueryResult<EdgeRow> = await this.pool.query<EdgeRow>(
      'SELECT * FROM edges WHERE id = $1',
      [id],
    );

    const row = result.rows[0];
    return row === undefined ? undefined : toEdge(row);
  }
}
