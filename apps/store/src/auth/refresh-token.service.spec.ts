import type { FieldDef, Pool, PoolClient, QueryResult, QueryResultRow } from 'pg';
import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { AuthConfig } from './auth-config.js';
import { AUTH_CONFIG } from './auth-config.token.js';
import {
  InvalidRefreshTokenError,
  RefreshTokenService,
} from './refresh-token.service.js';
import type { RefreshTokenRepository } from '../persistence/refresh-token.repository.js';
import { RefreshTokenRepository as RefreshTokenRepositoryToken } from '../persistence/refresh-token.repository.js';
import { PG_POOL } from '../persistence/pg-pool.token.js';

function buildConfig(overrides: Partial<AuthConfig> = {}): AuthConfig {
  return {
    githubClientId: 'client-id',
    githubClientSecret: 'client-secret',
    githubRedirectUri: 'http://localhost:3000/auth/github/callback',
    githubAuthorizeUrl: 'https://github.com/login/oauth/authorize',
    githubTokenExchangeUrl: 'https://github.com/login/oauth/access_token',
    githubUserApiUrl: 'https://api.github.com/user',
    sessionTokenSecret: 'session-secret',
    sessionTokenMaxAgeSeconds: 900,
    refreshTokenMaxAgeSeconds: 2_592_000,
    oauthStateSecret: 'state-secret',
    oauthStateMaxAgeSeconds: 600,
    ...overrides,
  } satisfies AuthConfig;
}

function buildQueryResult<TRow extends QueryResultRow>(rows: TRow[]): QueryResult<TRow> {
  return {
    command: 'SELECT',
    rowCount: rows.length,
    oid: 0,
    rows,
    fields: [] as FieldDef[],
  };
}

