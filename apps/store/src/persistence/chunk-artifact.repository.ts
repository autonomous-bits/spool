import type { Pool, PoolClient, QueryResult, QueryResultRow } from 'pg';
import { ConflictException, Inject, Injectable } from '@nestjs/common';
import { ChunkArtifactAssociation } from '../domain/chunk-artifact-association.js';
import type { ChunkArtifactAssociationStatus } from '../domain/types/vocabulary/chunk-artifact-association-status.js';
import { PG_POOL } from './pg-pool.token.js';

interface ChunkArtifactRow extends QueryResultRow {
  id: string;
  workspace_id: string;
  chunk_label: string;
  artifact_id: string;
  status: string;
  branch_id: string | null;
  origin_branch_id: string | null;
  created_by_stakeholder_id: string;
  updated_by_stakeholder_id: string;
  created_at: Date;
  updated_at: Date;
}

interface EffectiveRow extends QueryResultRow {
  artifact_id: string;
  status: string;
}

/**
 * Effective association tuple returned by `findEffectiveForChunk` (Meridian IDEA-32/IDEA-60/
 * IDEA-62 overlay read). `branchId` is `null` when the effective row is mainline-authoritative
 * (no branch-scoped row overrides it for this branch), or the queried branch's id when a
 * branch-scoped row is authoritative for that artifact.
 */
export interface EffectiveChunkArtifact {
  artifactId: string;
  branchId: string | null;
  status: ChunkArtifactAssociationStatus;
}

function toChunkArtifactAssociation(row: ChunkArtifactRow): ChunkArtifactAssociation {
  return new ChunkArtifactAssociation({
    id: row.id,
    workspaceId: row.workspace_id,
    chunkLabel: row.chunk_label,
    artifactId: row.artifact_id,
    status: row.status as ChunkArtifactAssociationStatus,
    ...(row.branch_id === null ? {} : { branchId: row.branch_id }),
    ...(row.origin_branch_id === null ? {} : { originBranchId: row.origin_branch_id }),
    createdByStakeholderId: row.created_by_stakeholder_id,
    updatedByStakeholderId: row.updated_by_stakeholder_id,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  });
}

type Queryable = Pick<Pool, 'query'>;

function createNonDraftBranchConflict(branchId: string): ConflictException {
  return new ConflictException(`Branch ${branchId} is not in draft status`);
}

async function insertChunkArtifact(
  queryable: Queryable,
  association: ChunkArtifactAssociation,
): Promise<ChunkArtifactAssociation> {
  const result: QueryResult<ChunkArtifactRow> = await queryable.query<ChunkArtifactRow>(
    `INSERT INTO chunk_artifacts (
       id, workspace_id, chunk_label, artifact_id, status,
       branch_id, origin_branch_id,
       created_by_stakeholder_id, updated_by_stakeholder_id,
       created_at, updated_at
     ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
     RETURNING *`,
    [
      association.id,
      association.workspaceId,
      association.chunkLabel,
      association.artifactId,
      association.status,
      association.branchId ?? null,
      association.originBranchId ?? null,
      association.createdByStakeholderId,
      association.updatedByStakeholderId,
      association.createdAt,
      association.updatedAt,
    ],
  );

  const row = result.rows[0];
  if (row === undefined) {
    throw new Error('ChunkArtifactRepository.create: INSERT ... RETURNING * produced no row');
  }

  return toChunkArtifactAssociation(row);
}

async function assertDraftBranchLock(
  client: PoolClient,
  branchId: string,
  workspaceId: string,
): Promise<void> {
  const result: QueryResult<{ status: string } & QueryResultRow> = await client.query(
    'SELECT status FROM branches WHERE id = $1 AND workspace_id = $2 FOR UPDATE',
    [branchId, workspaceId],
  );

  const row = result.rows[0];
  if (row?.status !== 'draft') {
    throw createNonDraftBranchConflict(branchId);
  }
}

/**
 * Postgres-backed repository for the ChunkArtifactAssociation aggregate (Meridian IDEA-32/
 * IDEA-60/IDEA-62), mirroring EdgeRepository's branch-scoped write and draft-lock pattern.
 */
@Injectable()
export class ChunkArtifactRepository {
  constructor(@Inject(PG_POOL) private readonly pool: Pool) {}

  /**
   * Persists a newly-constructed ChunkArtifactAssociation and returns the persisted entity
   * (round-tripped from the database row, not the in-memory instance).
   */
  async create(association: ChunkArtifactAssociation): Promise<ChunkArtifactAssociation> {
    if (association.branchId === undefined) {
      return insertChunkArtifact(this.pool, association);
    }

    const client = await this.pool.connect();
    let transactionOpen = false;

    try {
      await client.query('BEGIN');
      transactionOpen = true;

      await assertDraftBranchLock(client, association.branchId, association.workspaceId);
      await client.query('UPDATE branches SET updated_at = clock_timestamp() WHERE id = $1', [
        association.branchId,
      ]);

      const created = await insertChunkArtifact(client, association);

      await client.query('COMMIT');
      transactionOpen = false;
      return created;
    } catch (error) {
      if (transactionOpen) {
        await client.query('ROLLBACK');
      }
      throw error;
    } finally {
      client.release();
    }
  }

  /**
   * Set-based overlay read (Meridian IDEA-32/IDEA-60 branch-delta model): resolves the effective
   * set of artifact associations for `chunkLabel`, optionally overlaid by `branchId`'s
   * branch-scoped rows.
   *
   * Within each scope (mainline, and — if `branchId` is given — that branch), the
   * most-recently-created row per `artifact_id` is authoritative (ties broken by `id` so the
   * result is deterministic even for rows created in the same instant). A branch-authoritative
   * row for a given artifact always overrides that artifact's mainline-authoritative row,
   * including introducing a brand-new artifact association the mainline never had. The final
   * result is filtered to `status = 'active'` only — a deactivated (or, for schema parity,
   * superseded) winner means that artifact is not effectively associated with the chunk in this
   * scope, so it is omitted entirely rather than returned with a non-active status.
   */
  async findEffectiveForChunk(
    chunkLabel: string,
    branchId: string | undefined,
    workspaceId: string,
  ): Promise<EffectiveChunkArtifact[]> {
    const mainlineRows = await this.pool.query<EffectiveRow>(
      `SELECT DISTINCT ON (artifact_id) artifact_id, status
       FROM chunk_artifacts
       WHERE chunk_label = $1 AND branch_id IS NULL AND workspace_id = $2
       ORDER BY artifact_id, created_at DESC, id DESC`,
      [chunkLabel, workspaceId],
    );

    const effective = new Map<string, EffectiveChunkArtifact>();
    for (const row of mainlineRows.rows) {
      effective.set(row.artifact_id, {
        artifactId: row.artifact_id,
        branchId: null,
        status: row.status as ChunkArtifactAssociationStatus,
      });
    }

    if (branchId !== undefined) {
      const branchRows = await this.pool.query<EffectiveRow>(
        `SELECT DISTINCT ON (artifact_id) artifact_id, status
         FROM chunk_artifacts
         WHERE chunk_label = $1 AND branch_id = $2 AND workspace_id = $3
         ORDER BY artifact_id, created_at DESC, id DESC`,
        [chunkLabel, branchId, workspaceId],
      );

      for (const row of branchRows.rows) {
        effective.set(row.artifact_id, {
          artifactId: row.artifact_id,
          branchId,
          status: row.status as ChunkArtifactAssociationStatus,
        });
      }
    }

    return [...effective.values()].filter((entry) => entry.status === 'active');
  }
}
