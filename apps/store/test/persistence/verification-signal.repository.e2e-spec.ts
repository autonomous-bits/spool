import { randomUUID } from 'node:crypto';
import type { Pool } from 'pg';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { Branch } from '../../src/domain/branch.js';
import { BranchRepository } from '../../src/persistence/branch.repository.js';
import { BOOTSTRAP_STAKEHOLDER_ID } from '../../src/persistence/bootstrap-stakeholder.js';
import {
  StakeholderRepository,
  type StakeholderRecord,
} from '../../src/persistence/stakeholder.repository.js';
import { VerificationSignalRepository } from '../../src/persistence/verification-signal.repository.js';
import { setUpTestDatabase, type TestDatabase } from '../support/test-database.js';

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

function buildBranch(): Branch {
  return new Branch({
    name: `signal-branch-${Math.random().toString(36).slice(2, 10)}`,
    discipline: 'engineering',
    createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
  });
}

class FaultInjectingStakeholderRepository extends StakeholderRepository {
  constructor(
    pool: Pool,
    private readonly rowsToReturn: StakeholderRecord[],
  ) {
    super(pool);
  }

  override async findAll(): Promise<StakeholderRecord[]> {
    return this.rowsToReturn;
  }
}

describe('VerificationSignalRepository (containerized Postgres)', () => {
  let database: TestDatabase;
  let pool: Pool;
  let branchRepository: BranchRepository;
  let stakeholderRepository: StakeholderRepository;
  let verificationSignalRepository: VerificationSignalRepository;

  beforeAll(async () => {
    database = await setUpTestDatabase();
    pool = database.pool;
    branchRepository = new BranchRepository(pool);
    stakeholderRepository = new StakeholderRepository(pool);
    verificationSignalRepository = new VerificationSignalRepository(pool, stakeholderRepository);
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

  it('create fans out one unread notification to every stakeholder that exists at signal time', async () => {
    const additionalStakeholderA = await seedStakeholder(pool);
    const additionalStakeholderB = await seedStakeholder(pool, { discipline: null });
    const branch = await createSubmittedBranch();

    const result = await verificationSignalRepository.create({
      branchId: branch.id,
      verifierName: 'ci-evaluator',
      status: 'pass',
      reason: 'all checks green',
    });

    expect(result.kind).toBe('created');
    if (result.kind !== 'created') {
      throw new Error('expected created result');
    }

    const notifications = await pool.query<{
      branch_id: string;
      stakeholder_id: string;
      signal_id: string;
      status: string;
    }>(
      `SELECT branch_id, stakeholder_id, signal_id, status
         FROM feedback_notifications
        WHERE signal_id = $1
        ORDER BY stakeholder_id ASC`,
      [result.signal.id],
    );

    // Scoped to stakeholders that existed at signal-creation time (not "every row currently in
    // the table"), since e2e spec files share one containerized Postgres and run concurrently --
    // a stakeholder inserted by a different, unrelated in-flight test file after this signal was
    // created must never appear in this signal's fan-out set. Mirrors the time-scoping already
    // used by the "does not retroactively notify" test below.
    const stakeholderIds = (
      await pool.query<{ id: string }>(
        'SELECT id FROM stakeholders WHERE created_at <= $1 ORDER BY id ASC',
        [result.signal.createdAt],
      )
    ).rows.map((row) => row.id);

    expect(notifications.rows).toHaveLength(stakeholderIds.length);
    expect(notifications.rows.map((row) => row.stakeholder_id)).toEqual(stakeholderIds);
    expect(notifications.rows.every((row) => row.branch_id === branch.id)).toBe(true);
    expect(notifications.rows.every((row) => row.signal_id === result.signal.id)).toBe(true);
    expect(notifications.rows.every((row) => row.status === 'unread')).toBe(true);
    expect(stakeholderIds).toContain(additionalStakeholderA.id);
    expect(stakeholderIds).toContain(additionalStakeholderB.id);
  });

  it('create does not retroactively notify stakeholders added after the signal is persisted', async () => {
    const branch = await createSubmittedBranch();

    const result = await verificationSignalRepository.create({
      branchId: branch.id,
      verifierName: 'human-reviewer',
      status: 'fail',
      reason: 'needs rework',
    });

    expect(result.kind).toBe('created');
    if (result.kind !== 'created') {
      throw new Error('expected created result');
    }

    const lateStakeholder = await seedStakeholder(pool);
    const notifications = await pool.query<{ stakeholder_id: string }>(
      'SELECT stakeholder_id FROM feedback_notifications WHERE signal_id = $1 ORDER BY stakeholder_id ASC',
      [result.signal.id],
    );
    const stakeholderCountAtSignalTime = (
      await pool.query<{ id: string }>(
        'SELECT id FROM stakeholders WHERE created_at <= $1',
        [result.signal.createdAt],
      )
    ).rows.length;

    expect(notifications.rows.map((row) => row.stakeholder_id)).not.toContain(lateStakeholder.id);
    expect(notifications.rows).toHaveLength(stakeholderCountAtSignalTime);
  });

  it('rolls back the signal insert when notification fan-out fails mid-transaction', async () => {
    const validStakeholder = await seedStakeholder(pool);
    const branch = await createSubmittedBranch();
    const repositoryWithFault = new VerificationSignalRepository(
      pool,
      new FaultInjectingStakeholderRepository(pool, [
        validStakeholder,
        { id: '00000000-0000-0000-0000-00000000dead', discipline: null },
      ]),
    );

    await expect(
      repositoryWithFault.create({
        branchId: branch.id,
        verifierName: 'fault-injector',
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
});
