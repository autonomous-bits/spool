import { randomUUID } from 'node:crypto';
import type { Pool, QueryResult, QueryResultRow } from 'pg';
import { Inject, Injectable } from '@nestjs/common';
import { Branch } from '../domain/branch.js';
import { DivergencePoint } from '../domain/divergence-point.js';
import { DeliveryAttemptRepository } from './delivery-attempt.repository.js';
import { PG_POOL } from './pg-pool.token.js';

export interface BranchRow extends QueryResultRow {
  id: string;
  workspace_id: string;
  name: string;
  discipline: string;
  status: string;
  diverged_at: Date;
  submitted_at: Date | null;
  verified_at: Date | null;
  merged_at: Date | null;
  merged_by_stakeholder_id: string | null;
  origin_suggestion_id: string | null;
  created_by_stakeholder_id: string;
  created_at: Date;
  updated_at: Date;
}

interface TimestampRow extends QueryResultRow {
  attempted_at: Date;
}

interface LabelRow extends QueryResultRow {
  label: string;
}

interface EdgeIdentityRow extends QueryResultRow {
  from_chunk_label: string;
  to_chunk_label: string;
  type: string;
}

interface ChunkArtifactIdentityRow extends QueryResultRow {
  chunk_label: string;
  artifact_id: string;
}

interface DeliverySubscriptionIdRow extends QueryResultRow {
  id: string;
}

/**
 * Result of a successful or conflicted merge attempt (Meridian IDEA-74/IDEA-46, scoped per G06
 * OQ2 to "any mainline identity collision blocks the merge"). `undefined` is reserved for the
 * distinct "branch not mergeable" outcome (not verified / not found).
 */
export type BranchMergeResult =
  | { kind: 'merged'; branch: Branch }
  | { kind: 'conflict'; reason: string };

function edgeIdentityKey(row: Pick<EdgeIdentityRow, 'from_chunk_label' | 'to_chunk_label' | 'type'>): string {
  return `${row.from_chunk_label}\u0000${row.to_chunk_label}\u0000${row.type}`;
}

function chunkArtifactIdentityKey(row: ChunkArtifactIdentityRow): string {
  return `${row.chunk_label}\u0000${row.artifact_id}`;
}

