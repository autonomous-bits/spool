import { randomUUID } from 'node:crypto';
import type { Pool } from 'pg';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { StakeholderRepository } from '../../src/persistence/stakeholder.repository.js';
import { setUpTestDatabase, type TestDatabase } from '../support/test-database.js';

interface SeededStakeholder {
  id: string;
  githubLogin: string;
  discipline: string | null;
}

async function seedStakeholder(
  pool: Pool,
  overrides: Partial<Pick<SeededStakeholder, 'discipline'>> = {},
): Promise<SeededStakeholder> {
  const id = randomUUID();
  const suffix = Math.random().toString(36).slice(2, 10);
  const githubLogin = `octocat-${suffix}`;
  const discipline = 'discipline' in overrides ? (overrides.discipline ?? null) : 'engineering';

  await pool.query(
    `INSERT INTO stakeholders (id, name, email, role, discipline, github_login)
     VALUES ($1, $2, $3, 'stakeholder', $4, $5)`,
    [id, `Test Stakeholder ${suffix}`, `stakeholder-${suffix}@spool.local`, discipline, githubLogin],
  );

  return { id, githubLogin, discipline };
}

describe('StakeholderRepository (containerized Postgres)', () => {
  let database: TestDatabase;
  let pool: Pool;
  let repository: StakeholderRepository;

  beforeAll(async () => {
    database = await setUpTestDatabase();
    pool = database.pool;
    repository = new StakeholderRepository(pool);
  });

  afterAll(async () => {
    await database.close();
  });

  it('findByGithubLogin round-trips id and discipline for a seeded stakeholder', async () => {
    const seeded = await seedStakeholder(pool);

    const found = await repository.findByGithubLogin(seeded.githubLogin);

    expect(found).toEqual({ id: seeded.id, discipline: 'engineering' });
  });

  it('findByGithubLogin round-trips a null-discipline stakeholder', async () => {
    const seeded = await seedStakeholder(pool, { discipline: null });

    const found = await repository.findByGithubLogin(seeded.githubLogin);

    expect(found).toEqual({ id: seeded.id, discipline: null });
  });

  it('findByGithubLogin returns undefined for an unmapped GitHub login', async () => {
    const found = await repository.findByGithubLogin('no-such-github-login');

    expect(found).toBeUndefined();
  });

  it('findById round-trips id and nullable discipline for seeded stakeholders', async () => {
    const seededWithDiscipline = await seedStakeholder(pool);
    const seededWithoutDiscipline = await seedStakeholder(pool, { discipline: null });

    const foundWithDiscipline = await repository.findById(seededWithDiscipline.id);
    const foundWithoutDiscipline = await repository.findById(seededWithoutDiscipline.id);

    expect(foundWithDiscipline).toEqual({
      id: seededWithDiscipline.id,
      discipline: seededWithDiscipline.discipline,
    });
    expect(foundWithoutDiscipline).toEqual({
      id: seededWithoutDiscipline.id,
      discipline: null,
    });
  });

  it('findById returns undefined for an unknown stakeholder id', async () => {
    const found = await repository.findById('00000000-0000-0000-0000-00000000dead');

    expect(found).toBeUndefined();
  });

  it('findAll returns every stakeholder deterministically oldest-first', async () => {
    const first = await seedStakeholder(pool);
    const second = await seedStakeholder(pool, { discipline: null });

    const found = await repository.findAll();

    const firstIndex = found.findIndex((stakeholder) => stakeholder.id === first.id);
    const secondIndex = found.findIndex((stakeholder) => stakeholder.id === second.id);

    expect(firstIndex).toBeGreaterThanOrEqual(0);
    expect(secondIndex).toBeGreaterThan(firstIndex);
    expect(found[firstIndex]).toEqual({ id: first.id, discipline: 'engineering' });
    expect(found[secondIndex]).toEqual({ id: second.id, discipline: null });
  });
});
