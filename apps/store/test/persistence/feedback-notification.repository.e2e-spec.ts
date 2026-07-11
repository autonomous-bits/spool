import { randomUUID } from 'node:crypto';
import type { Pool } from 'pg';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { Branch } from '../../src/domain/branch.js';
import { Workspace } from '../../src/domain/workspace.js';
import { BOOTSTRAP_STAKEHOLDER_ID } from '../../src/persistence/bootstrap-stakeholder.js';
import { BranchRepository } from '../../src/persistence/branch.repository.js';
import { FeedbackNotificationRepository } from '../../src/persistence/feedback-notification.repository.js';
import type { StakeholderRecord } from '../../src/persistence/stakeholder.repository.js';
import { VerificationSignalRepository } from '../../src/persistence/verification-signal.repository.js';
import { WorkspaceRepository } from '../../src/persistence/workspace.repository.js';
import { setUpTestDatabase, type TestDatabase } from '../support/test-database.js';

const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';

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

async function addWorkspaceMember(pool: Pool, workspaceId: string, stakeholderId: string): Promise<void> {
  await pool.query(
    `INSERT INTO workspace_memberships (workspace_id, stakeholder_id)
     VALUES ($1, $2)
     ON CONFLICT DO NOTHING`,
    [workspaceId, stakeholderId],
  );
}

async function seedWorkspaceMemberStakeholder(pool: Pool, workspaceId: string): Promise<StakeholderRecord> {
  const stakeholder = await seedStakeholder(pool);
  await addWorkspaceMember(pool, workspaceId, stakeholder.id);
  return stakeholder;
}

function buildBranch(): Branch {
  return new Branch({
    workspaceId: WORKSPACE_ID,
    name: `notification-branch-${Math.random().toString(36).slice(2, 10)}`,
    discipline: 'engineering',
    createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
  });
}

