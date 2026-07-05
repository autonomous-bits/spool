import type { Pool, QueryResult, QueryResultRow } from 'pg';
import { Inject, Injectable } from '@nestjs/common';
import { PG_POOL } from './pg-pool.token.js';

interface StakeholderRow extends QueryResultRow {
  id: string;
  discipline: string | null;
}

/**
 * Minimal stakeholder projection needed by auth/session-token flows: identity plus discipline
 * (nullable — not every stakeholder has one assigned yet, e.g. the bootstrap stakeholder).
 */
export interface StakeholderRecord {
  id: string;
  discipline: string | null;
}

function toStakeholderRecord(row: StakeholderRow): StakeholderRecord {
  return {
    id: row.id,
    discipline: row.discipline,
  };
}

/**
 * Postgres-backed repository for the Stakeholder aggregate (Meridian IDEA-31's authoritative
 * schema). Only exposes the lookups needed by the current goal's slices.
 */
@Injectable()
export class StakeholderRepository {
  constructor(@Inject(PG_POOL) private readonly pool: Pool) {}

  async findById(id: string): Promise<StakeholderRecord | undefined> {
    const result: QueryResult<StakeholderRow> = await this.pool.query<StakeholderRow>(
      'SELECT id, discipline FROM stakeholders WHERE id = $1',
      [id],
    );

    const row = result.rows[0];
    return row === undefined ? undefined : toStakeholderRecord(row);
  }

  /**
   * Looks up a stakeholder by their resolved GitHub login (Meridian IDEA-81's OAuth mechanism).
   * Returns `undefined` as an explicit not-found result rather than throwing.
   */
  async findByGithubLogin(githubLogin: string): Promise<StakeholderRecord | undefined> {
    const result: QueryResult<StakeholderRow> = await this.pool.query<StakeholderRow>(
      'SELECT id, discipline FROM stakeholders WHERE github_login = $1',
      [githubLogin],
    );

    const row = result.rows[0];
    return row === undefined ? undefined : toStakeholderRecord(row);
  }
}
