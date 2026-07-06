import { randomUUID } from 'node:crypto';
import type { Pool } from 'pg';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { Branch } from '../../src/domain/branch.js';
import { BOOTSTRAP_STAKEHOLDER_ID } from '../../src/persistence/bootstrap-stakeholder.js';
import { BranchRepository } from '../../src/persistence/branch.repository.js';
import { FeedbackNotificationRepository } from '../../src/persistence/feedback-notification.repository.js';
import { StakeholderRepository, type StakeholderRecord } from '../../src/persistence/stakeholder.repository.js';
import { VerificationSignalRepository } from '../../src/persistence/verification-signal.repository.js';
import { setUpTestDatabase, type TestDatabase } from '../support/test-database.js';

async function seedStakeholder(pool: Pool): Promise<StakeholderRecord> {
  const id = randomUUID();
  const suffix = Math.random().toString(36).slice(2, 10);

  await pool.query(
    `INSERT INTO stakeholders (id, name, email, role, discipline, github_login)
     VALUES ($1, $2, $3, 'stakeholder', 'engineering', $4)`,
    [id, `Notification Stakeholder ${suffix}`, `notification-${suffix}@spool.local`, `notification-${suffix}`],
  );

  return { id, discipline: 'engineering' };
}

function buildBranch(): Branch {
  return new Branch({
    name: `notification-branch-${Math.random().toString(36).slice(2, 10)}`,
    discipline: 'engineering',
    createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
  });
}

describe('FeedbackNotificationRepository (containerized Postgres)', () => {
  let database: TestDatabase;
  let pool: Pool;
  let branchRepository: BranchRepository;
  let stakeholderRepository: StakeholderRepository;
  let verificationSignalRepository: VerificationSignalRepository;
  let feedbackNotificationRepository: FeedbackNotificationRepository;

  beforeAll(async () => {
    database = await setUpTestDatabase();
    pool = database.pool;
    branchRepository = new BranchRepository(pool);
    stakeholderRepository = new StakeholderRepository(pool);
    verificationSignalRepository = new VerificationSignalRepository(pool, stakeholderRepository);
    feedbackNotificationRepository = new FeedbackNotificationRepository(pool);
  });

  afterAll(async () => {
    await database.close();
  });

  async function createSubmittedBranch(): Promise<Branch> {
    const created = await branchRepository.create(buildBranch());
    const submitted = await branchRepository.submit(created.id);
    if (submitted === undefined) {
      throw new Error('expected submitted branch');
    }
    return submitted;
  }

  async function createSignalNotifiedFor(stakeholderId: string): Promise<{
    branch: Branch;
    signalId: string;
    notificationId: string;
  }> {
    const branch = await createSubmittedBranch();
    const result = await verificationSignalRepository.create({
      branchId: branch.id,
      verifierName: 'ci-evaluator',
      status: 'pass',
    });
    if (result.kind !== 'created') {
      throw new Error('expected created result');
    }

    const notificationRow = await pool.query<{ id: string }>(
      'SELECT id FROM feedback_notifications WHERE signal_id = $1 AND stakeholder_id = $2',
      [result.signal.id, stakeholderId],
    );
    const notificationId = notificationRow.rows[0]?.id;
    if (notificationId === undefined) {
      throw new Error('expected a fanned-out notification for the given stakeholder');
    }

    return { branch, signalId: result.signal.id, notificationId };
  }

  it('findByStakeholderId lists only the given stakeholder\'s notifications, newest first', async () => {
    const stakeholder = await seedStakeholder(pool);
    const other = await seedStakeholder(pool);

    const first = await createSignalNotifiedFor(stakeholder.id);
    const second = await createSignalNotifiedFor(stakeholder.id);
    // Fan-out targets every stakeholder that exists at signal time (Meridian IDEA-67), so this
    // third signal -- created to prove `other`'s own notification exists -- also notifies
    // `stakeholder` (seeded before it). Other e2e spec files run concurrently against the same
    // shared containerized Postgres and may create further signals that also notify
    // `stakeholder`, so assertions here only check for presence of the ids this test itself
    // caused, plus strict ownership scoping -- never an exact total count.
    const third = await createSignalNotifiedFor(other.id);
    const thirdNotificationForStakeholder = await pool.query<{ id: string }>(
      'SELECT id FROM feedback_notifications WHERE signal_id = $1 AND stakeholder_id = $2',
      [third.signalId, stakeholder.id],
    );
    const thirdNotificationId = thirdNotificationForStakeholder.rows[0]?.id;
    if (thirdNotificationId === undefined) {
      throw new Error('expected the third signal to also notify the pre-existing stakeholder');
    }

    const notifications = await feedbackNotificationRepository.findByStakeholderId(stakeholder.id);
    const notificationIds = notifications.map((n) => n.id);

    expect(notificationIds).toEqual(
      expect.arrayContaining([first.notificationId, second.notificationId, thirdNotificationId]),
    );
    expect(notifications.every((n) => n.stakeholderId === stakeholder.id)).toBe(true);
  });

  it('findByStakeholderId filters by status', async () => {
    const stakeholder = await seedStakeholder(pool);
    const { notificationId } = await createSignalNotifiedFor(stakeholder.id);

    const readResult = await feedbackNotificationRepository.markAsRead(notificationId, stakeholder.id);
    expect(readResult?.status).toBe('read');

    const unread = await feedbackNotificationRepository.findByStakeholderId(stakeholder.id, 'unread');
    const read = await feedbackNotificationRepository.findByStakeholderId(stakeholder.id, 'read');

    expect(unread.map((n) => n.id)).not.toContain(notificationId);
    expect(read.map((n) => n.id)).toContain(notificationId);
  });

  it('markAsRead sets status=read for the owning stakeholder', async () => {
    const stakeholder = await seedStakeholder(pool);
    const { notificationId } = await createSignalNotifiedFor(stakeholder.id);

    const result = await feedbackNotificationRepository.markAsRead(notificationId, stakeholder.id);

    expect(result?.status).toBe('read');
    expect(result?.id).toBe(notificationId);
  });

  it('markAsRead returns undefined for a notification belonging to another stakeholder', async () => {
    const stakeholder = await seedStakeholder(pool);
    const other = await seedStakeholder(pool);
    const { notificationId } = await createSignalNotifiedFor(stakeholder.id);

    const result = await feedbackNotificationRepository.markAsRead(notificationId, other.id);

    expect(result).toBeUndefined();
  });

  it('markAsRead returns undefined for an unknown notification id', async () => {
    const stakeholder = await seedStakeholder(pool);

    const result = await feedbackNotificationRepository.markAsRead(randomUUID(), stakeholder.id);

    expect(result).toBeUndefined();
  });
});