describe('FeedbackNotificationRepository (containerized Postgres)', () => {
  let database: TestDatabase;
  let pool: Pool;
  let branchRepository: BranchRepository;
  let workspaceRepository: WorkspaceRepository;
  let verificationSignalRepository: VerificationSignalRepository;
  let feedbackNotificationRepository: FeedbackNotificationRepository;

  beforeAll(async () => {
    database = await setUpTestDatabase();
    pool = database.pool;
    branchRepository = new BranchRepository(pool);
    workspaceRepository = new WorkspaceRepository(pool);
    verificationSignalRepository = new VerificationSignalRepository(pool);
    feedbackNotificationRepository = new FeedbackNotificationRepository(pool);
  });

  afterAll(async () => {
    await database.close();
  });

  async function createSubmittedBranch(): Promise<Branch> {
    const created = await branchRepository.create(buildBranch());
    const submitted = await branchRepository.submit(created.id, WORKSPACE_ID);
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
      workspaceId: WORKSPACE_ID,
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
    const stakeholder = await seedWorkspaceMemberStakeholder(pool, WORKSPACE_ID);
    const other = await seedWorkspaceMemberStakeholder(pool, WORKSPACE_ID);

    const first = await createSignalNotifiedFor(stakeholder.id);
    const second = await createSignalNotifiedFor(stakeholder.id);
    // Fan-out targets every member of the branch's workspace at signal time (Meridian
    // IDEA-67/IDEA-98/IDEA-103), so this third signal -- created to prove `other`'s own
    // notification exists -- also notifies `stakeholder` (a member of the same workspace,
    // seeded before it). Other e2e spec files run concurrently against the same shared
    // containerized Postgres and may create further signals that also notify `stakeholder`, so
    // assertions here only check for presence of the ids this test itself caused, plus strict
    // ownership scoping -- never an exact total count.
    const third = await createSignalNotifiedFor(other.id);
    const thirdNotificationForStakeholder = await pool.query<{ id: string }>(
      'SELECT id FROM feedback_notifications WHERE signal_id = $1 AND stakeholder_id = $2',
      [third.signalId, stakeholder.id],
    );
    const thirdNotificationId = thirdNotificationForStakeholder.rows[0]?.id;
    if (thirdNotificationId === undefined) {
      throw new Error('expected the third signal to also notify the pre-existing stakeholder');
    }

    const notifications = await feedbackNotificationRepository.findByStakeholderId(
      stakeholder.id,
      WORKSPACE_ID,
    );
    const notificationIds = notifications.map((n) => n.id);

    expect(notificationIds).toEqual(
      expect.arrayContaining([first.notificationId, second.notificationId, thirdNotificationId]),
    );
    expect(notifications.every((n) => n.stakeholderId === stakeholder.id)).toBe(true);
    expect(notifications.every((n) => n.workspaceId === WORKSPACE_ID)).toBe(true);
  });

  it('findByStakeholderId filters by status', async () => {
    const stakeholder = await seedWorkspaceMemberStakeholder(pool, WORKSPACE_ID);
    const { notificationId } = await createSignalNotifiedFor(stakeholder.id);

    const readResult = await feedbackNotificationRepository.markAsRead(
      notificationId,
      stakeholder.id,
      WORKSPACE_ID,
    );
    expect(readResult?.status).toBe('read');

    const unread = await feedbackNotificationRepository.findByStakeholderId(
      stakeholder.id,
      WORKSPACE_ID,
      'unread',
    );
    const read = await feedbackNotificationRepository.findByStakeholderId(
      stakeholder.id,
      WORKSPACE_ID,
      'read',
    );

    expect(unread.map((n) => n.id)).not.toContain(notificationId);
    expect(read.map((n) => n.id)).toContain(notificationId);
  });

  it('findByStakeholderId returns nothing for a different workspace', async () => {
    const stakeholder = await seedWorkspaceMemberStakeholder(pool, WORKSPACE_ID);
    await createSignalNotifiedFor(stakeholder.id);

    const otherWorkspace = await workspaceRepository.createWithFirstMember(
      new Workspace({
        name: `notification-find-workspace-${Date.now()}`,
        createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      }),
    );

    const result = await feedbackNotificationRepository.findByStakeholderId(
      stakeholder.id,
      otherWorkspace.id,
    );

    expect(result).toEqual([]);
  });

  it('markAsRead sets status=read for the owning stakeholder', async () => {
    const stakeholder = await seedWorkspaceMemberStakeholder(pool, WORKSPACE_ID);
    const { notificationId } = await createSignalNotifiedFor(stakeholder.id);

    const result = await feedbackNotificationRepository.markAsRead(
      notificationId,
      stakeholder.id,
      WORKSPACE_ID,
    );

    expect(result?.status).toBe('read');
    expect(result?.id).toBe(notificationId);
  });

  it('markAsRead returns undefined for a notification belonging to another stakeholder', async () => {
    const stakeholder = await seedWorkspaceMemberStakeholder(pool, WORKSPACE_ID);
    const other = await seedWorkspaceMemberStakeholder(pool, WORKSPACE_ID);
    const { notificationId } = await createSignalNotifiedFor(stakeholder.id);

    const result = await feedbackNotificationRepository.markAsRead(
      notificationId,
      other.id,
      WORKSPACE_ID,
    );

    expect(result).toBeUndefined();
  });

  it('markAsRead returns undefined for a notification in a different workspace', async () => {
    const stakeholder = await seedWorkspaceMemberStakeholder(pool, WORKSPACE_ID);
    const { notificationId } = await createSignalNotifiedFor(stakeholder.id);

    const otherWorkspace = await workspaceRepository.createWithFirstMember(
      new Workspace({
        name: `notification-mark-workspace-${Date.now()}`,
        createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      }),
    );

    const result = await feedbackNotificationRepository.markAsRead(
      notificationId,
      stakeholder.id,
      otherWorkspace.id,
    );

    expect(result).toBeUndefined();
  });

  it('markAsRead returns undefined for an unknown notification id', async () => {
    const stakeholder = await seedWorkspaceMemberStakeholder(pool, WORKSPACE_ID);

    const result = await feedbackNotificationRepository.markAsRead(
      randomUUID(),
      stakeholder.id,
      WORKSPACE_ID,
    );

    expect(result).toBeUndefined();
  });
});
