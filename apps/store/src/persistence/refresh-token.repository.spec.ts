import type { FieldDef, Pool, QueryResult, QueryResultRow } from 'pg';
import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { PG_POOL } from './pg-pool.token.js';
import { RefreshTokenRepository } from './refresh-token.repository.js';

function buildQueryResult<TRow extends QueryResultRow>(rows: TRow[]): QueryResult<TRow> {
  return {
    command: 'SELECT',
    rowCount: rows.length,
    oid: 0,
    rows,
    fields: [] as FieldDef[],
  };
}

describe('RefreshTokenRepository', () => {
  let repository: RefreshTokenRepository;
  let pool: Pick<Pool, 'query'>;

  beforeEach(async () => {
    pool = {
      query: vi.fn<Pool['query']>(),
    } satisfies Pick<Pool, 'query'>;

    const module: TestingModule = await Test.createTestingModule({
      providers: [
        RefreshTokenRepository,
        {
          provide: PG_POOL,
          useValue: pool,
        },
      ],
    }).compile();

    repository = module.get(RefreshTokenRepository);
  });

  it('create inserts a refresh token row and returns its id', async () => {
    vi.mocked(pool.query).mockResolvedValue(
      buildQueryResult([
        {
          id: 'refresh-token-1',
        },
      ]),
    );

    await expect(
      repository.create({
        stakeholderId: 'stakeholder-1',
        workspaceId: 'workspace-1',
        tokenHash: 'hash-1',
        expiresAt: new Date('2026-08-01T00:00:00.000Z'),
      }),
    ).resolves.toEqual({ id: 'refresh-token-1' });

    expect(pool.query).toHaveBeenCalledWith(
      expect.stringContaining('INSERT INTO refresh_tokens'),
      ['stakeholder-1', 'workspace-1', 'hash-1', new Date('2026-08-01T00:00:00.000Z')],
    );
  });

  it('findActiveByTokenHash returns the active row mapped to camelCase fields', async () => {
    vi.mocked(pool.query).mockResolvedValue(
      buildQueryResult([
        {
          id: 'refresh-token-1',
          stakeholder_id: 'stakeholder-1',
          workspace_id: null,
          token_hash: 'hash-1',
          created_at: new Date('2026-07-01T00:00:00.000Z'),
          expires_at: new Date('2026-08-01T00:00:00.000Z'),
          revoked_at: null,
          replaced_by_id: null,
        },
      ]),
    );

    await expect(repository.findActiveByTokenHash('hash-1')).resolves.toEqual({
      id: 'refresh-token-1',
      stakeholderId: 'stakeholder-1',
      workspaceId: null,
      tokenHash: 'hash-1',
      createdAt: new Date('2026-07-01T00:00:00.000Z'),
      expiresAt: new Date('2026-08-01T00:00:00.000Z'),
      revokedAt: null,
      replacedById: null,
    });
  });

  it('findActiveByTokenHash returns undefined when no active token matches', async () => {
    vi.mocked(pool.query).mockResolvedValue(buildQueryResult([]));

    await expect(repository.findActiveByTokenHash('missing-hash')).resolves.toBeUndefined();
  });

  it('revoke marks the token revoked and records its replacement id when provided', async () => {
    vi.mocked(pool.query).mockResolvedValue(buildQueryResult([]));

    await repository.revoke('refresh-token-1', 'refresh-token-2');

    expect(pool.query).toHaveBeenCalledWith(
      expect.stringContaining('UPDATE refresh_tokens'),
      ['refresh-token-1', 'refresh-token-2'],
    );
  });
});
