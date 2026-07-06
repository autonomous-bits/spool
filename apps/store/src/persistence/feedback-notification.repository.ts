import type { Pool, QueryResult, QueryResultRow } from 'pg';
import { Inject, Injectable } from '@nestjs/common';
import { FeedbackNotification } from '../domain/feedback-notification.js';
import type { NotificationStatus } from '../domain/types/vocabulary/notification-status.js';
import { PG_POOL } from './pg-pool.token.js';

export interface FeedbackNotificationRow extends QueryResultRow {
  id: string;
  branch_id: string;
  stakeholder_id: string;
  signal_id: string;
  status: string;
  created_at: Date;
  updated_at: Date;
}

export function toFeedbackNotification(row: FeedbackNotificationRow): FeedbackNotification {
  return new FeedbackNotification({
    id: row.id,
    branchId: row.branch_id,
    stakeholderId: row.stakeholder_id,
    signalId: row.signal_id,
    status: row.status as NotificationStatus,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  });
}

/**
 * Postgres-backed repository for the FeedbackNotification aggregate (Meridian IDEA-31's
 * authoritative schema). Rows are only ever created by `VerificationSignalRepository.create`'s
 * fan-out transaction (G09 SG2); this repository only reads and marks-as-read (G09 SG3).
 */
@Injectable()
export class FeedbackNotificationRepository {
  constructor(@Inject(PG_POOL) private readonly pool: Pool) {}

  /**
   * Lists a single stakeholder's notifications, newest first (G09 SG3). Always scoped by
   * `stakeholderId` -- callers must derive it from verified session-token claims, never a
   * client-supplied id, so one stakeholder can never list another's notifications.
   */
  async findByStakeholderId(
    stakeholderId: string,
    status?: NotificationStatus,
  ): Promise<FeedbackNotification[]> {
    const result: QueryResult<FeedbackNotificationRow> =
      status === undefined
        ? await this.pool.query<FeedbackNotificationRow>(
            'SELECT * FROM feedback_notifications WHERE stakeholder_id = $1 ORDER BY created_at DESC, id DESC',
            [stakeholderId],
          )
        : await this.pool.query<FeedbackNotificationRow>(
            'SELECT * FROM feedback_notifications WHERE stakeholder_id = $1 AND status = $2 ORDER BY created_at DESC, id DESC',
            [stakeholderId, status],
          );

    return result.rows.map(toFeedbackNotification);
  }

  /**
   * Marks a single notification read, scoped to `stakeholderId` in the same query as the id
   * lookup (G09 SG3) -- returns `undefined` for both "no such notification" and "exists but
   * belongs to another stakeholder", so the service layer maps both to a single 404 without
   * leaking which case occurred. Idempotent: re-marking an already-read row simply reapplies
   * `status = 'read'` and refreshes `updated_at`.
   */
  async markAsRead(id: string, stakeholderId: string): Promise<FeedbackNotification | undefined> {
    const result: QueryResult<FeedbackNotificationRow> = await this.pool.query<FeedbackNotificationRow>(
      `UPDATE feedback_notifications
       SET status = 'read', updated_at = now()
       WHERE id = $1 AND stakeholder_id = $2
       RETURNING *`,
      [id, stakeholderId],
    );

    const row = result.rows[0];
    return row === undefined ? undefined : toFeedbackNotification(row);
  }
}
