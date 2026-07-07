import { randomUUID } from 'node:crypto';
import type { Pool } from 'pg';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { Workspace } from '../../src/domain/workspace.js';
import { WorkspaceMembershipAlreadyExistsError } from '../../src/domain/workspace-membership.js';
import { WorkspaceRepository } from '../../src/persistence/workspace.repository.js';
import { BOOTSTRAP_STAKEHOLDER_ID } from '../../src/persistence/bootstrap-stakeholder.js';
import { setUpTestDatabase, type TestDatabase } from '../support/test-database.js';

function buildWorkspace(overrides: Partial<ConstructorParameters<typeof Workspace>[0]> = {}): Workspace {
  return new Workspace({
    name: `workspace-${Math.random().toString(36).slice(2, 10)}`,
    createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    ...overrides,
  });
}

describe('WorkspaceRepository (containerized Postgres)', () => {
  let database: TestDatabase;
  let pool: Pool;
  let workspaceRepository: WorkspaceRepository;
  let secondStakeholderId: string;

  beforeAll(async () => {
    database = await setUpTestDatabase();
    pool = database.pool;
    workspaceRepository = new WorkspaceRepository(pool);

    secondStakeholderId = randomUUID();
    await pool.query(
      `INSERT INTO stakeholders (id, name, email, role, discipline, github_login)
       VALUES ($1, 'Second Stakeholder', $2, 'stakeholder', 'engineering', $3)`,
      [secondStakeholderId, `second-${secondStakeholderId}@spool.local`, `second-${secondStakeholderId}`],
    );
  });

  afterAll(async () => {
    await database.close();
  });

  it('createWithFirstMember persists the workspace and a single creator membership row', async () => {
    const workspace = buildWorkspace();

    const created = await workspaceRepository.createWithFirstMember(workspace);

    expect(created.id).toBe(workspace.id);
    expect(created.name).toBe(workspace.name);
    expect(created.createdByStakeholderId).toBe(BOOTSTRAP_STAKEHOLDER_ID);

    const workspaceRow = await pool.query('SELECT id, name FROM workspaces WHERE id = $1', [
      created.id,
    ]);
    expect(workspaceRow.rows).toHaveLength(1);

    const membershipRows = await pool.query(
      'SELECT stakeholder_id FROM workspace_memberships WHERE workspace_id = $1',
      [created.id],
    );
    expect(membershipRows.rows).toHaveLength(1);
    expect(membershipRows.rows[0]?.stakeholder_id).toBe(BOOTSTRAP_STAKEHOLDER_ID);
  });

  it('rejects an unknown createdByStakeholderId via the foreign key constraint', async () => {
    const workspace = buildWorkspace({
      createdByStakeholderId: '00000000-0000-0000-0000-00000000dead',
    });

    await expect(workspaceRepository.createWithFirstMember(workspace)).rejects.toThrow();
  });

  it('findById round-trips a persisted workspace exactly', async () => {
    const workspace = buildWorkspace();
    const created = await workspaceRepository.createWithFirstMember(workspace);

    const found = await workspaceRepository.findById(created.id);

    expect(found).toEqual(created);
  });

  it('findById returns undefined for an unknown id', async () => {
    const found = await workspaceRepository.findById('00000000-0000-0000-0000-00000000dead');

    expect(found).toBeUndefined();
  });

  it('isMember reflects the creator as a member and a stranger as not a member', async () => {
    const workspace = await workspaceRepository.createWithFirstMember(buildWorkspace());

    await expect(workspaceRepository.isMember(workspace.id, BOOTSTRAP_STAKEHOLDER_ID)).resolves.toBe(
      true,
    );
    await expect(workspaceRepository.isMember(workspace.id, secondStakeholderId)).resolves.toBe(
      false,
    );
  });

  it('addMember adds a new member when the caller is an existing member', async () => {
    const workspace = await workspaceRepository.createWithFirstMember(buildWorkspace());

    const result = await workspaceRepository.addMember(
      workspace.id,
      BOOTSTRAP_STAKEHOLDER_ID,
      secondStakeholderId,
    );

    expect(result.kind).toBe('added');
    if (result.kind !== 'added') {
      throw new Error('expected added result');
    }
    expect(result.membership.workspaceId).toBe(workspace.id);
    expect(result.membership.stakeholderId).toBe(secondStakeholderId);

    await expect(workspaceRepository.isMember(workspace.id, secondStakeholderId)).resolves.toBe(
      true,
    );
  });

  it('addMember returns workspace_not_found for an unknown workspace id', async () => {
    const result = await workspaceRepository.addMember(
      '00000000-0000-0000-0000-00000000dead',
      BOOTSTRAP_STAKEHOLDER_ID,
      secondStakeholderId,
    );

    expect(result.kind).toBe('workspace_not_found');
  });

  it('addMember returns caller_not_member when the acting stakeholder is not a member', async () => {
    const workspace = await workspaceRepository.createWithFirstMember(buildWorkspace());

    const result = await workspaceRepository.addMember(
      workspace.id,
      secondStakeholderId,
      BOOTSTRAP_STAKEHOLDER_ID,
    );

    expect(result.kind).toBe('caller_not_member');
    await expect(
      workspaceRepository.isMember(workspace.id, BOOTSTRAP_STAKEHOLDER_ID),
    ).resolves.toBe(true);
  });

  it('addMember throws WorkspaceMembershipAlreadyExistsError when the target is already a member', async () => {
    const workspace = await workspaceRepository.createWithFirstMember(buildWorkspace());

    await expect(
      workspaceRepository.addMember(workspace.id, BOOTSTRAP_STAKEHOLDER_ID, BOOTSTRAP_STAKEHOLDER_ID),
    ).rejects.toThrow(WorkspaceMembershipAlreadyExistsError);
  });
});
