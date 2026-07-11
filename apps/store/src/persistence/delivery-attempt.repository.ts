import type { Pool, PoolClient, QueryResult, QueryResultRow } from 'pg';
import { Inject, Injectable } from '@nestjs/common';
import { DeliveryAttempt } from '../domain/delivery-attempt.js';
import type { DeliveryAttemptStatus } from '../domain/types/vocabulary/delivery-attempt-status.js';
import { PG_POOL } from './pg-pool.token.js';

interface DeliveryAttemptRow extends QueryResultRow {
  id: string;
  subscription_id: string;
  merge_event_id: string;
  branch_id: string;
  merged_at: Date;
  status: string;
  attempt_count: number;
  last_attempted_at: Date | null;
  next_retry_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

function toDeliveryAttempt(row: DeliveryAttemptRow): DeliveryAttempt {
  return new DeliveryAttempt({
    id: row.id,
    subscriptionId: row.subscription_id,
    mergeEventId: row.merge_event_id,
    branchId: row.branch_id,
    mergedAt: row.merged_at,
    status: row.status as DeliveryAttemptStatus,
    attemptCount: row.attempt_count,
    lastAttemptedAt: row.last_attempted_at,
    nextRetryAt: row.next_retry_at,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  });
}

export interface CreatePendingDeliveryAttemptParams {
  subscriptionId: string;
  mergeEventId: string;
  branchId: string;
  mergedAt: Date;
}

/**
 * Postgres-backed repository for the DeliveryAttempt aggregate (Meridian IDEA-127, amended by
 * IDEA-129, gap-resolved by IDEA-132/goal G14 OQ2). `createPending` is called from
 * `BranchRepository.merge()`'s fan-out transaction (G14 SG2); `claimBatch`/`markSucceeded`/
 * `markFailed` are called from the in-process `DeliveryWorkerService` poll loop (G14 SG3).
 *
 * Retry/terminal policy ownership: this repository only exposes state-transition primitives. It
 * does NOT decide when a failure is terminal vs retry-eligible -- `markFailed`'s caller (the
 * SG3 worker) must inspect the claimed row's `attemptCount` (incremented by `claimBatch`) and
 * pass `nextRetryAt: null` once the backoff schedule (2s, 4s, then terminal) is exhausted,
 * otherwise pass the next backoff timestamp. This keeps the backoff schedule itself out of the
 * persistence layer.
 *
 * No crash-recovery reclaim of rows stranded `in_progress` by a worker crash is implemented here
 * (interim scoping per IDEA-132/OQ2, user-approved; revisit before any multi-instance deployment
 * per the forward-looking IDEA-133 spike).
 */
@Injectable()
export class DeliveryAttemptRepository {
  constructor(@Inject(PG_POOL) private readonly pool: Pool) {}

  /**
   * Inserts a new pending delivery attempt. Idempotent on `(subscription_id, merge_event_id)`
   * via `ON CONFLICT DO NOTHING` -- returns `undefined` when a row for that pair already exists,
   * which is a valid, error-free outcome (not a failure), consistent with IDEA-127's
   * idempotency-key guarantee. Callers that need the existing row must look it up separately;
   * the fan-out path (G14 SG2) only needs the idempotent-insert behavior itself.
   */
  async createPending(
    params: CreatePendingDeliveryAttemptParams,
    client?: PoolClient,
  ): Promise<DeliveryAttempt | undefined> {
    const result: QueryResult<DeliveryAttemptRow> = await (client ?? this.pool).query<DeliveryAttemptRow>(
      `INSERT INTO delivery_attempts (
         id, subscription_id, merge_event_id, branch_id, merged_at, status, attempt_count,
         created_at, updated_at
       ) VALUES (gen_random_uuid(), $1, $2, $3, $4, 'pending', 0, now(), now())
       ON CONFLICT (subscription_id, merge_event_id) DO NOTHING
       RETURNING *`,
      [params.subscriptionId, params.mergeEventId, params.branchId, params.mergedAt],
    );

    const row = result.rows[0];
    return row === undefined ? undefined : toDeliveryAttempt(row);
  }

