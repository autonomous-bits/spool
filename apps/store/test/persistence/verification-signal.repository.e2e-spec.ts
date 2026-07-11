import { randomUUID } from 'node:crypto';
import type { Pool } from 'pg';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { Branch } from '../../src/domain/branch.js';
import { Workspace } from '../../src/domain/workspace.js';
import { BranchRepository } from '../../src/persistence/branch.repository.js';
import { BOOTSTRAP_STAKEHOLDER_ID } from '../../src/persistence/bootstrap-stakeholder.js';
import type { StakeholderRecord } from '../../src/persistence/stakeholder.repository.js';
import { VerificationSignalRepository } from '../../src/persistence/verification-signal.repository.js';
import { WorkspaceRepository } from '../../src/persistence/workspace.repository.js';
import { setUpTestDatabase, type TestDatabase } from '../support/test-database.js';

const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';

async function seedStakeholder(
  pool: Pool,
  overrides: Partial<Pick<StakeholderRecord, 'discipline'>> = {},
): Promise<StakeholderRecord> {
  const id = randomUUID();
  const suffix = Math.random().toString(36).slice(2, 10);
  const discipline = 'discipline' in overrides ? (overrides.discipline ?? null) : 'engineering';

  await pool.query(
    `INSERT INTO stakeholders (id, name, email, role, discipline, github_login)
     VALUES ($1, $2, $3, 'stakeholder', $4, $5)`,
    [id, `Signal Stakeholder ${suffix}`, `signal-${suffix}@spool.local`, discipline, `signal-${suffix}`],
  );

  return { id, discipline };
}

async function addWorkspaceMember(pool: Pool, workspaceId: string, stakeholderId: string): Promise<void> {
  await pool.query(
    `INSERT INTO workspace_memberships (workspace_id, stakeholder_id)
     VALUES ($1, $2)
     ON CONFLICT DO NOTHING`,
    [workspaceId, stakeholderId],
  );
}

function buildBranch(workspaceId: string = WORKSPACE_ID): Branch {
  return new Branch({
    workspaceId,
    name: `signal-branch-${Math.random().toString(36).slice(2, 10)}`,
    discipline: 'engineering',
    createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
  });
}