describe('RefreshTokenService', () => {
  let service: RefreshTokenService;
  let repository: Pick<RefreshTokenRepository, 'create' | 'revoke'>;
  let pool: Pick<Pool, 'connect'>;
  let client: PoolClient;
  let clientQuery: ReturnType<typeof vi.fn<PoolClient['query']>>;

  beforeEach(async () => {
    clientQuery = vi.fn<PoolClient['query']>();
    client = {
      query: clientQuery,
      release: vi.fn<PoolClient['release']>(),
    } as PoolClient;
    pool = {
      connect: vi.fn<Pool['connect']>().mockResolvedValue(client),
    } satisfies Pick<Pool, 'connect'>;
    repository = {
      create: vi.fn<RefreshTokenRepository['create']>(),
      revoke: vi.fn<RefreshTokenRepository['revoke']>(),
    } satisfies Pick<RefreshTokenRepository, 'create' | 'revoke'>;

    const module: TestingModule = await Test.createTestingModule({
      providers: [
        RefreshTokenService,
        { provide: AUTH_CONFIG, useValue: buildConfig() },
        { provide: PG_POOL, useValue: pool },
        { provide: RefreshTokenRepositoryToken, useValue: repository },
      ],
    }).compile();

    service = module.get(RefreshTokenService);
  });

  it('issue stores only the token hash and returns the raw token with its expiry', async () => {
    vi.useFakeTimers();
    try {
      const now = new Date('2026-07-11T22:30:00.000Z');
      const expectedExpiresAt = Math.floor(now.getTime() / 1000) + 2_592_000;
      vi.setSystemTime(now);
      vi.mocked(repository.create).mockResolvedValue({ id: 'refresh-token-1' });

      const issued = await service.issue({
        stakeholderId: 'stakeholder-1',
        workspaceId: 'workspace-1',
      });

      expect(issued.token).toHaveLength(43);
      expect(issued.expiresAt).toBe(expectedExpiresAt);
      expect(repository.create).toHaveBeenCalledWith(
        expect.objectContaining({
          stakeholderId: 'stakeholder-1',
          workspaceId: 'workspace-1',
          expiresAt: new Date(expectedExpiresAt * 1000),
        }),
        undefined,
      );

      const createCall = vi.mocked(repository.create).mock.calls[0];
      const createInput = createCall?.[0];
      if (createInput === undefined) {
        throw new Error('expected repository.create to be called');
      }

      expect(createInput.tokenHash).toBe(service.hashToken(issued.token));
      expect(createInput.tokenHash).not.toBe(issued.token);
    } finally {
      vi.useRealTimers();
    }
  });

  it('verifyAndRotate rotates a valid refresh token and revokes the old row', async () => {
    vi.useFakeTimers();
    try {
      const now = new Date('2026-07-11T22:30:00.000Z');
      const expectedExpiresAt = Math.floor(now.getTime() / 1000) + 2_592_000;
      vi.setSystemTime(now);
      vi.mocked(repository.create).mockResolvedValue({ id: 'refresh-token-2' });
      clientQuery
        .mockResolvedValueOnce(buildQueryResult([]))
        .mockResolvedValueOnce(
          buildQueryResult([
            {
              id: 'refresh-token-1',
              stakeholder_id: 'stakeholder-1',
              workspace_id: 'workspace-1',
              token_hash: service.hashToken('old-refresh-token'),
              created_at: new Date('2026-07-01T00:00:00.000Z'),
              expires_at: new Date('2026-08-01T00:00:00.000Z'),
              revoked_at: null,
              replaced_by_id: null,
            },
          ]),
        )
        .mockResolvedValueOnce(buildQueryResult([]));

      const rotated = await service.verifyAndRotate({ token: 'old-refresh-token' });

      expect(rotated).toEqual({
        stakeholderId: 'stakeholder-1',
        workspaceId: 'workspace-1',
        newToken: expect.any(String),
        newExpiresAt: expectedExpiresAt,
      });
      expect(repository.create).toHaveBeenCalledWith(
        expect.objectContaining({
          stakeholderId: 'stakeholder-1',
          workspaceId: 'workspace-1',
          tokenHash: service.hashToken(rotated.newToken),
        }),
        client,
      );
      expect(repository.revoke).toHaveBeenCalledWith('refresh-token-1', 'refresh-token-2', client);
      expect(clientQuery).toHaveBeenNthCalledWith(1, 'BEGIN');
      expect(clientQuery).toHaveBeenNthCalledWith(
        2,
        expect.stringContaining('FROM refresh_tokens'),
        [service.hashToken('old-refresh-token')],
      );
      expect(clientQuery).toHaveBeenNthCalledWith(3, 'COMMIT');
      expect(client.release).toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });

  it('verifyAndRotate rejects an already revoked token', async () => {
    clientQuery
      .mockResolvedValueOnce(buildQueryResult([]))
      .mockResolvedValueOnce(
        buildQueryResult([
          {
            id: 'refresh-token-1',
            stakeholder_id: 'stakeholder-1',
            workspace_id: null,
            token_hash: service.hashToken('revoked-token'),
            created_at: new Date('2026-07-01T00:00:00.000Z'),
            expires_at: new Date('2026-08-01T00:00:00.000Z'),
            revoked_at: new Date('2026-07-02T00:00:00.000Z'),
            replaced_by_id: 'refresh-token-2',
          },
        ]),
      )
      .mockResolvedValueOnce(buildQueryResult([]));

    await expect(service.verifyAndRotate({ token: 'revoked-token' })).rejects.toThrow(
      InvalidRefreshTokenError,
    );

    expect(clientQuery).toHaveBeenNthCalledWith(3, 'ROLLBACK');
    expect(repository.create).not.toHaveBeenCalled();
    expect(repository.revoke).not.toHaveBeenCalled();
  });

  it('verifyAndRotate rejects an expired token', async () => {
    vi.useFakeTimers();
    try {
      vi.setSystemTime(new Date('2026-07-11T22:30:00.000Z'));
      clientQuery
        .mockResolvedValueOnce(buildQueryResult([]))
        .mockResolvedValueOnce(
          buildQueryResult([
            {
              id: 'refresh-token-1',
              stakeholder_id: 'stakeholder-1',
              workspace_id: null,
              token_hash: service.hashToken('expired-token'),
              created_at: new Date('2026-06-01T00:00:00.000Z'),
              expires_at: new Date('2026-07-01T00:00:00.000Z'),
              revoked_at: null,
              replaced_by_id: null,
            },
          ]),
        )
        .mockResolvedValueOnce(buildQueryResult([]));

      await expect(service.verifyAndRotate({ token: 'expired-token' })).rejects.toThrow(
        InvalidRefreshTokenError,
      );

      expect(clientQuery).toHaveBeenNthCalledWith(3, 'ROLLBACK');
      expect(repository.create).not.toHaveBeenCalled();
      expect(repository.revoke).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });
});
