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

  it('submit transitions a draft branch once and returns undefined on repeat submit', async () => {
    const created = await repository.create(buildBranch());

    const submitted = await repository.submit(created.id);
    const repeated = await repository.submit(created.id);

    expect(submitted).toBeDefined();
    expect(submitted?.status).toBe('submitted');
    expect(submitted?.submittedAt).toBeInstanceOf(Date);
    expect(repeated).toBeUndefined();
  });

  it('submit persists submitted_at and round-trips it through the branch mapper', async () => {
    const created = await repository.create(buildBranch());

    const submitted = await repository.submit(created.id);
    const found = await repository.findById(created.id);
    const row = await pool.query<{ submitted_at: Date | null; status: string }>(
      'SELECT submitted_at, status FROM branches WHERE id = $1',
      [created.id],
    );

    expect(submitted).toBeDefined();
    expect(found).toBeDefined();
    expect(row.rows).toHaveLength(1);
    expect(row.rows[0]?.status).toBe('submitted');
    expect(row.rows[0]?.submitted_at).toBeInstanceOf(Date);
    expect(submitted?.submittedAt?.toISOString()).toBe(row.rows[0]?.submitted_at?.toISOString());
    expect(found?.submittedAt?.toISOString()).toBe(row.rows[0]?.submitted_at?.toISOString());
  });
});
