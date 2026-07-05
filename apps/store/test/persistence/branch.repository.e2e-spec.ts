import type { Pool } from 'pg';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { Branch } from '../../src/domain/branch.js';
import { BranchRepository } from '../../src/persistence/branch.repository.js';
import { BOOTSTRAP_STAKEHOLDER_ID } from '../../src/persistence/bootstrap-stakeholder.js';
import { setUpTestDatabase, type TestDatabase } from '../support/test-database.js';

function buildBranch(overrides: Partial<ConstructorParameters<typeof Branch>[0]> = {}): Branch {
  return new Branch({
    name: `branch-${Math.random().toString(36).slice(2, 10)}`,
    discipline: 'engineering',
    createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    ...overrides,
  });
}

describe('BranchRepository (containerized Postgres)', () => {
  let database: TestDatabase;
  let pool: Pool;
  let repository: BranchRepository;

  beforeAll(async () => {
    database = await setUpTestDatabase();
    pool = database.pool;
    repository = new BranchRepository(pool);
  });

  afterAll(async () => {
    await database?.close();
  });

  it('create persists a branch with status draft', async () => {
    const branch = buildBranch();

    const created = await repository.create(branch);

    expect(created.status).toBe('draft');

    const row = await pool.query<{ status: string; discipline: string }>(
      'SELECT status, discipline FROM branches WHERE id = $1',
      [created.id],
    );

    expect(row.rows).toHaveLength(1);
    expect(row.rows[0]?.status).toBe('draft');
    expect(row.rows[0]?.discipline).toBe('engineering');
  });

  it('findById round-trips every field exactly for a persisted branch', async () => {
    const branch = buildBranch({
      name: `roundtrip-${Math.random().toString(36).slice(2, 10)}`,
      discipline: 'security',
    });

    const created = await repository.create(branch);
    const found = await repository.findById(created.id);

    expect(found).toBeDefined();
    expect(found).toEqual(created);
  });

  it('findById returns an explicit not-found result (undefined) for an unknown id', async () => {
    const found = await repository.findById('00000000-0000-0000-0000-00000000dead');

    expect(found).toBeUndefined();
  });
});
