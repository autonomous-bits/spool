import type { Pool, PoolClient, QueryResult, QueryResultRow } from 'pg';
import { Inject, Injectable } from '@nestjs/common';
import { PG_POOL } from './pg-pool.token.js';

interface RefreshTokenDbRow extends QueryResultRow {
  id: string;
  stakeholder_id: string;
  workspace_id: string | null;
  token_hash: string;
  created_at: Date;
  expires_at: Date;
  revoked_at: Date | null;
  replaced_by_id: string | null;
}

interface RefreshTokenIdRow extends QueryResultRow {
  id: string;
}

type Queryable = Pick<Pool, 'query'> | PoolClient;

export interface RefreshTokenRow {
  id: string;
  stakeholderId: string;
  workspaceId: string | null;
  tokenHash: string;
  createdAt: Date;
  expiresAt: Date;
  revokedAt: Date | null;
  replacedById: string | null;
}

export interface CreateRefreshTokenInput {
  stakeholderId: string;
  workspaceId: string | null;
  tokenHash: string;
  expiresAt: Date;
}

function toRefreshTokenRow(row: RefreshTokenDbRow): RefreshTokenRow {
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
export class RefreshTokenRepository {
  constructor(@Inject(PG_POOL) private readonly pool: Pool) {}

  async create(
    input: CreateRefreshTokenInput,
    client?: PoolClient,
  ): Promise<{ id: string }> {
    const result: QueryResult<RefreshTokenIdRow> = await (client ?? this.pool).query<RefreshTokenIdRow>(
      `INSERT INTO refresh_tokens (stakeholder_id, workspace_id, token_hash, expires_at)
       VALUES ($1, $2, $3, $4)
       RETURNING id`,
      [input.stakeholderId, input.workspaceId, input.tokenHash, input.expiresAt],
    );

    const row = result.rows[0];
    if (row === undefined) {
      throw new Error(
        'RefreshTokenRepository.create: INSERT refresh_tokens ... RETURNING id produced no row',
      );
    }

    return { id: row.id };
  }

  async findActiveByTokenHash(
    tokenHash: string,
    client?: PoolClient,
  ): Promise<RefreshTokenRow | undefined> {
    const result: QueryResult<RefreshTokenDbRow> = await (client ?? this.pool).query<RefreshTokenDbRow>(
      `SELECT id, stakeholder_id, workspace_id, token_hash, created_at, expires_at, revoked_at, replaced_by_id
       FROM refresh_tokens
       WHERE token_hash = $1
         AND revoked_at IS NULL
         AND expires_at > now()
       LIMIT 1`,
      [tokenHash],
    );

    const row = result.rows[0];
    return row === undefined ? undefined : toRefreshTokenRow(row);
  }

  async revoke(id: string, replacedById?: string, client?: PoolClient): Promise<void> {
    const queryable: Queryable = client ?? this.pool;
    await queryable.query(
      `UPDATE refresh_tokens
       SET revoked_at = now(),
           replaced_by_id = $2
       WHERE id = $1`,
      [id, replacedById ?? null],
    );
  }
}
