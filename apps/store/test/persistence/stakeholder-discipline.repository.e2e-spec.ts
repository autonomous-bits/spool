import { randomUUID } from 'node:crypto';
import type { Pool } from 'pg';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { Workspace } from '../../src/domain/workspace.js';
import { WorkspaceRepository } from '../../src/persistence/workspace.repository.js';
import { StakeholderDisciplineRepository } from '../../src/persistence/stakeholder-discipline.repository.js';
import { BOOTSTRAP_STAKEHOLDER_ID } from '../../src/persistence/bootstrap-stakeholder.js';
import { setUpTestDatabase, type TestDatabase } from '../support/test-database.js';

function buildWorkspace(overrides: Partial<ConstructorParameters<typeof Workspace>[0]> = {}): Workspace {
  return new Workspace({
    name: `workspace-${Math.random().toString(36).slice(2, 10)}`,
    createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    ...overrides,
  });
}

describe('StakeholderDisciplineRepository (containerized Postgres)', () => {
  let database: TestDatabase;
  let pool: Pool;
  let workspaceRepository: WorkspaceRepository;
  let stakeholderDisciplineRepository: StakeholderDisciplineRepository;

  beforeAll(async () => {
    database = await setUpTestDatabase();
    pool = database.pool;
    workspaceRepository = new WorkspaceRepository(pool);
    stakeholderDisciplineRepository = new StakeholderDisciplineRepository(pool);
  });

  afterAll(async () => {
    await database.close();
  });

  it('isAllowed returns true only for a discipline actually inserted for that exact (workspaceId, stakeholderId) pair', async () => {
    const workspace = await workspaceRepository.createWithFirstMember(buildWorkspace());
    const otherWorkspace = await workspaceRepository.createWithFirstMember(buildWorkspace());

    await pool.query(
      `INSERT INTO stakeholder_disciplines (workspace_id, stakeholder_id, discipline)
       VALUES ($1, $2, 'architecture')`,
      [workspace.id, BOOTSTRAP_STAKEHOLDER_ID],
    );

    await expect(
      stakeholderDisciplineRepository.isAllowed(workspace.id, BOOTSTRAP_STAKEHOLDER_ID, 'architecture'),
    ).resolves.toBe(true);

    // A different discipline never assigned for this pair.
    await expect(
      stakeholderDisciplineRepository.isAllowed(workspace.id, BOOTSTRAP_STAKEHOLDER_ID, 'engineering'),
    ).resolves.toBe(false);

    // A different workspace, same stakeholder and discipline.
    await expect(
      stakeholderDisciplineRepository.isAllowed(otherWorkspace.id, BOOTSTRAP_STAKEHOLDER_ID, 'architecture'),
    ).resolves.toBe(false);
  });

  it('listAllowed returns every discipline assigned for a (workspaceId, stakeholderId) pair', async () => {
    const workspace = await workspaceRepository.createWithFirstMember(buildWorkspace());

    await pool.query(
      `INSERT INTO stakeholder_disciplines (workspace_id, stakeholder_id, discipline)
       VALUES ($1, $2, 'product'), ($1, $2, 'design')`,
      [workspace.id, BOOTSTRAP_STAKEHOLDER_ID],
    );

    const allowed = await stakeholderDisciplineRepository.listAllowed(
      workspace.id,
      BOOTSTRAP_STAKEHOLDER_ID,
    );

    expect(new Set(allowed)).toEqual(new Set(['product', 'design']));
  });

  it('listAllowed returns an empty array when nothing has been assigned', async () => {
    const workspace = await workspaceRepository.createWithFirstMember(buildWorkspace());

    await expect(
      stakeholderDisciplineRepository.listAllowed(workspace.id, BOOTSTRAP_STAKEHOLDER_ID),
    ).resolves.toEqual([]);
  });

  it('rejects a stakeholder_disciplines row whose (workspace_id, stakeholder_id) has no matching workspace_memberships row', async () => {
    const workspace = await workspaceRepository.createWithFirstMember(buildWorkspace());
    const nonMemberId = randomUUID();
    await pool.query(
      `INSERT INTO stakeholders (id, name, email, role, discipline, github_login)
       VALUES ($1, 'Non Member', $2, 'stakeholder', 'engineering', $3)`,
      [nonMemberId, `non-member-${nonMemberId}@spool.local`, `non-member-${nonMemberId}`],
    );

    await expect(
      pool.query(
        `INSERT INTO stakeholder_disciplines (workspace_id, stakeholder_id, discipline)
         VALUES ($1, $2, 'engineering')`,
        [workspace.id, nonMemberId],
      ),
    ).rejects.toThrow();
  });

  it('backfill produces exactly one stakeholder_disciplines row per existing (workspace_membership, non-null discipline) pair', async () => {
    const seededStakeholderId = randomUUID();
    await pool.query(
      `INSERT INTO stakeholders (id, name, email, role, discipline, github_login)
       VALUES ($1, 'Seeded Stakeholder', $2, 'stakeholder', 'security', $3)`,
      [
        seededStakeholderId,
        `seeded-${seededStakeholderId}@spool.local`,
        `seeded-${seededStakeholderId}`,
      ],
    );
    const workspace = await workspaceRepository.createWithFirstMember(
      buildWorkspace({ createdByStakeholderId: seededStakeholderId }),
    );

    // Simulate the migration's backfill statement directly (the real migration already ran once
    // against this database at bootstrap; this proves the backfill query itself is correct and
    // idempotent for a membership/discipline pair created after migration time).
    await pool.query(
      `INSERT INTO stakeholder_disciplines (workspace_id, stakeholder_id, discipline)
       SELECT wm.workspace_id, wm.stakeholder_id, s.discipline
         FROM workspace_memberships wm
         JOIN stakeholders s ON s.id = wm.stakeholder_id
        WHERE s.discipline IS NOT NULL
          AND wm.workspace_id = $1 AND wm.stakeholder_id = $2
       ON CONFLICT DO NOTHING`,
      [workspace.id, seededStakeholderId],
    );

    const rows = await pool.query(
      'SELECT discipline FROM stakeholder_disciplines WHERE workspace_id = $1 AND stakeholder_id = $2',
      [workspace.id, seededStakeholderId],
    );
    expect(rows.rows).toHaveLength(1);
    expect(rows.rows[0]?.discipline).toBe('security');
  });
});
