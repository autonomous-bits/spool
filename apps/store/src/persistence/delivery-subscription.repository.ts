/**
 * Postgres-backed persistence adapter for downstream push-delivery
 * subscriptions (story S08).
 *
 * Sources of authority:
 * - Story S08 AC1: a downstream consumer's delivery preferences, including
 *   any discipline filter, remain registered across sessions without
 *   needing to be re-submitted.
 * - Technical spec §"Delivery subscription persistence" (`IDEA-65`):
 *   "Downstream push consumers and their discipline filters must be
 *   persisted as durable subscription records scoped to a workspace,
 *   independent of any single delivery attempt." This repository has no
 *   notion of a delivery attempt at all — it only ever reads/writes the
 *   subscriber's own registered preferences.
 * - Meridian `IDEA-65` (verified via `meridian-get-chunk` against workspace
 *   `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`): "Downstream Push consumers are
 *   tracked via a dedicated `delivery_subscriptions` database table,
 *   registering webhooks and optional discipline filters."
 * - Technical spec §"Tenant isolation": every method takes a `workspaceId`
 *   and every SQL predicate filters on it explicitly.
 * - nestjs-security skill: parameterized queries only, never string
 *   concatenation.
 */

import { Inject, Injectable } from '@nestjs/common';
import type { Pool } from 'pg';
import type { ConsumerId, Discipline, WorkspaceId } from '../domain/types/index.js';
import { PG_POOL } from './database-pool.provider.js';

export interface PersistedDeliverySubscription {
  readonly workspaceId: WorkspaceId;
  readonly consumerId: ConsumerId;
  readonly webhookUrl: string;
  /** `undefined` means no filter — every discipline is delivered. */
  readonly disciplines?: Discipline[];
  readonly createdAt: string;
  readonly updatedAt: string;
}

export interface DeliverySubscriptionInput {
  readonly workspaceId: WorkspaceId;
  readonly consumerId: ConsumerId;
  readonly webhookUrl: string;
  readonly disciplines?: Discipline[];
}

interface DeliverySubscriptionRow {
  readonly workspace_id: string;
  readonly consumer_id: string;
  readonly webhook_url: string;
  readonly disciplines: string[] | null;
  readonly created_at: string | Date;
  readonly updated_at: string | Date;
}

const SUBSCRIPTION_COLUMNS = `workspace_id, consumer_id, webhook_url, disciplines,
       created_at, updated_at`;

function toIsoString(value: string | Date): string {
  return value instanceof Date ? value.toISOString() : value;
}

function rowToPersistedSubscription(row: DeliverySubscriptionRow): PersistedDeliverySubscription {
  const base: PersistedDeliverySubscription = {
    workspaceId: row.workspace_id as WorkspaceId,
    consumerId: row.consumer_id as ConsumerId,
    webhookUrl: row.webhook_url,
    createdAt: toIsoString(row.created_at),
    updatedAt: toIsoString(row.updated_at),
  };
  return row.disciplines && row.disciplines.length > 0
    ? { ...base, disciplines: row.disciplines as Discipline[] }
    : base;
}

/** `undefined`/empty array is stored as SQL `NULL` (no filter). */
function toDisciplinesColumn(disciplines: Discipline[] | undefined): Discipline[] | null {
  return disciplines && disciplines.length > 0 ? disciplines : null;
}

@Injectable()
export class DeliverySubscriptionRepository {
  constructor(@Inject(PG_POOL) private readonly pool: Pool) {}

  /**
   * Registers a downstream consumer's delivery preferences, or updates them
   * in place if the consumer is already registered in this workspace (AC1:
   * "remain registered across sessions without needing to be
   * re-submitted" — re-submission is safe and idempotent, never a
   * duplicate/error).
   */
  async registerSubscription(
    input: DeliverySubscriptionInput,
  ): Promise<PersistedDeliverySubscription> {
    const result = await this.pool.query<DeliverySubscriptionRow>(
      `INSERT INTO delivery_subscriptions (workspace_id, consumer_id, webhook_url, disciplines)
       VALUES ($1, $2, $3, $4)
       ON CONFLICT (workspace_id, consumer_id)
       DO UPDATE SET webhook_url = EXCLUDED.webhook_url,
                     disciplines = EXCLUDED.disciplines,
                     updated_at = now()
       RETURNING ${SUBSCRIPTION_COLUMNS}`,
      [input.workspaceId, input.consumerId, input.webhookUrl, toDisciplinesColumn(input.disciplines)],
    );
    return rowToPersistedSubscription(result.rows[0]!);
  }

  async getSubscription(
    workspaceId: WorkspaceId,
    consumerId: ConsumerId,
  ): Promise<PersistedDeliverySubscription | undefined> {
    const result = await this.pool.query<DeliverySubscriptionRow>(
      `SELECT ${SUBSCRIPTION_COLUMNS} FROM delivery_subscriptions
       WHERE workspace_id = $1 AND consumer_id = $2`,
      [workspaceId, consumerId],
    );
    const row = result.rows[0];
    return row ? rowToPersistedSubscription(row) : undefined;
  }

  /** Lists every subscription registered in a workspace (AC3-adjacent: this read never depends on delivery/push state). */
  async listSubscriptions(workspaceId: WorkspaceId): Promise<PersistedDeliverySubscription[]> {
    const result = await this.pool.query<DeliverySubscriptionRow>(
      `SELECT ${SUBSCRIPTION_COLUMNS} FROM delivery_subscriptions
       WHERE workspace_id = $1
       ORDER BY consumer_id`,
      [workspaceId],
    );
    return result.rows.map(rowToPersistedSubscription);
  }

  /**
   * Lists the subscriptions in a workspace eligible to receive an update for
   * a given discipline: a subscription with no discipline filter (`NULL`)
   * matches every discipline; a subscription with a filter matches only if
   * the array contains the requested discipline.
   */
  async listSubscriptionsForDiscipline(
    workspaceId: WorkspaceId,
    discipline: Discipline,
  ): Promise<PersistedDeliverySubscription[]> {
    const result = await this.pool.query<DeliverySubscriptionRow>(
      `SELECT ${SUBSCRIPTION_COLUMNS} FROM delivery_subscriptions
       WHERE workspace_id = $1
         AND (disciplines IS NULL OR $2 = ANY(disciplines))
       ORDER BY consumer_id`,
      [workspaceId, discipline],
    );
    return result.rows.map(rowToPersistedSubscription);
  }

  /** Removes a consumer's subscription. Safe to call for an already-removed/unknown consumer (no-op). */
  async removeSubscription(workspaceId: WorkspaceId, consumerId: ConsumerId): Promise<void> {
    await this.pool.query(
      `DELETE FROM delivery_subscriptions WHERE workspace_id = $1 AND consumer_id = $2`,
      [workspaceId, consumerId],
    );
  }
}
