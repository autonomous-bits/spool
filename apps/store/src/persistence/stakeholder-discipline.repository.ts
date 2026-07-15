import type { Pool, QueryResult, QueryResultRow } from 'pg';
import { Inject, Injectable } from '@nestjs/common';
import { StakeholderDiscipline } from '../domain/stakeholder-discipline.js';
import type { Discipline } from '../domain/types/vocabulary/discipline.js';
import { PG_POOL } from './pg-pool.token.js';

interface AllowedDisciplineRow extends QueryResultRow {
  discipline: Discipline;
}

interface StakeholderDisciplineRow extends QueryResultRow {
  workspace_id: string;
  stakeholder_id: string;
  discipline: Discipline;
  created_at: Date;
}

function toStakeholderDiscipline(row: StakeholderDisciplineRow): StakeholderDiscipline {
  return new StakeholderDiscipline({
    workspaceId: row.workspace_id,
    stakeholderId: row.stakeholder_id,
    discipline: row.discipline,
    createdAt: row.created_at,
  });
}

/**
 * Postgres-backed repository for the stakeholder_disciplines allow-list (Meridian IDEA-142/
 * IDEA-143, G21 SG1). SG2 adds the assign/revoke write path while preserving the migration-backed,
 * per-workspace discipline semantics.
 */
@Injectable()
export class StakeholderDisciplineRepository {
  constructor(@Inject(PG_POOL) private readonly pool: Pool) {}

  /**
   * Lists every discipline a stakeholder is allowed to act as within a specific workspace.
   */
  async listAllowed(workspaceId: string, stakeholderId: string): Promise<Discipline[]> {
    const result = await this.pool.query<AllowedDisciplineRow>(
      'SELECT discipline FROM stakeholder_disciplines WHERE workspace_id = $1 AND stakeholder_id = $2',
      [workspaceId, stakeholderId],
    );

    return result.rows.map((row) => row.discipline);
  }

  /**
   * Reports whether a stakeholder is allowed to act as the given discipline within a specific
   * workspace. Scoped exactly to (workspaceId, stakeholderId, discipline) — a different workspace
   * or a discipline never assigned for this pair returns false.
   */
  async isAllowed(
    workspaceId: string,
    stakeholderId: string,
    discipline: Discipline,
  ): Promise<boolean> {
    const result = await this.pool.query(
      `SELECT 1 FROM stakeholder_disciplines
        WHERE workspace_id = $1 AND stakeholder_id = $2 AND discipline = $3`,
      [workspaceId, stakeholderId, discipline],
    );

    return result.rows.length > 0;
  }

  async assign(
    workspaceId: string,
    stakeholderId: string,
    discipline: Discipline,
  ): Promise<StakeholderDiscipline> {
    await this.pool.query(
      `INSERT INTO stakeholder_disciplines (workspace_id, stakeholder_id, discipline)
       VALUES ($1, $2, $3)
       ON CONFLICT (workspace_id, stakeholder_id, discipline) DO NOTHING`,
      [workspaceId, stakeholderId, discipline],
    );

    const result: QueryResult<StakeholderDisciplineRow> =
      await this.pool.query<StakeholderDisciplineRow>(
        `SELECT workspace_id, stakeholder_id, discipline, created_at
         FROM stakeholder_disciplines
         WHERE workspace_id = $1 AND stakeholder_id = $2 AND discipline = $3`,
        [workspaceId, stakeholderId, discipline],
      );

    const row = result.rows[0];
    if (row === undefined) {
      throw new Error(
        'StakeholderDisciplineRepository.assign: expected stakeholder discipline row after insert/select round-trip',
      );
    }

    return toStakeholderDiscipline(row);
  }

  async revoke(
    workspaceId: string,
    stakeholderId: string,
    discipline: Discipline,
  ): Promise<boolean> {
    const result = await this.pool.query(
      `DELETE FROM stakeholder_disciplines
       WHERE workspace_id = $1 AND stakeholder_id = $2 AND discipline = $3
       RETURNING 1`,
      [workspaceId, stakeholderId, discipline],
    );

    return (result.rowCount ?? 0) > 0;
  }
}
