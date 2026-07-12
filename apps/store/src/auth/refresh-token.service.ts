import { randomBytes, createHash } from 'node:crypto';
import type { Pool, PoolClient, QueryResult, QueryResultRow } from 'pg';
import { Inject, Injectable } from '@nestjs/common';
import type { AuthConfig } from './auth-config.js';
import { AUTH_CONFIG } from './auth-config.token.js';
import { RefreshTokenRepository } from '../persistence/refresh-token.repository.js';
import { PG_POOL } from '../persistence/pg-pool.token.js';

interface RefreshTokenLookupRow extends QueryResultRow {
  id: string;
  stakeholder_id: string;
  workspace_id: string | null;
  token_hash: string;
  created_at: Date;
  expires_at: Date;
  revoked_at: Date | null;
  replaced_by_id: string | null;
}

interface RefreshTokenLookupRecord {
  id: string;
  stakeholderId: string;
  workspaceId: string | null;
  tokenHash: string;
  createdAt: Date;
  expiresAt: Date;
  revokedAt: Date | null;
  replacedById: string | null;
}

export interface IssueRefreshTokenInput {
  stakeholderId: string;
  workspaceId: string | null;
  maxAgeSeconds?: number;
}

export interface IssuedRefreshToken {
  token: string;
  expiresAt: number;
}

export interface VerifyAndRotateRefreshTokenInput {
  token: string;
  stakeholderIdHint?: string;
}

export interface RotatedRefreshToken {
  stakeholderId: string;
  workspaceId: string | null;
  newToken: string;
  newExpiresAt: number;
}

export class InvalidRefreshTokenError extends Error {
  constructor(reason: string) {
    super(`Invalid refresh token: ${reason}`);
    this.name = 'InvalidRefreshTokenError';
  }
}

function nowSeconds(): number {
  return Math.floor(Date.now() / 1000);
}

function toRefreshTokenLookupRecord(row: RefreshTokenLookupRow): RefreshTokenLookupRecord {
  return {
    id: row.id,
    stakeholderId: row.stakeholder_id,
    workspaceId: row.workspace_id,
    tokenHash: row.token_hash,
    createdAt: row.created_at,
    expiresAt: row.expires_at,
    revokedAt: row.revoked_at,
    replacedById: row.replaced_by_id,
  };
}

@Injectable()
export class RefreshTokenService {
  constructor(
    @Inject(AUTH_CONFIG) private readonly config: AuthConfig,
    @Inject(PG_POOL) private readonly pool: Pool,
    private readonly refreshTokenRepository: RefreshTokenRepository,
  ) {}

  async issue(input: IssueRefreshTokenInput): Promise<IssuedRefreshToken> {
    return this.issueWithMaxAge(input.stakeholderId, input.workspaceId, input.maxAgeSeconds);
  }

  async verifyAndRotate(
    input: VerifyAndRotateRefreshTokenInput,
  ): Promise<RotatedRefreshToken> {
    const tokenHash = this.hashToken(input.token);
    const client = await this.pool.connect();
    let transactionOpen = false;

    try {
      await client.query('BEGIN');
      transactionOpen = true;

      const lookupResult: QueryResult<RefreshTokenLookupRow> = await client.query<RefreshTokenLookupRow>(
        `SELECT id, stakeholder_id, workspace_id, token_hash, created_at, expires_at, revoked_at, replaced_by_id
         FROM refresh_tokens
         WHERE token_hash = $1
         FOR UPDATE`,
        [tokenHash],
      );

      const row = lookupResult.rows[0];
      if (row === undefined) {
        throw new InvalidRefreshTokenError('token not found');
      }

      const currentToken = toRefreshTokenLookupRecord(row);
      if (currentToken.revokedAt !== null) {
        throw new InvalidRefreshTokenError('token revoked');
      }
      if (currentToken.expiresAt.getTime() <= Date.now()) {
        throw new InvalidRefreshTokenError('token expired');
      }
      if (
        input.stakeholderIdHint !== undefined &&
        input.stakeholderIdHint !== currentToken.stakeholderId
      ) {
        throw new InvalidRefreshTokenError('stakeholder mismatch');
      }

      const issued = await this.issueWithMaxAge(
        currentToken.stakeholderId,
        currentToken.workspaceId,
        undefined,
        client,
      );

      await this.refreshTokenRepository.revoke(currentToken.id, issued.id, client);

      await client.query('COMMIT');
      transactionOpen = false;

      return {
        stakeholderId: currentToken.stakeholderId,
        workspaceId: currentToken.workspaceId,
        newToken: issued.token,
        newExpiresAt: issued.expiresAt,
      };
    } catch (error) {
      if (transactionOpen) {
        await client.query('ROLLBACK');
      }
      throw error;
    } finally {
      client.release();
    }
  }

  hashToken(token: string): string {
    return createHash('sha256').update(token).digest('hex');
  }

  private async issueWithMaxAge(
    stakeholderId: string,
    workspaceId: string | null,
    maxAgeSeconds = this.config.refreshTokenMaxAgeSeconds,
    client?: PoolClient,
  ): Promise<IssuedRefreshToken & { id: string }> {
    const token = randomBytes(32).toString('base64url');
    const expiresAt = nowSeconds() + maxAgeSeconds;
    const tokenHash = this.hashToken(token);
    const created = await this.refreshTokenRepository.create(
      {
        stakeholderId,
        workspaceId,
        tokenHash,
        expiresAt: new Date(expiresAt * 1000),
      },
      client,
    );

    return {
      id: created.id,
      token,
      expiresAt,
    };
  }
}