export function toBranch(row: BranchRow): Branch {
  return new Branch({
    id: row.id,
    workspaceId: row.workspace_id,
    name: row.name,
    discipline: row.discipline as Branch['discipline'],
    status: row.status as Branch['status'],
    divergedAt: new DivergencePoint(row.diverged_at.toISOString()),
    ...(row.submitted_at === null ? {} : { submittedAt: row.submitted_at }),
    ...(row.verified_at === null ? {} : { verifiedAt: row.verified_at }),
    ...(row.merged_at === null ? {} : { mergedAt: row.merged_at }),
    ...(row.merged_by_stakeholder_id === null
      ? {}
      : { mergedByStakeholderId: row.merged_by_stakeholder_id }),
    ...(row.origin_suggestion_id === null
      ? {}
      : { originSuggestionId: row.origin_suggestion_id }),
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
  constructor(
    @Inject(PG_POOL) private readonly pool: Pool,
    private readonly deliveryAttemptRepository: DeliveryAttemptRepository = new DeliveryAttemptRepository(
      pool,
    ),
  ) {}

  /**
   * Persists a newly-constructed Branch as a draft and returns the persisted entity
   * (round-tripped from the database row, not the in-memory instance).
   */
  async create(branch: Branch): Promise<Branch> {
    const result: QueryResult<BranchRow> = await this.pool.query<BranchRow>(
      `WITH persisted_timestamps AS (
         SELECT clock_timestamp() AS persisted_at
       )
       INSERT INTO branches (
         id, workspace_id, name, discipline, status, diverged_at,
         created_by_stakeholder_id, created_at, updated_at
       )
       SELECT
         $1, $2, $3, $4, $5, $6, $7,
         persisted_timestamps.persisted_at,
         persisted_timestamps.persisted_at
       FROM persisted_timestamps
       RETURNING *`,
      [
        branch.id,
        branch.workspaceId,
        branch.name,
        branch.discipline,
        branch.status,
        branch.divergedAt.toISOString(),
        branch.createdByStakeholderId,
      ],
    );

    const row = result.rows[0];
    if (row === undefined) {
      throw new Error('BranchRepository.create: INSERT ... RETURNING * produced no row');
    }

    return toBranch(row);
  }

  /**
   * Looks up a branch by id, scoped to a workspace (Meridian IDEA-98/IDEA-100, G11 SG4). A
   * cross-workspace id is indistinguishable from "does not exist" (returns `undefined`, not a
   * scope violation) so a lookup can never leak whether an id exists in another workspace.
   */
  async findById(id: string, workspaceId: string): Promise<Branch | undefined> {
    const result: QueryResult<BranchRow> = await this.pool.query<BranchRow>(
      'SELECT * FROM branches WHERE id = $1 AND workspace_id = $2',
      [id, workspaceId],
    );

    const row = result.rows[0];
    return row === undefined ? undefined : toBranch(row);
  }

  async submit(branchId: string, workspaceId: string): Promise<Branch | undefined> {
    const client = await this.pool.connect();
    let transactionOpen = false;

    try {
      await client.query('BEGIN');
      transactionOpen = true;

      const timestampResult: QueryResult<TimestampRow> =
        await client.query<TimestampRow>('SELECT clock_timestamp() AS attempted_at');
      const attemptedAt = timestampResult.rows[0]?.attempted_at;
      if (attemptedAt === undefined) {
        throw new Error('BranchRepository.submit: SELECT clock_timestamp() produced no row');
      }

      const branchResult: QueryResult<BranchRow> = await client.query<BranchRow>(
        'SELECT * FROM branches WHERE id = $1 AND workspace_id = $2 FOR UPDATE',
        [branchId, workspaceId],
      );
      const branchRow = branchResult.rows[0];
      if (
        branchRow?.status !== 'draft' ||
        branchRow.updated_at > attemptedAt
      ) {
        await client.query('ROLLBACK');
        transactionOpen = false;
        return undefined;
      }

      const result: QueryResult<BranchRow> = await client.query<BranchRow>(
        "UPDATE branches SET status='submitted', submitted_at=now(), updated_at=now() WHERE id=$1 AND workspace_id=$2 RETURNING *",
        [branchId, workspaceId],
      );

      const row = result.rows[0];
      if (row === undefined) {
        throw new Error('BranchRepository.submit: UPDATE ... RETURNING * produced no row');
      }

      await client.query('COMMIT');
      transactionOpen = false;
      return toBranch(row);
    } catch (error) {
      if (transactionOpen) {
        await client.query('ROLLBACK');
      }
      throw error;
    } finally {
      client.release();
    }
  }

  async verify(branchId: string, workspaceId: string): Promise<Branch | undefined> {
    const client = await this.pool.connect();
    let transactionOpen = false;

    try {
      await client.query('BEGIN');
      transactionOpen = true;

      const timestampResult: QueryResult<TimestampRow> =
        await client.query<TimestampRow>('SELECT clock_timestamp() AS attempted_at');
      const attemptedAt = timestampResult.rows[0]?.attempted_at;
      if (attemptedAt === undefined) {
        throw new Error('BranchRepository.verify: SELECT clock_timestamp() produced no row');
      }

      const branchResult: QueryResult<BranchRow> = await client.query<BranchRow>(
        'SELECT * FROM branches WHERE id = $1 AND workspace_id = $2 FOR UPDATE',
        [branchId, workspaceId],
      );
      const branchRow = branchResult.rows[0];
      if (
        branchRow?.status !== 'submitted' ||
        branchRow.updated_at > attemptedAt
      ) {
        await client.query('ROLLBACK');
        transactionOpen = false;
        return undefined;
      }

      const result: QueryResult<BranchRow> = await client.query<BranchRow>(
        "UPDATE branches SET status='verified', verified_at=now(), updated_at=now() WHERE id=$1 AND workspace_id=$2 RETURNING *",
        [branchId, workspaceId],
      );

      const row = result.rows[0];
      if (row === undefined) {
        throw new Error('BranchRepository.verify: UPDATE ... RETURNING * produced no row');
      }

      await client.query('COMMIT');
      transactionOpen = false;
      return toBranch(row);
    } catch (error) {
      if (transactionOpen) {
        await client.query('ROLLBACK');
      }
      throw error;
    } finally {
      client.release();
    }
  }

  async reject(branchId: string, workspaceId: string): Promise<Branch | undefined> {
    const client = await this.pool.connect();
    let transactionOpen = false;

    try {
      await client.query('BEGIN');
      transactionOpen = true;

      const timestampResult: QueryResult<TimestampRow> =
        await client.query<TimestampRow>('SELECT clock_timestamp() AS attempted_at');
      const attemptedAt = timestampResult.rows[0]?.attempted_at;
      if (attemptedAt === undefined) {
        throw new Error('BranchRepository.reject: SELECT clock_timestamp() produced no row');
      }

      const branchResult: QueryResult<BranchRow> = await client.query<BranchRow>(
        'SELECT * FROM branches WHERE id = $1 AND workspace_id = $2 FOR UPDATE',
        [branchId, workspaceId],
      );
      const branchRow = branchResult.rows[0];
      if (
        branchRow === undefined ||
        !['submitted', 'verified'].includes(branchRow.status) ||
        branchRow.updated_at > attemptedAt
      ) {
        await client.query('ROLLBACK');
        transactionOpen = false;
        return undefined;
      }

      const result: QueryResult<BranchRow> = await client.query<BranchRow>(
        "UPDATE branches SET status='draft', verified_at=NULL, submitted_at=NULL, updated_at=now() WHERE id=$1 AND workspace_id=$2 RETURNING *",
        [branchId, workspaceId],
      );

      const row = result.rows[0];
      if (row === undefined) {
        throw new Error('BranchRepository.reject: UPDATE ... RETURNING * produced no row');
      }

      await client.query('COMMIT');
      transactionOpen = false;
      return toBranch(row);
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
   * Merges a verified branch into mainline as a single atomic transaction (Meridian IDEA-40's
   * verified -> merged transition, IDEA-74's merge-lineage/provenance shape, IDEA-46's conflict
   * gate scoped per G06 OQ2 to "any mainline identity collision blocks the merge").
   *
   * Promotes every draft chunk and active edge attached to the branch (branch_id -> NULL,
   * chunk status -> 'promoted'; origin_branch_id is left untouched, since it was already set to
   * this branch's id at capture time), promotes only the authoritative (most-recently-created)
   * branch-scoped chunk_artifacts row per (chunk_label, artifact_id) pair (Meridian IDEA-46's
   * chunk-artifact-modification conflict scope; the repo-local collision mechanics mirror the
   * chunk-label/edge-identity checks below, extended to chunk_artifacts), and marks the branch
   * 'merged' with merged_at/merged_by_stakeholder_id — or rejects the merge in full, with zero
   * rows mutated, if any branch chunk label, edge identity ((from,to,type)), or *active*
   * chunk-artifact pair ((chunk_label, artifact_id)) already exists on mainline. Older,
   * non-authoritative branch-scoped chunk_artifacts rows sharing a pair are left untouched as
   * branch history — only the authoritative row per pair is ever promoted, so promotion can never
   * violate IDEA-64's partial unique index. A deactivated authoritative row is promoted
   * unconditionally (no collision check), since deactivation can never collide with mainline's
   * uniqueness constraint. Returns `undefined` (no mutation at all) when the branch is not found
   * or not in 'verified' status, so callers can distinguish "not mergeable" from a genuine
   * identity conflict.
   */
  async merge(
    branchId: string,
    workspaceId: string,
    mergedByStakeholderId: string,
  ): Promise<BranchMergeResult | undefined> {
    const client = await this.pool.connect();
    let transactionOpen = false;

    try {
      await client.query('BEGIN');
      transactionOpen = true;

      const branchResult: QueryResult<BranchRow> = await client.query<BranchRow>(
        'SELECT * FROM branches WHERE id = $1 AND workspace_id = $2 FOR UPDATE',
        [branchId, workspaceId],
      );
      const branchRow = branchResult.rows[0];
      if (branchRow?.status !== 'verified') {
        await client.query('ROLLBACK');
        transactionOpen = false;
        return undefined;
      }

      // G11 SG4: every collision check and mutation below is additionally scoped to
      // `workspace_id` (both the branch-scoped and mainline sides), so a merge can never
      // collide with — or promote into — a row from a different workspace.
      const chunkLabelCollisions: QueryResult<LabelRow> = await client.query<LabelRow>(
        `SELECT branch_chunks.label
           FROM chunks branch_chunks
           JOIN chunks mainline_chunks
             ON mainline_chunks.label = branch_chunks.label
            AND mainline_chunks.workspace_id = branch_chunks.workspace_id
            AND mainline_chunks.branch_id IS NULL
            AND mainline_chunks.status = 'promoted'
          WHERE branch_chunks.branch_id = $1
            AND branch_chunks.workspace_id = $2
            AND branch_chunks.status = 'draft'`,
        [branchId, workspaceId],
      );
      if (chunkLabelCollisions.rows.length > 0) {
        await client.query('ROLLBACK');
        transactionOpen = false;
        return {
          kind: 'conflict',
          reason: `mainline chunk label collision: ${chunkLabelCollisions.rows.map((row) => row.label).join(', ')}`,
        };
      }

      const edgeIdentityCollisions: QueryResult<EdgeIdentityRow> = await client.query<EdgeIdentityRow>(
        `SELECT branch_edges.from_chunk_label, branch_edges.to_chunk_label, branch_edges.type
           FROM edges branch_edges
           JOIN edges mainline_edges
             ON mainline_edges.from_chunk_label = branch_edges.from_chunk_label
            AND mainline_edges.to_chunk_label = branch_edges.to_chunk_label
            AND mainline_edges.type = branch_edges.type
            AND mainline_edges.workspace_id = branch_edges.workspace_id
            AND mainline_edges.branch_id IS NULL
            AND mainline_edges.status = 'active'
          WHERE branch_edges.branch_id = $1
            AND branch_edges.workspace_id = $2
            AND branch_edges.status = 'active'`,
        [branchId, workspaceId],
      );
      if (edgeIdentityCollisions.rows.length > 0) {
        await client.query('ROLLBACK');
        transactionOpen = false;
        return {
          kind: 'conflict',
          reason: `mainline edge identity collision: ${edgeIdentityCollisions.rows.map(edgeIdentityKey).join(', ')}`,
        };
      }

      // Meridian IDEA-46/IDEA-62/IDEA-64: only the most-recently-created branch-scoped
      // chunk_artifacts row per (chunk_label, artifact_id) pair is authoritative for this branch;
      // older rows sharing a pair are branch history and must never be promoted, so promotion
      // can never violate IDEA-64's mainline partial unique index.
      const chunkArtifactCollisions: QueryResult<ChunkArtifactIdentityRow> =
        await client.query<ChunkArtifactIdentityRow>(
          `WITH authoritative_branch_rows AS (
             SELECT DISTINCT ON (chunk_label, artifact_id) chunk_label, artifact_id, status
               FROM chunk_artifacts
              WHERE branch_id = $1
                AND workspace_id = $2
              ORDER BY chunk_label, artifact_id, created_at DESC, id DESC
           )
           SELECT authoritative_branch_rows.chunk_label, authoritative_branch_rows.artifact_id
             FROM authoritative_branch_rows
             JOIN chunk_artifacts mainline_chunk_artifacts
               ON mainline_chunk_artifacts.chunk_label = authoritative_branch_rows.chunk_label
              AND mainline_chunk_artifacts.artifact_id = authoritative_branch_rows.artifact_id
              AND mainline_chunk_artifacts.workspace_id = $2
              AND mainline_chunk_artifacts.branch_id IS NULL
              AND mainline_chunk_artifacts.status = 'active'
            WHERE authoritative_branch_rows.status = 'active'`,
          [branchId, workspaceId],
        );
      if (chunkArtifactCollisions.rows.length > 0) {
        await client.query('ROLLBACK');
        transactionOpen = false;
        return {
          kind: 'conflict',
          reason: `mainline chunk-artifact association collision: ${chunkArtifactCollisions.rows.map(chunkArtifactIdentityKey).join(', ')}`,
        };
      }

      await client.query(
        "UPDATE chunks SET branch_id = NULL, status = 'promoted', updated_at = now() WHERE branch_id = $1 AND workspace_id = $2 AND status = 'draft'",
        [branchId, workspaceId],
      );
      await client.query(
        'UPDATE edges SET branch_id = NULL, updated_at = now() WHERE branch_id = $1 AND workspace_id = $2 AND status = $3',
        [branchId, workspaceId, 'active'],
      );
      // Promote only the authoritative (most-recently-created) branch-scoped chunk_artifacts row
      // per (chunk_label, artifact_id) pair — active rows are safe here since the collision check
      // above already ruled out any mainline conflict; deactivated rows are promoted
      // unconditionally, since a deactivation can never violate IDEA-64's active-only unique
      // index. Older, non-authoritative rows sharing a pair are intentionally left untouched.
      await client.query(
        `WITH authoritative_branch_rows AS (
           SELECT DISTINCT ON (chunk_label, artifact_id) id
             FROM chunk_artifacts
            WHERE branch_id = $1
              AND workspace_id = $2
            ORDER BY chunk_label, artifact_id, created_at DESC, id DESC
         )
         UPDATE chunk_artifacts
            SET branch_id = NULL, updated_at = now()
          WHERE id IN (SELECT id FROM authoritative_branch_rows)`,
        [branchId, workspaceId],
      );

      const mergedResult: QueryResult<BranchRow> = await client.query<BranchRow>(
        `UPDATE branches
            SET status = 'merged', merged_at = now(), merged_by_stakeholder_id = $3, updated_at = now()
          WHERE id = $1 AND workspace_id = $2
          RETURNING *`,
        [branchId, workspaceId, mergedByStakeholderId],
      );

      const mergedRow = mergedResult.rows[0];
      if (mergedRow === undefined) {
        throw new Error('BranchRepository.merge: UPDATE ... RETURNING * produced no row');
      }
      if (mergedRow.merged_at === null) {
        throw new Error('BranchRepository.merge: merged row had null merged_at after merge update');
      }

      const matchingSubscriptions: QueryResult<DeliverySubscriptionIdRow> =
        await client.query<DeliverySubscriptionIdRow>(
          `SELECT id
             FROM delivery_subscriptions
            WHERE workspace_id = $1
              AND is_active = true
              AND (discipline_filter IS NULL OR discipline_filter ? $2)`,
          [workspaceId, branchRow.discipline],
        );
      const mergeEventId = randomUUID();
      for (const subscriptionRow of matchingSubscriptions.rows) {
        await this.deliveryAttemptRepository.createPending(
          {
            subscriptionId: subscriptionRow.id,
            mergeEventId,
            branchId,
            mergedAt: mergedRow.merged_at,
          },
          client,
        );
      }

      await client.query('COMMIT');
      transactionOpen = false;
      return { kind: 'merged', branch: toBranch(mergedRow) };
    } catch (error) {
      if (transactionOpen) {
        await client.query('ROLLBACK');
      }
      throw error;
    } finally {
      client.release();
    }
  }
}
