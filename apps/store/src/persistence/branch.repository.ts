import type { Pool, QueryResult, QueryResultRow } from 'pg';
import { Inject, Injectable } from '@nestjs/common';
import { Branch } from '../domain/branch.js';
import { DivergencePoint } from '../domain/divergence-point.js';
import { PG_POOL } from './pg-pool.token.js';

export interface BranchRow extends QueryResultRow {
  id: string;
  name: string;
  discipline: string;
  status: string;
  diverged_at: Date;
  submitted_at: Date | null;
  verified_at: Date | null;
  merged_at: Date | null;
  merged_by_stakeholder_id: string | null;
  origin_suggestion_id: string | null;
  created_by_stakeholder_id: string;
  created_at: Date;
  updated_at: Date;
}

interface TimestampRow extends QueryResultRow {
  attempted_at: Date;
}

interface LabelRow extends QueryResultRow {
  label: string;
}

interface EdgeIdentityRow extends QueryResultRow {
  from_chunk_label: string;
  to_chunk_label: string;
  type: string;
}

/**
 * Result of a successful or conflicted merge attempt (Meridian IDEA-74/IDEA-46, scoped per G06
 * OQ2 to "any mainline identity collision blocks the merge"). `undefined` is reserved for the
 * distinct "branch not mergeable" outcome (not verified / not found).
 */
export type BranchMergeResult =
  | { kind: 'merged'; branch: Branch }
  | { kind: 'conflict'; reason: string };

function edgeIdentityKey(row: Pick<EdgeIdentityRow, 'from_chunk_label' | 'to_chunk_label' | 'type'>): string {
  return `${row.from_chunk_label}\u0000${row.to_chunk_label}\u0000${row.type}`;
}

export function toBranch(row: BranchRow): Branch {
  return new Branch({
    id: row.id,
    name: row.name,
    discipline: row.discipline as Branch['discipline'],
    status: row.status as Branch['status'],
    divergedAt: new DivergencePoint(row.diverged_at.toISOString()),
    ...(row.submitted_at === null ? {} : { submittedAt: row.submitted_at }),
    ...(row.verified_at === null ? {} : { verifiedAt: row.verified_at }),
    ...(row.merged_at === null ? {} : { mergedAt: row.merged_at }),
    ...(row.merged_by_stakeholder_id === null
      ? {}
      : { mergedByStakeholderId: row.merged_by_stakeholder_id }),
    ...(row.origin_suggestion_id === null
      ? {}
      : { originSuggestionId: row.origin_suggestion_id }),
    createdByStakeholderId: row.created_by_stakeholder_id,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  });
}

/**
 * Postgres-backed repository for the Branch aggregate (Meridian IDEA-31's authoritative schema).
 * G02 only ever persists draft branches created directly by a stakeholder; submitted_at,
 * merged_at, origin_suggestion_id, and merged_by_stakeholder_id are always NULL on create.
 */
@Injectable()
export class BranchRepository {
  constructor(@Inject(PG_POOL) private readonly pool: Pool) {}

  /**
   * Persists a newly-constructed Branch as a draft and returns the persisted entity
   * (round-tripped from the database row, not the in-memory instance).
   */
  async create(branch: Branch): Promise<Branch> {
    const result: QueryResult<BranchRow> = await this.pool.query<BranchRow>(
      `WITH persisted_timestamps AS (
         SELECT clock_timestamp() AS persisted_at
       )
       INSERT INTO branches (
         id, name, discipline, status, diverged_at,
         created_by_stakeholder_id, created_at, updated_at
       )
       SELECT
         $1, $2, $3, $4, $5, $6,
         persisted_timestamps.persisted_at,
         persisted_timestamps.persisted_at
       FROM persisted_timestamps
       RETURNING *`,
      [
        branch.id,
        branch.name,
        branch.discipline,
        branch.status,
        branch.divergedAt.toISOString(),
        branch.createdByStakeholderId,
      ],
    );

    const row = result.rows[0];
    if (row === undefined) {
      throw new Error('BranchRepository.create: INSERT ... RETURNING * produced no row');
    }

    return toBranch(row);
  }

  /**
   * Looks up a branch by id. Returns `undefined` as an explicit not-found result rather than
   * throwing, so callers can distinguish "not found" from an actual persistence error.
   */
  async findById(id: string): Promise<Branch | undefined> {
    const result: QueryResult<BranchRow> = await this.pool.query<BranchRow>(
      'SELECT * FROM branches WHERE id = $1',
      [id],
    );

    const row = result.rows[0];
    return row === undefined ? undefined : toBranch(row);
  }

