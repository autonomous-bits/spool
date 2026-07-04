/**
 * Adapter-level integration test proving durable, workspace-scoped
 * downstream delivery subscription persistence (story S08) against a real
 * containerized Postgres.
 *
 * Technical spec §"Testing expectations" requires "delivery subscription
 * persistence" and "tenant/workspace isolation" to be proven at the
 * adapter level against a real containerized Postgres, not an in-memory
 * substitute. Start it locally before running this file:
 *
 *   docker compose up -d postgres
 *
 * and export the matching connection env vars (see apps/store/AGENTS.md and
 * config/store.env.example), e.g.:
 *
 *   export STORE_DB_HOST=localhost STORE_DB_PORT=5433 \
 *     STORE_DB_USER=spool STORE_DB_PASSWORD=spool_dev STORE_DB_NAME=spool
 */

import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { Pool } from 'pg';
import { randomUUID } from 'node:crypto';
import { loadDatabaseConfig } from '../src/persistence/database-config.js';
import { ensureSchema } from '../src/persistence/schema.js';
import { DeliverySubscriptionRepository } from '../src/persistence/delivery-subscription.repository.js';
import { workspaceId, consumerId } from '../src/domain/types/index.js';

function openPool(): Pool {
  const config = loadDatabaseConfig();
  return new Pool({
    host: config.host,
    port: config.port,
    user: config.user,
    password: config.password,
    database: config.database,
  });
}

const hasDatabaseConfig = [
  'STORE_DB_HOST',
  'STORE_DB_PORT',
  'STORE_DB_USER',
  'STORE_DB_PASSWORD',
  'STORE_DB_NAME',
].every((key) => Boolean(process.env[key]?.trim()));

