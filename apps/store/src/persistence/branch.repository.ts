import type { Pool, QueryResult, QueryResultRow } from 'pg';
import { Inject, Injectable } from '@nestjs/common';
import { Branch } from '../domain/branch.js';
import { DivergencePoint } from '../domain/divergence-point.js';
import { PG_POOL } from './pg-pool.token.js';

interface BranchRow extends QueryResultRow {
  id: string;
  name: string;
  discipline: string;
  status: string;
  diverged_at: Date;
  created_by_stakeholder_id: string;
  created_at: Date;
  updated_at: Date;
}

function toBranch(row: BranchRow): Branch {
  return new Branch({
    id: row.id,
    name: row.name,
    discipline: row.discipline as Branch['discipline'],
    status: row.status as Branch['status'],
    divergedAt: new DivergencePoint(row.diverged_at.toISOString()),
    createdByStakeholderId: row.created_by_stakeholder_id,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  });
}

/**
 * Postgres-backed repository for the Branch aggregate (Meridian IDEA-31's authoritative schema).
 * G02 only ever persists draft branches created directly by a stakeholder; submitted_at,
 * merged_at, origin_suggestion_id, and merged_by_stakeholder_id are always NULL on create.
 */
@Injectable()
export class BranchRepository {
  constructor(@Inject(PG_POOL) private readonly pool: Pool) {}

  /**
   * Persists a newly-constructed Branch as a draft and returns the persisted entity
   * (round-tripped from the database row, not the in-memory instance).
   */
  async create(branch: Branch): Promise<Branch> {
    const result: QueryResult<BranchRow> = await this.pool.query<BranchRow>(
      `INSERT INTO branches (
         id, name, discipline, status, diverged_at,
         created_by_stakeholder_id, created_at, updated_at
       ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
       RETURNING *`,
      [
        branch.id,
        branch.name,
        branch.discipline,
        branch.status,
        branch.divergedAt.toISOString(),
        branch.createdByStakeholderId,
        branch.createdAt,
        branch.updatedAt,
      ],
    );

    const row = result.rows[0];
    if (row === undefined) {
      throw new Error('BranchRepository.create: INSERT ... RETURNING * produced no row');
    }

    return toBranch(row);
  }

  /**
   * Looks up a branch by id. Returns `undefined` as an explicit not-found result rather than
   * throwing, so callers can distinguish "not found" from an actual persistence error.
   */
  async findById(id: string): Promise<Branch | undefined> {
    const result: QueryResult<BranchRow> = await this.pool.query<BranchRow>(
      'SELECT * FROM branches WHERE id = $1',
      [id],
    );

    const row = result.rows[0];
    return row === undefined ? undefined : toBranch(row);
  }
}
