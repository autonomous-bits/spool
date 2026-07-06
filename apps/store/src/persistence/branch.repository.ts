import type { Pool, QueryResult, QueryResultRow } from 'pg';
import { Inject, Injectable } from '@nestjs/common';
import { Branch } from '../domain/branch.js';
import { DivergencePoint } from '../domain/divergence-point.js';
import { PG_POOL } from './pg-pool.token.js';

interface BranchRow extends QueryResultRow {
  id: string;
  name: string;
  discipline: string;
  status: string;
  diverged_at: Date;
  submitted_at: Date | null;
  verified_at: Date | null;
  created_by_stakeholder_id: string;
  created_at: Date;
  updated_at: Date;
}

interface TimestampRow extends QueryResultRow {
  attempted_at: Date;
}

function toBranch(row: BranchRow): Branch {
  return new Branch({
    id: row.id,
    name: row.name,
    discipline: row.discipline as Branch['discipline'],
    status: row.status as Branch['status'],
    divergedAt: new DivergencePoint(row.diverged_at.toISOString()),
    ...(row.submitted_at === null ? {} : { submittedAt: row.submitted_at }),
    ...(row.verified_at === null ? {} : { verifiedAt: row.verified_at }),
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
        branchRow === undefined ||
        branchRow.status !== 'draft' ||
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
        branchRow === undefined ||
        branchRow.status !== 'submitted' ||
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
}
