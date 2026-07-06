import type { Pool, QueryResult, QueryResultRow } from 'pg';
import { Inject, Injectable } from '@nestjs/common';
import { assertReviewableStatus, BranchLifecycleError } from '../domain/branch-lifecycle.js';
import { VerificationSignal } from '../domain/verification-signal.js';
import type { VerificationSignalStatus } from '../domain/types/vocabulary/verification-signal-status.js';
import { parseBranchStatus } from '../domain/types/vocabulary/branch-status.js';
import { PG_POOL } from './pg-pool.token.js';
import type { BranchRow } from './branch.repository.js';
import { StakeholderRepository } from './stakeholder.repository.js';

export interface VerificationSignalRow extends QueryResultRow {
  id: string;
  branch_id: string;
  verifier_name: string;
  status: string;
  reason: string | null;
  created_at: Date;
}

export function toVerificationSignal(row: VerificationSignalRow): VerificationSignal {
  return new VerificationSignal({
    id: row.id,
    branchId: row.branch_id,
    verifierName: row.verifier_name,
    status: row.status as VerificationSignalStatus,
    ...(row.reason === null ? {} : { reason: row.reason }),
    createdAt: row.created_at,
  });
}

/**
 * Result of a verification-signal submission attempt (Meridian IDEA-21/IDEA-43, G09 SG1).
 * `not_found` and `not_reviewable` are distinct outcomes so the service layer can map them to
 * 404 vs. 409 without an extra lookup.
 */
export type VerificationSignalCreateResult =
  | { kind: 'created'; signal: VerificationSignal }
  | { kind: 'not_found' }
  | { kind: 'not_reviewable'; branchStatus: string };

export interface CreateVerificationSignalParams {
  branchId: string;
  verifierName: string;
  status: VerificationSignalStatus;
  reason?: string;
}

/**
 * Postgres-backed repository for the VerificationSignal aggregate (Meridian IDEA-31's
 * authoritative schema). Submission is a single atomic transaction: lock the branch row, assert
 * it is reviewable (submitted/verified), insert the signal, then fan out one unread
 * feedback_notification per stakeholder -- all-or-nothing, and the branch's own status/updated_at
 * are never mutated (Meridian IDEA-43's no-auto-transition rule).
 */
@Injectable()
export class VerificationSignalRepository {
  constructor(
    @Inject(PG_POOL) private readonly pool: Pool,
    private readonly stakeholderRepository: StakeholderRepository,
  ) {}

  async create(params: CreateVerificationSignalParams): Promise<VerificationSignalCreateResult> {
    const client = await this.pool.connect();
    let transactionOpen = false;

    try {
      await client.query('BEGIN');
      transactionOpen = true;

      const branchResult: QueryResult<BranchRow> = await client.query<BranchRow>(
        'SELECT * FROM branches WHERE id = $1 FOR UPDATE',
        [params.branchId],
      );
      const branchRow = branchResult.rows[0];
      if (branchRow === undefined) {
        await client.query('ROLLBACK');
        transactionOpen = false;
        return { kind: 'not_found' };
      }
      try {
        assertReviewableStatus({ status: parseBranchStatus(branchRow.status) });
      } catch (error) {
        if (error instanceof BranchLifecycleError) {
          await client.query('ROLLBACK');
          transactionOpen = false;
          return { kind: 'not_reviewable', branchStatus: branchRow.status };
        }
        throw error;
      }

      const result: QueryResult<VerificationSignalRow> = await client.query<VerificationSignalRow>(
        `INSERT INTO verification_signals (id, branch_id, verifier_name, status, reason, created_at)
         VALUES (gen_random_uuid(), $1, $2, $3, $4, now())
         RETURNING *`,
        [params.branchId, params.verifierName, params.status, params.reason ?? null],
      );

      const row = result.rows[0];
      if (row === undefined) {
        throw new Error('VerificationSignalRepository.create: INSERT ... RETURNING * produced no row');
      }

      const stakeholders = await this.stakeholderRepository.findAll(client);
      for (const stakeholder of stakeholders) {
        await client.query(
          `INSERT INTO feedback_notifications (
             id, branch_id, stakeholder_id, signal_id, status, created_at, updated_at
           ) VALUES (gen_random_uuid(), $1, $2, $3, 'unread', now(), now())`,
          [params.branchId, stakeholder.id, row.id],
        );
      }

      await client.query('COMMIT');
      transactionOpen = false;
      return { kind: 'created', signal: toVerificationSignal(row) };
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
   * Lists verification signals for a branch ordered oldest-first (G09 SG1). Deliberately
   * unauthenticated, matching this codebase's existing GET /branches, GET /suggestions
   * precedent of unauthenticated reads.
   */
  async findByBranchId(branchId: string): Promise<VerificationSignal[]> {
    const result: QueryResult<VerificationSignalRow> = await this.pool.query<VerificationSignalRow>(
      'SELECT * FROM verification_signals WHERE branch_id = $1 ORDER BY created_at ASC',
      [branchId],
    );

    return result.rows.map(toVerificationSignal);
  }
}