  async submit(branchId: string): Promise<Branch | undefined> {
    const client = await this.pool.connect();
    let transactionOpen = false;

    try {
      await client.query('BEGIN');
      transactionOpen = true;

      const timestampResult: QueryResult<TimestampRow> =
        await client.query<TimestampRow>('SELECT clock_timestamp() AS attempted_at');
      const attemptedAt = timestampResult.rows[0]?.attempted_at;
      if (attemptedAt === undefined) {
        throw new Error('BranchRepository.submit: SELECT clock_timestamp() produced no row');
      }

      const branchResult: QueryResult<BranchRow> = await client.query<BranchRow>(
        'SELECT * FROM branches WHERE id = $1 FOR UPDATE',
        [branchId],
      );
      const branchRow = branchResult.rows[0];
      if (
        branchRow?.status !== 'draft' ||
        branchRow.updated_at > attemptedAt
      ) {
        await client.query('ROLLBACK');
        transactionOpen = false;
        return undefined;
      }

      const result: QueryResult<BranchRow> = await client.query<BranchRow>(
        "UPDATE branches SET status='submitted', submitted_at=now(), updated_at=now() WHERE id=$1 RETURNING *",
        [branchId],
      );

      const row = result.rows[0];
      if (row === undefined) {
        throw new Error('BranchRepository.submit: UPDATE ... RETURNING * produced no row');
      }

      await client.query('COMMIT');
      transactionOpen = false;
      return toBranch(row);
    } catch (error) {
      if (transactionOpen) {
        await client.query('ROLLBACK');
      }
      throw error;
    } finally {
      client.release();
    }
  }

  async verify(branchId: string): Promise<Branch | undefined> {
    const client = await this.pool.connect();
    let transactionOpen = false;

    try {
      await client.query('BEGIN');
      transactionOpen = true;

      const timestampResult: QueryResult<TimestampRow> =
        await client.query<TimestampRow>('SELECT clock_timestamp() AS attempted_at');
      const attemptedAt = timestampResult.rows[0]?.attempted_at;
      if (attemptedAt === undefined) {
        throw new Error('BranchRepository.verify: SELECT clock_timestamp() produced no row');
      }

      const branchResult: QueryResult<BranchRow> = await client.query<BranchRow>(
        'SELECT * FROM branches WHERE id = $1 FOR UPDATE',
        [branchId],
      );
      const branchRow = branchResult.rows[0];
      if (
        branchRow?.status !== 'submitted' ||
        branchRow.updated_at > attemptedAt
      ) {
        await client.query('ROLLBACK');
        transactionOpen = false;
        return undefined;
      }

      const result: QueryResult<BranchRow> = await client.query<BranchRow>(
        "UPDATE branches SET status='verified', verified_at=now(), updated_at=now() WHERE id=$1 RETURNING *",
        [branchId],
      );

      const row = result.rows[0];
      if (row === undefined) {
        throw new Error('BranchRepository.verify: UPDATE ... RETURNING * produced no row');
      }

      await client.query('COMMIT');
      transactionOpen = false;
      return toBranch(row);
    } catch (error) {
      if (transactionOpen) {
        await client.query('ROLLBACK');
      }
      throw error;
    } finally {
      client.release();
    }
  }

