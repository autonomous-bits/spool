import type { FieldDef, Pool, QueryResult, QueryResultRow } from 'pg';
import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { PG_POOL } from './pg-pool.token.js';
import { PairingCodeRepository } from './pairing-code.repository.js';

function buildQueryResult<TRow extends QueryResultRow>(rows: TRow[]): QueryResult<TRow> {
  return {
    command: 'SELECT',
    rowCount: rows.length,
    oid: 0,
    rows,
    fields: [] as FieldDef[],
  };
}

describe('PairingCodeRepository', () => {
  let repository: PairingCodeRepository;
  let pool: Pick<Pool, 'query'>;

  beforeEach(async () => {
    pool = {
      query: vi.fn<Pool['query']>(),
    } satisfies Pick<Pool, 'query'>;

    const module: TestingModule = await Test.createTestingModule({
      providers: [
        PairingCodeRepository,
        {
          provide: PG_POOL,
          useValue: pool,
        },
      ],
    }).compile();

    repository = module.get(PairingCodeRepository);
  });

  it('create inserts a pairing code row and returns its id', async () => {
    vi.mocked(pool.query).mockResolvedValue(
      buildQueryResult([
        {
          id: 'pairing-code-1',
        },
      ]),
    );

    await expect(
      repository.create({
        codeHash: 'hash-1',
        sessionToken: 'session-token-1',
        refreshToken: 'refresh-token-1',
        expiresAt: new Date('2026-07-11T23:59:00.000Z'),
      }),
    ).resolves.toEqual({ id: 'pairing-code-1' });

    expect(pool.query).toHaveBeenCalledWith(
      expect.stringContaining('INSERT INTO pairing_codes'),
      [
        'hash-1',
        'session-token-1',
        'refresh-token-1',
        new Date('2026-07-11T23:59:00.000Z'),
      ],
    );
  });

  it('consume atomically marks a matching pairing code consumed and returns the stored tokens', async () => {
    vi.mocked(pool.query).mockResolvedValue(
      buildQueryResult([
        {
          session_token: 'session-token-1',
          refresh_token: 'refresh-token-1',
        },
      ]),
    );

    await expect(repository.consume('hash-1')).resolves.toEqual({
      sessionToken: 'session-token-1',
      refreshToken: 'refresh-token-1',
    });

    expect(pool.query).toHaveBeenCalledWith(
      expect.stringContaining('UPDATE pairing_codes'),
      ['hash-1'],
    );
  });

  it('consume returns undefined when the code is missing, expired, or already consumed', async () => {
    vi.mocked(pool.query).mockResolvedValue(buildQueryResult([]));

    await expect(repository.consume('missing-hash')).resolves.toBeUndefined();
  });
});
