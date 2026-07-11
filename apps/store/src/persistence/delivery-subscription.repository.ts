import type { Pool, QueryResult, QueryResultRow } from 'pg';
import { Inject, Injectable } from '@nestjs/common';
import { DeliverySubscription } from '../domain/delivery-subscription.js';
import type { Discipline } from '../domain/types/vocabulary/discipline.js';
import { PG_POOL } from './pg-pool.token.js';

interface DeliverySubscriptionRow extends QueryResultRow {
  id: string;
  workspace_id: string;
  url: string;
  discipline_filter: Discipline[] | null;
  signing_secret: string;
  is_active: boolean;
  created_by_stakeholder_id: string;
  created_at: Date;
  updated_at: Date;
}

function toDeliverySubscription(row: DeliverySubscriptionRow): DeliverySubscription {
  return new DeliverySubscription({
    id: row.id,
    workspaceId: row.workspace_id,
    url: row.url,
    ...(row.discipline_filter === null ? {} : { disciplineFilter: row.discipline_filter }),
    signingSecret: row.signing_secret,
    isActive: row.is_active,
    createdByStakeholderId: row.created_by_stakeholder_id,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  });
}

/**
 * Postgres-backed repository for the DeliverySubscription aggregate (Meridian IDEA-63/IDEA-65/
 * IDEA-104, G13 SG1). `findById` and `deactivate` are always scoped by BOTH `id` AND
 * `workspace_id` in the same WHERE clause — this is a tenant-isolation requirement, not just a
 * convenience filter, so a member of workspace A can never discover, read, or deactivate a
 * subscription belonging to workspace B. Deactivation is a soft-delete (`is_active = false`),
 * consistent with the codebase's non-destructive-delete precedent (e.g. edges' superseding
 * instead of deleting, IDEA-26/IDEA-38).
 */
@Injectable()
export class DeliverySubscriptionRepository {
  constructor(@Inject(PG_POOL) private readonly pool: Pool) {}

  /**
   * Persists a newly-constructed DeliverySubscription and returns the persisted entity
   * (round-tripped from the database row, not the in-memory instance).
   */
  async create(subscription: DeliverySubscription): Promise<DeliverySubscription> {
    const result: QueryResult<DeliverySubscriptionRow> = await this.pool.query<DeliverySubscriptionRow>(
      `INSERT INTO delivery_subscriptions (
         id, workspace_id, url, discipline_filter, signing_secret, is_active,
         created_by_stakeholder_id, created_at, updated_at
       ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
       RETURNING *`,
      [
        subscription.id,
        subscription.workspaceId,
        subscription.url,
        subscription.disciplineFilter === undefined
          ? null
          : JSON.stringify(subscription.disciplineFilter),
        subscription.signingSecret,
        subscription.isActive,
        subscription.createdByStakeholderId,
        subscription.createdAt,
        subscription.updatedAt,
      ],
    );

    const row = result.rows[0];
    if (row === undefined) {
      throw new Error('DeliverySubscriptionRepository.create: INSERT ... RETURNING * produced no row');
    }

    return toDeliverySubscription(row);
  }

  /**
   * Lists every subscription (active and inactive) registered for a workspace, most recently
   * created first.
   */
  async listByWorkspace(workspaceId: string): Promise<DeliverySubscription[]> {
    const result: QueryResult<DeliverySubscriptionRow> = await this.pool.query<DeliverySubscriptionRow>(
      'SELECT * FROM delivery_subscriptions WHERE workspace_id = $1 ORDER BY created_at DESC',
      [workspaceId],
    );

    return result.rows.map(toDeliverySubscription);
  }

  /**
   * Looks up a subscription by id, scoped to the given workspace. Returns `undefined` if the id
   * is unknown OR belongs to a different workspace — the two cases are indistinguishable by
   * design, so a caller can never learn that a subscription id exists in another workspace.
   */
  async findById(id: string, workspaceId: string): Promise<DeliverySubscription | undefined> {
    const result: QueryResult<DeliverySubscriptionRow> = await this.pool.query<DeliverySubscriptionRow>(
      'SELECT * FROM delivery_subscriptions WHERE id = $1 AND workspace_id = $2',
      [id, workspaceId],
    );

    const row = result.rows[0];
    return row === undefined ? undefined : toDeliverySubscription(row);
  }

  /**
   * Internal-only lookup by subscription id with NO workspace filter. This intentionally bypasses
   * the normal tenant-scoped read path and exists only for trusted in-process infrastructure code
   * (the delivery worker) that already obtained the id from a delivery_attempts row created by the
   * tenant-scoped merge fan-out. Never expose this through controllers or MCP tools.
   */
  async findByIdUnscoped(id: string): Promise<DeliverySubscription | undefined> {
    const result: QueryResult<DeliverySubscriptionRow> = await this.pool.query<DeliverySubscriptionRow>(
      'SELECT * FROM delivery_subscriptions WHERE id = $1',
      [id],
    );

    const row = result.rows[0];
    return row === undefined ? undefined : toDeliverySubscription(row);
  }

  /**
   * Soft-deletes (`is_active = false`) a subscription, scoped to the given workspace. Returns the
   * updated subscription, or `undefined` if the id is unknown or belongs to a different workspace
   * (same indistinguishable-by-design behavior as `findById`).
   */
  async deactivate(id: string, workspaceId: string): Promise<DeliverySubscription | undefined> {
    const result: QueryResult<DeliverySubscriptionRow> = await this.pool.query<DeliverySubscriptionRow>(
      `UPDATE delivery_subscriptions
       SET is_active = false, updated_at = clock_timestamp()
       WHERE id = $1 AND workspace_id = $2
       RETURNING *`,
      [id, workspaceId],
    );

    const row = result.rows[0];
    return row === undefined ? undefined : toDeliverySubscription(row);
  }
}
