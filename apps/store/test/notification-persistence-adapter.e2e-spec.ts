/**
 * Adapter-level integration test proving evaluation feedback, verification
 * signals, and notification routing/acknowledgement (story S09) against a
 * real containerized Postgres.
 *
 * Technical spec §"Testing expectations" requires "notification persistence
 * and non-destructive acknowledgement" to be proven at the adapter level
 * against a real containerized Postgres, not an in-memory substitute. Start
 * it locally before running this file:
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
import { ConflictDetectionRepository } from '../src/persistence/conflict-detection.repository.js';
import { NotificationRepository } from '../src/persistence/notification.repository.js';
import { recordFeedbackItem } from '../src/domain/notification-routing.js';
import { recordVerificationSignal } from '../src/domain/verification-signal.js';
import { BranchLifecycleError } from '../src/domain/branch-lifecycle.js';
import {
  NotificationError,
  branchId,
  delegatedActor,
  feedbackItemId,
  humanActor,
  notificationId,
  stakeholderId,
  verificationSignalId,
  workspaceId,
} from '../src/domain/types/index.js';

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
  'NotificationRepository (Postgres adapter, feedback/verification-signal notification routing)',
  () => {
    const workspaceA = workspaceId(`ws-${randomUUID()}`);
    const workspaceB = workspaceId(`ws-${randomUUID()}`);
    const author = stakeholderId(`author-${randomUUID()}`);
    const otherStakeholder = stakeholderId(`other-${randomUUID()}`);
    const TS = '2026-07-04T20:00:00.000Z';
    const TS_LATER = '2026-07-04T21:00:00.000Z';

    beforeAll(async () => {
      const bootstrapPool = openPool();
      await ensureSchema(bootstrapPool);
      await bootstrapPool.end();
    });

    afterAll(async () => {
      const cleanupPool = openPool();
      await cleanupPool.query(
        'DELETE FROM notifications WHERE workspace_id = $1 OR workspace_id = $2',
        [workspaceA, workspaceB],
      );
      await cleanupPool.query(
        'DELETE FROM feedback_items WHERE workspace_id = $1 OR workspace_id = $2',
        [workspaceA, workspaceB],
      );
      await cleanupPool.query(
        'DELETE FROM verification_signals WHERE workspace_id = $1 OR workspace_id = $2',
        [workspaceA, workspaceB],
      );
      await cleanupPool.query(
        'DELETE FROM branches WHERE workspace_id = $1 OR workspace_id = $2',
        [workspaceA, workspaceB],
      );
      await cleanupPool.end();
    });

    /** Registers a fresh branch with a durable author, returning its id. */
    async function registerAuthoredBranch(pool: Pool, ws = workspaceA): Promise<ReturnType<typeof branchId>> {
      const conflicts = new ConflictDetectionRepository(pool);
      const id = branchId(`branch-${randomUUID()}`);
      await conflicts.registerBranch(ws, id, 'engineering', author);
      return id;
    }

    it('AC1: a submitted verification signal is retrievable attached to the branch it evaluated', async () => {
      const pool = openPool();
      const branch = await registerAuthoredBranch(pool);
      const repo = new NotificationRepository(pool);

      const signal = recordVerificationSignal(
        delegatedActor(stakeholderId('agent-1')),
        { workspaceId: workspaceA, branchId: branch },
        verificationSignalId(`signal-${randomUUID()}`),
        'failing',
        TS,
        'integration suite failed',
      );
      await repo.submitVerificationSignal(signal);

      const signals = await repo.listVerificationSignalsForBranch(workspaceA, branch);
      await pool.end();

      expect(signals).toHaveLength(1);
      expect(signals[0]?.outcome).toBe('failing');
      expect(signals[0]?.summary).toBe('integration suite failed');
    });

    it('AC1: submitted evaluation feedback is retrievable attached to the branch it evaluated', async () => {
      const pool = openPool();
      const branch = await registerAuthoredBranch(pool);
      const repo = new NotificationRepository(pool);

      const feedback = recordFeedbackItem(
        humanActor(author),
        { workspaceId: workspaceA, branchId: branch },
        feedbackItemId(`feedback-${randomUUID()}`),
        TS,
        'please rename this variable',
      );
      await repo.submitFeedbackItem(feedback);

      const items = await repo.listFeedbackItemsForBranch(workspaceA, branch);
      await pool.end();

      expect(items).toHaveLength(1);
      expect(items[0]?.content).toBe('please rename this variable');
    });

    it('AC2: the branch author receives a notification immediately upon feedback submission', async () => {
      const pool = openPool();
      const branch = await registerAuthoredBranch(pool);
      const repo = new NotificationRepository(pool);

      const feedback = recordFeedbackItem(
        humanActor(otherStakeholder),
        { workspaceId: workspaceA, branchId: branch },
        feedbackItemId(`feedback-${randomUUID()}`),
        TS,
        'looks good',
      );
      const { notifications } = await repo.submitFeedbackItem(feedback);

      const authorNotifications = await repo.listNotificationsForStakeholder(workspaceA, author);
      await pool.end();

      // This assertion is unconditional on any "online" concept — the
      // notification row exists synchronously right after the call, proving
      // routing does not depend on the recipient having an active session.
      expect(notifications).toHaveLength(1);
      expect(notifications[0]?.recipientStakeholderId).toBe(author);
      expect(authorNotifications.some((n) => n.notificationId === notifications[0]?.notificationId)).toBe(true);
    });

    it('AC2: additional relevant stakeholders are notified alongside the mandatory author', async () => {
      const pool = openPool();
      const branch = await registerAuthoredBranch(pool);
      const repo = new NotificationRepository(pool);

      const signal = recordVerificationSignal(
        delegatedActor(stakeholderId('agent-2')),
        { workspaceId: workspaceA, branchId: branch },
        verificationSignalId(`signal-${randomUUID()}`),
        'passing',
        TS,
        'all green',
      );
      const { notifications } = await repo.submitVerificationSignal(signal, {
        additionalStakeholderIds: [otherStakeholder],
      });
      await pool.end();

      const recipientIds = notifications.map((n) => n.recipientStakeholderId).sort();
      expect(recipientIds).toEqual([author, otherStakeholder].sort());
    });

    it('AC3: acknowledging a notification does not delete or mutate the feedback record it references', async () => {
      const pool = openPool();
      const branch = await registerAuthoredBranch(pool);
      const repo = new NotificationRepository(pool);

      const feedback = recordFeedbackItem(
        humanActor(author),
        { workspaceId: workspaceA, branchId: branch },
        feedbackItemId(`feedback-${randomUUID()}`),
        TS,
        'original feedback content',
      );
      const { notifications } = await repo.submitFeedbackItem(feedback);
      const notification = notifications[0]!;

      const acknowledged = await repo.acknowledgeNotification(workspaceA, notification.notificationId, TS_LATER);

      const itemsAfterAck = await repo.listFeedbackItemsForBranch(workspaceA, branch);
      await pool.end();

      expect(acknowledged.acknowledgedAt).toBe(TS_LATER);
      expect(itemsAfterAck).toHaveLength(1);
      expect(itemsAfterAck[0]?.content).toBe('original feedback content');
    });

    it('AC3: acknowledging is idempotent — a second acknowledgement does not overwrite the first timestamp', async () => {
      const pool = openPool();
      const branch = await registerAuthoredBranch(pool);
      const repo = new NotificationRepository(pool);

      const feedback = recordFeedbackItem(
        humanActor(author),
        { workspaceId: workspaceA, branchId: branch },
        feedbackItemId(`feedback-${randomUUID()}`),
        TS,
        'content',
      );
      const { notifications } = await repo.submitFeedbackItem(feedback);
      const notification = notifications[0]!;

      await repo.acknowledgeNotification(workspaceA, notification.notificationId, TS_LATER);
      const secondAck = await repo.acknowledgeNotification(
        workspaceA,
        notification.notificationId,
        '2026-07-04T23:00:00.000Z',
      );
      await pool.end();

      expect(secondAck.acknowledgedAt).toBe(TS_LATER);
    });

    it('AC3: acknowledging an unknown notification throws NotificationError not-found', async () => {
      const pool = openPool();
      const repo = new NotificationRepository(pool);

      await expect(
        repo.acknowledgeNotification(workspaceA, notificationId(`notif-${randomUUID()}`), TS),
      ).rejects.toMatchObject({ code: 'not-found' } satisfies Partial<NotificationError>);
      await pool.end();
    });

    it('AC4: submitting feedback/signals does not change the branch lifecycle status', async () => {
      const pool = openPool();
      const branch = await registerAuthoredBranch(pool);
      const repo = new NotificationRepository(pool);

      const beforeResult = await pool.query('SELECT status FROM branches WHERE workspace_id = $1 AND branch_id = $2', [
        workspaceA,
        branch,
      ]);

      const feedback = recordFeedbackItem(
        humanActor(author),
        { workspaceId: workspaceA, branchId: branch },
        feedbackItemId(`feedback-${randomUUID()}`),
        TS,
        'content',
      );
      await repo.submitFeedbackItem(feedback);
      const signal = recordVerificationSignal(
        humanActor(author),
        { workspaceId: workspaceA, branchId: branch },
        verificationSignalId(`signal-${randomUUID()}`),
        'failing',
        TS,
        'still failing',
      );
      await repo.submitVerificationSignal(signal);

      const afterResult = await pool.query('SELECT status FROM branches WHERE workspace_id = $1 AND branch_id = $2', [
        workspaceA,
        branch,
      ]);
      await pool.end();

      expect(afterResult.rows[0]?.status).toBe(beforeResult.rows[0]?.status);
      expect(afterResult.rows[0]?.status).toBe('draft');
    });

    it('AC5: persisted provenance always matches the authenticated actor, never a separate claim', async () => {
      const pool = openPool();
      const branch = await registerAuthoredBranch(pool);
      const repo = new NotificationRepository(pool);

      const humanFeedback = recordFeedbackItem(
        humanActor(author),
        { workspaceId: workspaceA, branchId: branch },
        feedbackItemId(`feedback-${randomUUID()}`),
        TS,
        'human review',
      );
      await repo.submitFeedbackItem(humanFeedback);

      const delegatedSignal = recordVerificationSignal(
        delegatedActor(stakeholderId('agent-3')),
        { workspaceId: workspaceA, branchId: branch },
        verificationSignalId(`signal-${randomUUID()}`),
        'passing',
        TS,
        'ci passed',
      );
      await repo.submitVerificationSignal(delegatedSignal);

      const items = await repo.listFeedbackItemsForBranch(workspaceA, branch);
      const signals = await repo.listVerificationSignalsForBranch(workspaceA, branch);
      await pool.end();

      expect(items[0]?.authoredByStakeholderId).toBe(author);
      expect(items[0]?.authoredByActorKind).toBe('human');
      expect(signals[0]?.reportedByStakeholderId).toBe('agent-3');
      expect(signals[0]?.reportedByActorKind).toBe('delegated');
    });

    it('submitting feedback for an unregistered branch throws BranchLifecycleError not-found', async () => {
      const pool = openPool();
      const repo = new NotificationRepository(pool);
      const missingBranch = branchId(`missing-${randomUUID()}`);

      const feedback = recordFeedbackItem(
        humanActor(author),
        { workspaceId: workspaceA, branchId: missingBranch },
        feedbackItemId(`feedback-${randomUUID()}`),
        TS,
        'content',
      );

      await expect(repo.submitFeedbackItem(feedback)).rejects.toMatchObject({
        code: 'not-found',
      } satisfies Partial<BranchLifecycleError>);
      await pool.end();
    });

    it('submitting feedback for a branch with no recorded author throws NotificationError not-found', async () => {
      const pool = openPool();
      const conflicts = new ConflictDetectionRepository(pool);
      const authorlessBranch = branchId(`authorless-${randomUUID()}`);
      await conflicts.registerBranch(workspaceA, authorlessBranch, 'engineering');
      const repo = new NotificationRepository(pool);

      const feedback = recordFeedbackItem(
        humanActor(author),
        { workspaceId: workspaceA, branchId: authorlessBranch },
        feedbackItemId(`feedback-${randomUUID()}`),
        TS,
        'content',
      );

      await expect(repo.submitFeedbackItem(feedback)).rejects.toMatchObject({
        code: 'not-found',
      } satisfies Partial<NotificationError>);
      await pool.end();
    });

    it('tenant isolation: a workspace cannot read another workspace’s feedback, signals, or notifications', async () => {
      const pool = openPool();
      const branchInA = await registerAuthoredBranch(pool, workspaceA);
      const repo = new NotificationRepository(pool);

      const feedback = recordFeedbackItem(
        humanActor(author),
        { workspaceId: workspaceA, branchId: branchInA },
        feedbackItemId(`feedback-${randomUUID()}`),
        TS,
        'workspace A only',
      );
      await repo.submitFeedbackItem(feedback);

      const signal = recordVerificationSignal(
        delegatedActor(stakeholderId('agent-tenant-isolation')),
        { workspaceId: workspaceA, branchId: branchInA },
        verificationSignalId(`signal-${randomUUID()}`),
        'failing',
        TS,
        'workspace A only signal',
      );
      await repo.submitVerificationSignal(signal);

      const itemsFromWorkspaceB = await repo.listFeedbackItemsForBranch(workspaceB, branchInA);
      const signalsFromWorkspaceB = await repo.listVerificationSignalsForBranch(
        workspaceB,
        branchInA,
      );
      const notificationsFromWorkspaceB = await repo.listNotificationsForStakeholder(workspaceB, author);
      await pool.end();

      expect(itemsFromWorkspaceB).toHaveLength(0);
      expect(signalsFromWorkspaceB).toHaveLength(0);
      expect(notificationsFromWorkspaceB).toHaveLength(0);
    });

    it('resubmitting the same feedback item id throws NotificationError invalid-state-transition and rolls back the whole submission', async () => {
      const pool = openPool();
      const branch = await registerAuthoredBranch(pool);
      const repo = new NotificationRepository(pool);
      const duplicateId = feedbackItemId(`feedback-${randomUUID()}`);

      const first = recordFeedbackItem(
        humanActor(author),
        { workspaceId: workspaceA, branchId: branch },
        duplicateId,
        TS,
        'first submission',
      );
      const { notifications: firstNotifications } = await repo.submitFeedbackItem(first);
      const notificationCountBefore = (await repo.listNotificationsForStakeholder(workspaceA, author)).length;

      const second = recordFeedbackItem(
        humanActor(author),
        { workspaceId: workspaceA, branchId: branch },
        duplicateId,
        TS_LATER,
        'second submission with a duplicate id',
      );

      await expect(repo.submitFeedbackItem(second)).rejects.toMatchObject({
        code: 'invalid-state-transition',
      } satisfies Partial<NotificationError>);

      // The failed second submission must not have left behind a stray
      // notification row for its own (never-committed) feedback item — the
      // whole transaction, including any notification inserts already
      // attempted before the duplicate-key failure, must roll back together.
      const items = await repo.listFeedbackItemsForBranch(workspaceA, branch);
      const notificationCountAfter = (await repo.listNotificationsForStakeholder(workspaceA, author)).length;
      await pool.end();

      expect(items).toHaveLength(1);
      expect(items[0]?.content).toBe('first submission');
      expect(firstNotifications).toHaveLength(1);
      expect(notificationCountAfter).toBe(notificationCountBefore);
    });
  },
);