describe.skipIf(!hasDatabaseConfig)(
  'DeliverySubscriptionRepository (Postgres adapter, downstream delivery subscription persistence)',
  () => {
    const workspaceA = workspaceId(`ws-${randomUUID()}`);
    const workspaceB = workspaceId(`ws-${randomUUID()}`);

    beforeAll(async () => {
      const bootstrapPool = openPool();
      await ensureSchema(bootstrapPool);
      await bootstrapPool.end();
    });

    afterAll(async () => {
      const cleanupPool = openPool();
      await cleanupPool.query(
        'DELETE FROM delivery_subscriptions WHERE workspace_id = $1 OR workspace_id = $2',
        [workspaceA, workspaceB],
      );
      await cleanupPool.end();
    });

    it('AC1: a registered subscription, including its discipline filter, persists and can be read back', async () => {
      const pool = openPool();
      const repo = new DeliverySubscriptionRepository(pool);
      const id = consumerId(`consumer-${randomUUID()}`);

      const created = await repo.registerSubscription({
        workspaceId: workspaceA,
        consumerId: id,
        webhookUrl: 'https://example.test/hooks/downstream',
        disciplines: ['engineering', 'architecture'],
      });

      const fetched = await repo.getSubscription(workspaceA, id);
      await pool.end();

      expect(created.disciplines).toEqual(['engineering', 'architecture']);
      expect(fetched?.webhookUrl).toBe('https://example.test/hooks/downstream');
      expect(fetched?.disciplines).toEqual(['engineering', 'architecture']);
    });

    it('AC1: a subscription registered without a discipline filter matches every discipline', async () => {
      const pool = openPool();
      const repo = new DeliverySubscriptionRepository(pool);
      const id = consumerId(`consumer-${randomUUID()}`);

      await repo.registerSubscription({
        workspaceId: workspaceA,
        consumerId: id,
        webhookUrl: 'https://example.test/hooks/all-disciplines',
      });

      const engineering = await repo.listSubscriptionsForDiscipline(workspaceA, 'engineering');
      const design = await repo.listSubscriptionsForDiscipline(workspaceA, 'design');
      const fetched = await repo.getSubscription(workspaceA, id);
      await pool.end();

      expect(engineering.some((s) => s.consumerId === id)).toBe(true);
      expect(design.some((s) => s.consumerId === id)).toBe(true);
      expect(fetched?.disciplines).toBeUndefined();
    });

    it('listSubscriptionsForDiscipline excludes a subscription whose discipline filter does not include the requested discipline', async () => {
      const pool = openPool();
      const repo = new DeliverySubscriptionRepository(pool);
      const id = consumerId(`consumer-${randomUUID()}`);

      await repo.registerSubscription({
        workspaceId: workspaceA,
        consumerId: id,
        webhookUrl: 'https://example.test/hooks/engineering-only',
        disciplines: ['engineering'],
      });

      const engineering = await repo.listSubscriptionsForDiscipline(workspaceA, 'engineering');
      const product = await repo.listSubscriptionsForDiscipline(workspaceA, 'product');
      await pool.end();

      expect(engineering.some((s) => s.consumerId === id)).toBe(true);
      expect(product.some((s) => s.consumerId === id)).toBe(false);
    });

    it('AC1: re-registering the same consumer updates preferences in place rather than duplicating or erroring', async () => {
      const pool = openPool();
      const repo = new DeliverySubscriptionRepository(pool);
      const id = consumerId(`consumer-${randomUUID()}`);

      await repo.registerSubscription({
        workspaceId: workspaceA,
        consumerId: id,
        webhookUrl: 'https://example.test/hooks/v1',
        disciplines: ['engineering'],
      });
      const updated = await repo.registerSubscription({
        workspaceId: workspaceA,
        consumerId: id,
        webhookUrl: 'https://example.test/hooks/v2',
        disciplines: ['design'],
      });

      const all = await repo.listSubscriptions(workspaceA);
      await pool.end();

      const matches = all.filter((s) => s.consumerId === id);
      expect(matches).toHaveLength(1);
      expect(updated.webhookUrl).toBe('https://example.test/hooks/v2');
      expect(updated.disciplines).toEqual(['design']);
    });

    it('tenant isolation: a subscription in workspace A is not visible via workspace B reads', async () => {
      const pool = openPool();
      const repo = new DeliverySubscriptionRepository(pool);
      const id = consumerId(`consumer-${randomUUID()}`);

      await repo.registerSubscription({
        workspaceId: workspaceA,
        consumerId: id,
        webhookUrl: 'https://example.test/hooks/isolated',
      });

      const crossWorkspaceGet = await repo.getSubscription(workspaceB, id);
      const workspaceBList = await repo.listSubscriptions(workspaceB);
      const workspaceBDisciplineList = await repo.listSubscriptionsForDiscipline(
        workspaceB,
        'engineering',
      );
      await pool.end();

      expect(crossWorkspaceGet).toBeUndefined();
      expect(workspaceBList.some((s) => s.consumerId === id)).toBe(false);
      expect(workspaceBDisciplineList.some((s) => s.consumerId === id)).toBe(false);
    });

    it('AC3: subscription reads succeed independent of whether any push delivery has ever been attempted', async () => {
      // This repository has no delivery-attempt/outcome column at all, so a
      // freshly registered subscription (never delivered to) reads back
      // identically to any other — proving pull-style access to a
      // consumer's own registration never depends on push success.
      const pool = openPool();
      const repo = new DeliverySubscriptionRepository(pool);
      const id = consumerId(`consumer-${randomUUID()}`);

      await repo.registerSubscription({
        workspaceId: workspaceA,
        consumerId: id,
        webhookUrl: 'https://example.test/hooks/never-delivered',
      });

      const fetched = await repo.getSubscription(workspaceA, id);
      await pool.end();

      expect(fetched).toBeDefined();
      expect(fetched?.consumerId).toBe(id);
    });

    it('tenant isolation (S10): the same consumerId registered in two workspaces stays two independent subscriptions', async () => {
      const pool = openPool();
      const repo = new DeliverySubscriptionRepository(pool);
      const sharedId = consumerId(`consumer-${randomUUID()}`);

      await repo.registerSubscription({
        workspaceId: workspaceA,
        consumerId: sharedId,
        webhookUrl: 'https://example.test/hooks/workspace-a',
        disciplines: ['engineering'],
      });
      await repo.registerSubscription({
        workspaceId: workspaceB,
        consumerId: sharedId,
        webhookUrl: 'https://example.test/hooks/workspace-b',
        disciplines: ['design'],
      });

      const fetchedA = await repo.getSubscription(workspaceA, sharedId);
      const fetchedB = await repo.getSubscription(workspaceB, sharedId);
      await pool.end();

      // Registering under B must not have overwritten or merged with A's row.
      expect(fetchedA?.webhookUrl).toBe('https://example.test/hooks/workspace-a');
      expect(fetchedA?.disciplines).toEqual(['engineering']);
      expect(fetchedB?.webhookUrl).toBe('https://example.test/hooks/workspace-b');
      expect(fetchedB?.disciplines).toEqual(['design']);
    });

    it('removeSubscription deletes a registration; a subsequent read finds nothing', async () => {
      const pool = openPool();
      const repo = new DeliverySubscriptionRepository(pool);
      const id = consumerId(`consumer-${randomUUID()}`);

      await repo.registerSubscription({
        workspaceId: workspaceA,
        consumerId: id,
        webhookUrl: 'https://example.test/hooks/removable',
      });
      await repo.removeSubscription(workspaceA, id);

      const fetched = await repo.getSubscription(workspaceA, id);
      await pool.end();

      expect(fetched).toBeUndefined();
    });
  },
);
