import { randomUUID } from 'node:crypto';
import type { Pool } from 'pg';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { DeliverySubscription } from '../../src/domain/delivery-subscription.js';
import { Workspace } from '../../src/domain/workspace.js';
import { BOOTSTRAP_STAKEHOLDER_ID } from '../../src/persistence/bootstrap-stakeholder.js';
import { DeliveryAttemptRepository } from '../../src/persistence/delivery-attempt.repository.js';
import { DeliverySubscriptionRepository } from '../../src/persistence/delivery-subscription.repository.js';
import { WorkspaceRepository } from '../../src/persistence/workspace.repository.js';
import { setUpTestDatabase, type TestDatabase } from '../support/test-database.js';

const FULL_QUEUE_CLAIM_LIMIT = 1_000_000;

describe('DeliveryAttemptRepository (containerized Postgres)', () => {
  let database: TestDatabase;
  let pool: Pool;
  let repository: DeliveryAttemptRepository;
  let subscriptionRepository: DeliverySubscriptionRepository;
  let workspace: Workspace;

  beforeAll(async () => {
    database = await setUpTestDatabase();
    pool = database.pool;
    repository = new DeliveryAttemptRepository(pool);
    subscriptionRepository = new DeliverySubscriptionRepository(pool);

    const workspaceRepository = new WorkspaceRepository(pool);
    workspace = await workspaceRepository.createWithFirstMember(
      new Workspace({
        name: `delivery-attempt-workspace-${Math.random().toString(36).slice(2, 10)}`,
        createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      }),
    );
  });

  afterAll(async () => {
    await database.close();
  });

  async function seedSubscription(): Promise<string> {
    const created = await subscriptionRepository.create(
      new DeliverySubscription({
        workspaceId: workspace.id,
        url: 'https://example.com/webhook',
        createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      }),
    );
    return created.id;
  }

  async function seedPending(
    subscriptionId: string,
    overrides: { nextRetryAt?: Date | null; createdAt?: Date } = {},
  ): Promise<string> {
    const id = randomUUID();
    await pool.query(
      `INSERT INTO delivery_attempts (
         id, subscription_id, merge_event_id, branch_id, merged_at, status, attempt_count,
         next_retry_at, created_at, updated_at
       ) VALUES ($1, $2, $3, $4, now(), 'pending', 0, $5, $6, $6)`,
      [
        id,
        subscriptionId,
        randomUUID(),
        randomUUID(),
        overrides.nextRetryAt ?? null,
        overrides.createdAt ?? new Date(),
      ],
    );
    return id;
  }

  async function claimAttempt(id: string): Promise<void> {
    const claimed = await repository.claimBatch(FULL_QUEUE_CLAIM_LIMIT);
    expect(claimed.find((attempt) => attempt.id === id)).toBeDefined();
  }

  describe('createPending', () => {
    it('inserts a new pending delivery attempt', async () => {
      const subscriptionId = await seedSubscription();
      const branchId = randomUUID();
      const mergeEventId = randomUUID();
      const mergedAt = new Date();

      const created = await repository.createPending({
        subscriptionId,
        mergeEventId,
        branchId,
        mergedAt,
      });

      expect(created).toBeDefined();
      expect(created?.subscriptionId).toBe(subscriptionId);
      expect(created?.mergeEventId).toBe(mergeEventId);
      expect(created?.branchId).toBe(branchId);
      expect(created?.status).toBe('pending');
      expect(created?.attemptCount).toBe(0);
    });

    it('is idempotent: a duplicate (subscriptionId, mergeEventId) is a no-op returning undefined', async () => {
      const subscriptionId = await seedSubscription();
      const mergeEventId = randomUUID();
      const params = { subscriptionId, mergeEventId, branchId: randomUUID(), mergedAt: new Date() };

      const first = await repository.createPending(params);
      const second = await repository.createPending(params);

      expect(first).toBeDefined();
      expect(second).toBeUndefined();

      const rows = await pool.query(
        'SELECT id FROM delivery_attempts WHERE subscription_id = $1 AND merge_event_id = $2',
        [subscriptionId, mergeEventId],
      );
      expect(rows.rows).toHaveLength(1);
    });
  });

  describe('claimBatch', () => {
    it('claims a due pending row, setting in_progress/attempt_count/last_attempted_at', async () => {
      const subscriptionId = await seedSubscription();
      const id = await seedPending(subscriptionId);

      const claimed = await repository.claimBatch(FULL_QUEUE_CLAIM_LIMIT);
      const ours = claimed.find((a) => a.id === id);

      expect(ours).toBeDefined();
      expect(ours?.status).toBe('in_progress');
      expect(ours?.attemptCount).toBe(1);
      expect(ours?.lastAttemptedAt).not.toBeNull();
    });

    it('does not claim a row whose next_retry_at is in the future', async () => {
      const subscriptionId = await seedSubscription();
      const future = new Date(Date.now() + 60_000);
      const id = await seedPending(subscriptionId, { nextRetryAt: future });

      const claimed = await repository.claimBatch(FULL_QUEUE_CLAIM_LIMIT);

      expect(claimed.map((a) => a.id)).not.toContain(id);
    });

    it('FIFO: an older not-yet-due row blocks a newer due row for the same subscription', async () => {
      const subscriptionId = await seedSubscription();
      const olderNotDueId = await seedPending(subscriptionId, {
        nextRetryAt: new Date(Date.now() + 60_000),
        createdAt: new Date(Date.now() - 10_000),
      });
      const newerDueId = await seedPending(subscriptionId, {
        nextRetryAt: null,
        createdAt: new Date(),
      });

      const claimed = await repository.claimBatch(FULL_QUEUE_CLAIM_LIMIT);
      const claimedIds = claimed.map((a) => a.id);

      // The older row isn't due yet, so it never falls through to let the newer row jump the
      // queue -- this subscription yields no candidate at all this tick (the FIFO invariant
      // from Meridian IDEA-132/goal OQ2).
      expect(claimedIds).not.toContain(olderNotDueId);
      expect(claimedIds).not.toContain(newerDueId);
    });

    it('claims only the oldest due row per subscription, never two at once', async () => {
      const subscriptionId = await seedSubscription();
      const olderId = await seedPending(subscriptionId, { createdAt: new Date(Date.now() - 5_000) });
      const newerId = await seedPending(subscriptionId, { createdAt: new Date() });

      const firstClaim = await repository.claimBatch(FULL_QUEUE_CLAIM_LIMIT);
      const firstClaimIds = firstClaim.map((a) => a.id);
      expect(firstClaimIds).toContain(olderId);
      expect(firstClaimIds).not.toContain(newerId);

      // The older row is now in_progress; a second poll tick must not claim the newer row for
      // the same subscription while one is still in flight.
      const secondClaim = await repository.claimBatch(FULL_QUEUE_CLAIM_LIMIT);
      expect(secondClaim.map((a) => a.id)).not.toContain(newerId);
    });

    it('respects the limit parameter', async () => {
      const subscriptionA = await seedSubscription();
      const subscriptionB = await seedSubscription();
      await seedPending(subscriptionA);
      await seedPending(subscriptionB);

      const claimed = await repository.claimBatch(1);

      expect(claimed).toHaveLength(1);
    });
  });

  describe('markSucceeded', () => {
    it('transitions an in_progress attempt to succeeded', async () => {
      const subscriptionId = await seedSubscription();
      const id = await seedPending(subscriptionId);
      await claimAttempt(id);

      const result = await repository.markSucceeded(id);

      expect(result?.status).toBe('succeeded');
    });

    it('returns undefined for a row that is not in_progress (e.g. still pending)', async () => {
      const subscriptionId = await seedSubscription();
      const id = await seedPending(subscriptionId);

      const result = await repository.markSucceeded(id);

      expect(result).toBeUndefined();
    });
  });

  describe('markFailed', () => {
    it('requeues to pending with next_retry_at set when given a future date (retry-eligible)', async () => {
      const subscriptionId = await seedSubscription();
      const id = await seedPending(subscriptionId);
      await claimAttempt(id);
      const retryAt = new Date(Date.now() + 2_000);

      const result = await repository.markFailed(id, retryAt);

      expect(result?.status).toBe('pending');
      expect(result?.nextRetryAt?.getTime()).toBe(retryAt.getTime());
    });

    it('terminalizes to failed when given null', async () => {
      const subscriptionId = await seedSubscription();
      const id = await seedPending(subscriptionId);
      await claimAttempt(id);

      const result = await repository.markFailed(id, null);

      expect(result?.status).toBe('failed');
    });

    it('returns undefined for a row that is not in_progress', async () => {
      const subscriptionId = await seedSubscription();
      const id = await seedPending(subscriptionId);

      const result = await repository.markFailed(id, null);

      expect(result).toBeUndefined();
    });

    it('supports a full 3-attempt retry-then-terminal sequence', async () => {
      const subscriptionId = await seedSubscription();
      const id = await seedPending(subscriptionId);
      const claimLimit = 1_000;

      const firstClaim = await repository.claimBatch(claimLimit);
      expect(firstClaim.find((a) => a.id === id)?.attemptCount).toBe(1);
      await repository.markFailed(id, new Date(Date.now() - 1)); // already due, for the test's sake

      const secondClaim = await repository.claimBatch(claimLimit);
      expect(secondClaim.find((a) => a.id === id)?.attemptCount).toBe(2);
      await repository.markFailed(id, new Date(Date.now() - 1));

      const thirdClaim = await repository.claimBatch(claimLimit);
      expect(thirdClaim.find((a) => a.id === id)?.attemptCount).toBe(3);
      const terminal = await repository.markFailed(id, null);
      expect(terminal?.status).toBe('failed');

      const fourthClaim = await repository.claimBatch(claimLimit);
      expect(fourthClaim.map((a) => a.id)).not.toContain(id);
    });
  });
});
