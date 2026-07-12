import { randomUUID } from 'node:crypto';
import type { Pool } from 'pg';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { PairingCodeRepository } from '../../src/persistence/pairing-code.repository.js';
import { setUpTestDatabase, type TestDatabase } from '../support/test-database.js';

describe('PairingCodeRepository (containerized Postgres)', () => {
  let database: TestDatabase;
  let pool: Pool;
  let repository: PairingCodeRepository;

  beforeAll(async () => {
    database = await setUpTestDatabase();
    pool = database.pool;
    repository = new PairingCodeRepository(pool);
  });

  afterAll(async () => {
    await database.close();
  });

  it('consume returns undefined for an expired, never-consumed pairing code', async () => {
    const codeHash = `expired-hash-${randomUUID()}`;
    await repository.create({
      codeHash,
      sessionToken: 'session-token-expired',
      refreshToken: 'refresh-token-expired',
      expiresAt: new Date(Date.now() - 1_000),
    });

    await expect(repository.consume(codeHash)).resolves.toBeUndefined();
  });

  it('consume only lets one of two concurrent callers claim the same pairing code', async () => {
    const codeHash = `concurrent-hash-${randomUUID()}`;
    await repository.create({
      codeHash,
      sessionToken: 'session-token-concurrent',
      refreshToken: 'refresh-token-concurrent',
      expiresAt: new Date(Date.now() + 60_000),
    });

    const [first, second] = await Promise.all([
      repository.consume(codeHash),
      repository.consume(codeHash),
    ]);

    const successes = [first, second].filter((result) => result !== undefined);
    const misses = [first, second].filter((result) => result === undefined);

    expect(successes).toHaveLength(1);
    expect(successes[0]).toEqual({
      sessionToken: 'session-token-concurrent',
      refreshToken: 'refresh-token-concurrent',
    });
    expect(misses).toHaveLength(1);

    const rows = await pool.query<{ consumed_at: Date | null }>(
      'SELECT consumed_at FROM pairing_codes WHERE code_hash = $1',
      [codeHash],
    );
    expect(rows.rows).toHaveLength(1);
    expect(rows.rows[0]?.consumed_at).toBeInstanceOf(Date);
  });
});