describe('VerificationSignalRepository (containerized Postgres)', () => {
  let database: TestDatabase;
  let pool: Pool;
  let branchRepository: BranchRepository;
  let workspaceRepository: WorkspaceRepository;
  let verificationSignalRepository: VerificationSignalRepository;

  beforeAll(async () => {
    database = await setUpTestDatabase();
    pool = database.pool;
    branchRepository = new BranchRepository(pool);
    workspaceRepository = new WorkspaceRepository(pool);
    verificationSignalRepository = new VerificationSignalRepository(pool);
  });

  afterAll(async () => {
    await database.close();
  });

  async function createSubmittedBranch(workspaceId: string = WORKSPACE_ID): Promise<Branch> {
    const created = await branchRepository.create(buildBranch(workspaceId));
    const submitted = await branchRepository.submit(created.id, workspaceId);
    if (submitted === undefined) {
      throw new Error('expected submitted branch');
    }

    return submitted;
  }

  it('create fans out one unread notification to every member of the branch workspace at signal time', async () => {
    const additionalStakeholderA = await seedStakeholder(pool);
    const additionalStakeholderB = await seedStakeholder(pool, { discipline: null });
    await addWorkspaceMember(pool, WORKSPACE_ID, additionalStakeholderA.id);
    await addWorkspaceMember(pool, WORKSPACE_ID, additionalStakeholderB.id);
    const branch = await createSubmittedBranch();

    const result = await verificationSignalRepository.create({
      branchId: branch.id,
      workspaceId: WORKSPACE_ID,
      verifierName: 'ci-evaluator',
      status: 'pass',
      reason: 'all checks green',
    });

    expect(result.kind).toBe('created');
    if (result.kind !== 'created') {
      throw new Error('expected created result');
    }
    expect(result.signal.workspaceId).toBe(WORKSPACE_ID);

    const notifications = await pool.query<{
      branch_id: string;
      workspace_id: string;
      stakeholder_id: string;
      signal_id: string;
      status: string;
    }>(
      `SELECT branch_id, workspace_id, stakeholder_id, signal_id, status
         FROM feedback_notifications
        WHERE signal_id = $1
        ORDER BY stakeholder_id ASC`,
      [result.signal.id],
    );

    const memberIds = (
      await pool.query<{ stakeholder_id: string }>(
        'SELECT stakeholder_id FROM workspace_memberships WHERE workspace_id = $1 ORDER BY stakeholder_id ASC',
        [WORKSPACE_ID],
      )
    ).rows.map((row) => row.stakeholder_id);

    expect(notifications.rows).toHaveLength(memberIds.length);
    expect(notifications.rows.map((row) => row.stakeholder_id)).toEqual(memberIds);
    expect(notifications.rows.every((row) => row.branch_id === branch.id)).toBe(true);
    expect(notifications.rows.every((row) => row.workspace_id === WORKSPACE_ID)).toBe(true);
    expect(notifications.rows.every((row) => row.signal_id === result.signal.id)).toBe(true);
    expect(notifications.rows.every((row) => row.status === 'unread')).toBe(true);
    expect(memberIds).toContain(additionalStakeholderA.id);
    expect(memberIds).toContain(additionalStakeholderB.id);
  });

  it('create does not notify stakeholders that belong to a different workspace', async () => {
    const outsideWorkspace = await workspaceRepository.createWithFirstMember(
      new Workspace({
        name: `signal-outside-workspace-${Date.now()}`,
        createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      }),
    );
    const outsideStakeholder = await seedStakeholder(pool);
    await addWorkspaceMember(pool, outsideWorkspace.id, outsideStakeholder.id);
    const branch = await createSubmittedBranch();

    const result = await verificationSignalRepository.create({
      branchId: branch.id,
      workspaceId: WORKSPACE_ID,
      verifierName: 'human-reviewer',
      status: 'fail',
      reason: 'needs rework',
    });

    expect(result.kind).toBe('created');
    if (result.kind !== 'created') {
      throw new Error('expected created result');
    }

    const notifications = await pool.query<{ stakeholder_id: string }>(
      'SELECT stakeholder_id FROM feedback_notifications WHERE signal_id = $1',
      [result.signal.id],
    );

    expect(notifications.rows.map((row) => row.stakeholder_id)).not.toContain(outsideStakeholder.id);
  });

  it('returns not_found and inserts nothing when the workspace does not match the branch', async () => {
    const otherWorkspace = await workspaceRepository.createWithFirstMember(
      new Workspace({
        name: `signal-mismatch-workspace-${Date.now()}`,
        createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      }),
    );
    const branch = await createSubmittedBranch(WORKSPACE_ID);

    const result = await verificationSignalRepository.create({
      branchId: branch.id,
      workspaceId: otherWorkspace.id,
      verifierName: 'ci-evaluator',
      status: 'pass',
    });

    expect(result.kind).toBe('not_found');

    const signalRows = await pool.query('SELECT id FROM verification_signals WHERE branch_id = $1', [
      branch.id,
    ]);
    expect(signalRows.rows).toHaveLength(0);
  });

  it('returns not_found for an unknown branch id', async () => {
    const result = await verificationSignalRepository.create({
      branchId: '00000000-0000-0000-0000-00000000dead',
      workspaceId: WORKSPACE_ID,
      verifierName: 'ci-evaluator',
      status: 'pass',
    });

    expect(result.kind).toBe('not_found');
  });

  it('rolls back the entire transaction when the insert violates a database constraint', async () => {
    const branch = await createSubmittedBranch();
    const tooLongVerifierName = 'x'.repeat(300);

    await expect(
      verificationSignalRepository.create({
        branchId: branch.id,
        workspaceId: WORKSPACE_ID,
        verifierName: tooLongVerifierName,
        status: 'pass',
      }),
    ).rejects.toThrow();

    const signalRows = await pool.query('SELECT id FROM verification_signals WHERE branch_id = $1', [
      branch.id,
    ]);
    const notificationRows = await pool.query(
      'SELECT id FROM feedback_notifications WHERE branch_id = $1',
      [branch.id],
    );

    expect(signalRows.rows).toHaveLength(0);
    expect(notificationRows.rows).toHaveLength(0);
  });

  it('findByBranchId is scoped by workspaceId', async () => {
    const branch = await createSubmittedBranch();
    const created = await verificationSignalRepository.create({
      branchId: branch.id,
      workspaceId: WORKSPACE_ID,
      verifierName: 'ci-evaluator',
      status: 'pass',
    });
    if (created.kind !== 'created') {
      throw new Error('expected created result');
    }

    const foundInWorkspace = await verificationSignalRepository.findByBranchId(branch.id, WORKSPACE_ID);
    expect(foundInWorkspace.map((signal) => signal.id)).toContain(created.signal.id);

    const otherWorkspace = await workspaceRepository.createWithFirstMember(
      new Workspace({
        name: `signal-find-workspace-${Date.now()}`,
        createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      }),
    );
    const foundInOtherWorkspace = await verificationSignalRepository.findByBranchId(
      branch.id,
      otherWorkspace.id,
    );
    expect(foundInOtherWorkspace).toEqual([]);
  });
});