  async reject(branchId: string): Promise<Branch | undefined> {
    const client = await this.pool.connect();
    let transactionOpen = false;

    try {
      await client.query('BEGIN');
      transactionOpen = true;

      const timestampResult: QueryResult<TimestampRow> =
        await client.query<TimestampRow>('SELECT clock_timestamp() AS attempted_at');
      const attemptedAt = timestampResult.rows[0]?.attempted_at;
      if (attemptedAt === undefined) {
        throw new Error('BranchRepository.reject: SELECT clock_timestamp() produced no row');
      }

      const branchResult: QueryResult<BranchRow> = await client.query<BranchRow>(
        'SELECT * FROM branches WHERE id = $1 FOR UPDATE',
        [branchId],
      );
      const branchRow = branchResult.rows[0];
      if (
        branchRow === undefined ||
        !['submitted', 'verified'].includes(branchRow.status) ||
        branchRow.updated_at > attemptedAt
      ) {
        await client.query('ROLLBACK');
        transactionOpen = false;
        return undefined;
      }

      const result: QueryResult<BranchRow> = await client.query<BranchRow>(
        "UPDATE branches SET status='draft', verified_at=NULL, submitted_at=NULL, updated_at=now() WHERE id=$1 RETURNING *",
        [branchId],
      );

      const row = result.rows[0];
      if (row === undefined) {
        throw new Error('BranchRepository.reject: UPDATE ... RETURNING * produced no row');
      }

      await client.query('COMMIT');
      transactionOpen = false;
      return toBranch(row);
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
   * Merges a verified branch into mainline as a single atomic transaction (Meridian IDEA-40's
   * verified -> merged transition, IDEA-74's merge-lineage/provenance shape, IDEA-46's conflict
   * gate scoped per G06 OQ2 to "any mainline identity collision blocks the merge").
   *
   * Promotes every draft chunk and active edge attached to the branch (branch_id -> NULL,
   * chunk status -> 'promoted'; origin_branch_id is left untouched, since it was already set to
   * this branch's id at capture time) and marks the branch 'merged' with merged_at/
   * merged_by_stakeholder_id — or rejects the merge in full, with zero rows mutated, if any
   * branch chunk label or edge identity ((from,to,type)) already exists on mainline. Returns
   * `undefined` (no mutation at all) when the branch is not found or not in 'verified' status, so
   * callers can distinguish "not mergeable" from a genuine identity conflict.
   */
  async merge(branchId: string, mergedByStakeholderId: string): Promise<BranchMergeResult | undefined> {
    const client = await this.pool.connect();
    let transactionOpen = false;

    try {
      await client.query('BEGIN');
      transactionOpen = true;

      const branchResult: QueryResult<BranchRow> = await client.query<BranchRow>(
        'SELECT * FROM branches WHERE id = $1 FOR UPDATE',
        [branchId],
      );
      const branchRow = branchResult.rows[0];
      if (branchRow?.status !== 'verified') {
        await client.query('ROLLBACK');
        transactionOpen = false;
        return undefined;
      }

      const chunkLabelCollisions: QueryResult<LabelRow> = await client.query<LabelRow>(
        `SELECT branch_chunks.label
           FROM chunks branch_chunks
           JOIN chunks mainline_chunks
             ON mainline_chunks.label = branch_chunks.label
            AND mainline_chunks.branch_id IS NULL
            AND mainline_chunks.status = 'promoted'
          WHERE branch_chunks.branch_id = $1
            AND branch_chunks.status = 'draft'`,
        [branchId],
      );
      if (chunkLabelCollisions.rows.length > 0) {
        await client.query('ROLLBACK');
        transactionOpen = false;
        return {
          kind: 'conflict',
          reason: `mainline chunk label collision: ${chunkLabelCollisions.rows.map((row) => row.label).join(', ')}`,
        };
      }

      const edgeIdentityCollisions: QueryResult<EdgeIdentityRow> = await client.query<EdgeIdentityRow>(
        `SELECT branch_edges.from_chunk_label, branch_edges.to_chunk_label, branch_edges.type
           FROM edges branch_edges
           JOIN edges mainline_edges
             ON mainline_edges.from_chunk_label = branch_edges.from_chunk_label
            AND mainline_edges.to_chunk_label = branch_edges.to_chunk_label
            AND mainline_edges.type = branch_edges.type
            AND mainline_edges.branch_id IS NULL
            AND mainline_edges.status = 'active'
          WHERE branch_edges.branch_id = $1
            AND branch_edges.status = 'active'`,
        [branchId],
      );
      if (edgeIdentityCollisions.rows.length > 0) {
        await client.query('ROLLBACK');
        transactionOpen = false;
        return {
          kind: 'conflict',
          reason: `mainline edge identity collision: ${edgeIdentityCollisions.rows.map(edgeIdentityKey).join(', ')}`,
        };
      }

      await client.query(
        "UPDATE chunks SET branch_id = NULL, status = 'promoted', updated_at = now() WHERE branch_id = $1 AND status = 'draft'",
        [branchId],
      );
      await client.query(
        'UPDATE edges SET branch_id = NULL, updated_at = now() WHERE branch_id = $1 AND status = $2',
        [branchId, 'active'],
      );

      const mergedResult: QueryResult<BranchRow> = await client.query<BranchRow>(
        `UPDATE branches
            SET status = 'merged', merged_at = now(), merged_by_stakeholder_id = $2, updated_at = now()
          WHERE id = $1
          RETURNING *`,
        [branchId, mergedByStakeholderId],
      );

      const mergedRow = mergedResult.rows[0];
      if (mergedRow === undefined) {
        throw new Error('BranchRepository.merge: UPDATE ... RETURNING * produced no row');
      }

      await client.query('COMMIT');
      transactionOpen = false;
      return { kind: 'merged', branch: toBranch(mergedRow) };
    } catch (error) {
      if (transactionOpen) {
        await client.query('ROLLBACK');
      }
      throw error;
    } finally {
      client.release();
    }
  }
}
