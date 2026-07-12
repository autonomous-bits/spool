import type { Pool, PoolClient, QueryResult, QueryResultRow } from 'pg';
import { Inject, Injectable } from '@nestjs/common';
import { PG_POOL } from './pg-pool.token.js';

interface PairingCodeIdRow extends QueryResultRow {
  id: string;
}

interface PairingCodeTokensRow extends QueryResultRow {
  session_token: string;
  refresh_token: string;
}

export interface CreatePairingCodeInput {
  codeHash: string;
  sessionToken: string;
  refreshToken: string;
  expiresAt: Date;
}

@Injectable()
export class PairingCodeRepository {
  constructor(@Inject(PG_POOL) private readonly pool: Pool) {}

  async create(input: CreatePairingCodeInput, client?: PoolClient): Promise<{ id: string }> {
    const result: QueryResult<PairingCodeIdRow> = await (client ?? this.pool).query<PairingCodeIdRow>(
      `INSERT INTO pairing_codes (code_hash, session_token, refresh_token, expires_at)
       VALUES ($1, $2, $3, $4)
       RETURNING id`,
      [input.codeHash, input.sessionToken, input.refreshToken, input.expiresAt],
    );

    const row = result.rows[0];
    if (row === undefined) {
      throw new Error(
        'PairingCodeRepository.create: INSERT pairing_codes ... RETURNING id produced no row',
      );
    }

    return { id: row.id };
  }

  async consume(
    codeHash: string,
    client?: PoolClient,
  ): Promise<{ sessionToken: string; refreshToken: string } | undefined> {
    const result: QueryResult<PairingCodeTokensRow> = await (client ?? this.pool).query<PairingCodeTokensRow>(
      `UPDATE pairing_codes
       SET consumed_at = now()
       WHERE code_hash = $1
         AND consumed_at IS NULL
         AND expires_at > now()
       RETURNING session_token, refresh_token`,
      [codeHash],
    );

    const row = result.rows[0];
    if (row === undefined) {
      return undefined;
    }

    return {
      sessionToken: row.session_token,
      refreshToken: row.refresh_token,
    };
  }
}