  /**
   * Claims up to `limit` due delivery attempts for outbound delivery, atomically flipping them
   * to `in_progress` and incrementing `attempt_count`. Two CTEs, per the ratified design
   * (IDEA-129's claim-queue + IDEA-132's FIFO gap resolution):
   *
   * - `candidate_ids`: dedup only, no locking clause (PostgreSQL disallows combining `DISTINCT`
   *   with `FOR UPDATE` in one `SELECT`). Wraps a nested subquery that first picks the single
   *   oldest `pending` row per subscription (`DISTINCT ON (subscription_id) ... ORDER BY
   *   subscription_id, created_at`), gated by `NOT EXISTS` an `in_progress` row for that
   *   subscription -- this is always the true oldest pending row, never a later one, regardless
   *   of whether it happens to be due yet. The outer query then filters that single
   *   oldest-per-subscription row down to ones actually due
   *   (`next_retry_at IS NULL OR next_retry_at <= now()`). A subscription whose oldest pending
   *   row isn't due yet yields NO candidate this tick -- it never falls through to claim a
   *   newer, already-due row out of order (the bug a plain due-time-first filter would have).
   * - `locked_ids`: locking only, no `DISTINCT` (already deduplicated by `candidate_ids`) --
   *   `FOR UPDATE SKIP LOCKED` plus `LIMIT` over `candidate_ids`.
   *
   * The final `UPDATE` re-checks `AND status = 'pending'` so a row a concurrent claimer already
   * flipped to `in_progress` between the CTE snapshot and this statement is never re-claimed.
   */
  async claimBatch(limit: number): Promise<DeliveryAttempt[]> {
    const result: QueryResult<DeliveryAttemptRow> = await this.pool.query<DeliveryAttemptRow>(
      `WITH candidate_ids AS (
         SELECT id FROM (
           SELECT DISTINCT ON (subscription_id) id, next_retry_at
           FROM delivery_attempts
           WHERE status = 'pending'
             AND NOT EXISTS (
               SELECT 1 FROM delivery_attempts in_progress_rows
               WHERE in_progress_rows.subscription_id = delivery_attempts.subscription_id
                 AND in_progress_rows.status = 'in_progress'
             )
           ORDER BY subscription_id, created_at ASC
         ) oldest_pending_per_subscription
         WHERE next_retry_at IS NULL OR next_retry_at <= now()
       ),
       locked_ids AS (
         SELECT id FROM delivery_attempts
         WHERE id IN (SELECT id FROM candidate_ids)
         ORDER BY created_at ASC
         LIMIT $1
         FOR UPDATE SKIP LOCKED
       )
       UPDATE delivery_attempts
       SET status = 'in_progress',
           attempt_count = attempt_count + 1,
           last_attempted_at = now(),
           updated_at = now()
       WHERE id IN (SELECT id FROM locked_ids)
         AND status = 'pending'
       RETURNING *`,
      [limit],
    );

    return result.rows.map(toDeliveryAttempt);
  }

  /**
   * Marks a claimed attempt succeeded. Guarded by `AND status = 'in_progress'` so a stale or
   * duplicate completion callback can never mutate a row that isn't currently claimed (already
   * terminal, or never claimed) -- returns `undefined` in that case.
   */
  async markSucceeded(id: string): Promise<DeliveryAttempt | undefined> {
    const result: QueryResult<DeliveryAttemptRow> = await this.pool.query<DeliveryAttemptRow>(
      `UPDATE delivery_attempts
       SET status = 'succeeded', updated_at = now()
       WHERE id = $1 AND status = 'in_progress'
       RETURNING *`,
      [id],
    );

    const row = result.rows[0];
    return row === undefined ? undefined : toDeliveryAttempt(row);
  }

  /**
   * Marks a claimed attempt failed. Guarded by `AND status = 'in_progress'`, same rationale as
   * `markSucceeded`. `nextRetryAt` of `null` terminalizes the row to `failed` (the caller has
   * exhausted the backoff schedule); any other `Date` requeues it to `pending` with
   * `next_retry_at` set, making it eligible for a future `claimBatch` call once due.
   */
  async markFailed(id: string, nextRetryAt: Date | null): Promise<DeliveryAttempt | undefined> {
    const result: QueryResult<DeliveryAttemptRow> =
      nextRetryAt === null
        ? await this.pool.query<DeliveryAttemptRow>(
            `UPDATE delivery_attempts
             SET status = 'failed', updated_at = now()
             WHERE id = $1 AND status = 'in_progress'
             RETURNING *`,
            [id],
          )
        : await this.pool.query<DeliveryAttemptRow>(
            `UPDATE delivery_attempts
             SET status = 'pending', next_retry_at = $2, updated_at = now()
             WHERE id = $1 AND status = 'in_progress'
             RETURNING *`,
            [id, nextRetryAt],
          );

    const row = result.rows[0];
    return row === undefined ? undefined : toDeliveryAttempt(row);
  }
}
