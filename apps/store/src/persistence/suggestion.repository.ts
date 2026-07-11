import type { Pool, PoolClient, QueryResult, QueryResultRow } from 'pg';
import { Inject, Injectable } from '@nestjs/common';
import { Branch } from '../domain/branch.js';
import { Suggestion, type SuggestionVariant } from '../domain/suggestion.js';
import { PG_POOL } from './pg-pool.token.js';
import { toBranch, type BranchRow } from './branch.repository.js';

interface SuggestionRow extends QueryResultRow {
  id: string;
  workspace_id: string;
  label: string | null;
  content: string | null;
  from_chunk_label: string | null;
  to_chunk_label: string | null;
  relationship_type: string | null;
  discipline: string;
  status: string;
  submitted_by_stakeholder_id: string;
  submitted_by_actor_kind: string;
  decided_by_stakeholder_id: string | null;
  decided_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

function toVariant(row: SuggestionRow): SuggestionVariant {
  if (row.label !== null && row.content !== null) {
    return { kind: 'chunk', label: row.label, content: row.content };
  }

  if (
    row.from_chunk_label !== null &&
    row.to_chunk_label !== null &&
    row.relationship_type !== null
  ) {
    return {
      kind: 'edge',
      fromChunkLabel: row.from_chunk_label,
      toChunkLabel: row.to_chunk_label,
      relationshipType: row.relationship_type as Extract<
        SuggestionVariant,
        { kind: 'edge' }
      >['relationshipType'],
    };
  }

  throw new Error(`SuggestionRepository: row ${row.id} matches neither chunk nor edge shape`);
}

function toSuggestion(row: SuggestionRow): Suggestion {
  return new Suggestion({
    id: row.id,
    workspaceId: row.workspace_id,
    variant: toVariant(row),
    discipline: row.discipline as Suggestion['discipline'],
    status: row.status as Suggestion['status'],
    submittedByStakeholderId: row.submitted_by_stakeholder_id,
    submittedByActorKind: row.submitted_by_actor_kind as Suggestion['submittedByActorKind'],
    ...(row.decided_by_stakeholder_id === null
      ? {}
      : { decidedByStakeholderId: row.decided_by_stakeholder_id }),
    ...(row.decided_at === null ? {} : { decidedAt: row.decided_at }),
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  });
}

async function insertSuggestion(client: PoolClient, suggestion: Suggestion): Promise<Suggestion> {
  const variant = suggestion.variant;
  const result: QueryResult<SuggestionRow> = await client.query<SuggestionRow>(
    `INSERT INTO suggestions (
       id, workspace_id, label, content, from_chunk_label, to_chunk_label, relationship_type,
       discipline, status, submitted_by_stakeholder_id, submitted_by_actor_kind,
       decided_by_stakeholder_id, decided_at, created_at, updated_at
     ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
     RETURNING *`,
    [
      suggestion.id,
      suggestion.workspaceId,
      variant.kind === 'chunk' ? variant.label : null,
      variant.kind === 'chunk' ? variant.content : null,
      variant.kind === 'edge' ? variant.fromChunkLabel : null,
      variant.kind === 'edge' ? variant.toChunkLabel : null,
      variant.kind === 'edge' ? variant.relationshipType : null,
      suggestion.discipline,
      suggestion.status,
      suggestion.submittedByStakeholderId,
      suggestion.submittedByActorKind,
      suggestion.decidedByStakeholderId ?? null,
      suggestion.decidedAt ?? null,
      suggestion.createdAt,
      suggestion.updatedAt,
    ],
  );

  const row = result.rows[0];
  if (row === undefined) {
    throw new Error('SuggestionRepository.create: INSERT ... RETURNING * produced no row');
  }

  return toSuggestion(row);
}

async function insertInitialStateLog(client: PoolClient, suggestion: Suggestion): Promise<void> {
  await client.query(
    `INSERT INTO suggestion_state_logs (
       id, suggestion_id, old_status, new_status, updated_by_stakeholder_id, created_at
     ) VALUES (gen_random_uuid(), $1, NULL, $2, $3, $4)`,
    [suggestion.id, suggestion.status, suggestion.submittedByStakeholderId, suggestion.createdAt],
  );
}

/**
 * Result of an accept attempt (Meridian IDEA-27/IDEA-49/IDEA-82, G07 SG2). `not_found` and
 * `not_pending` are distinct outcomes so the service layer can map them to 404 vs. 409 without
 * an extra lookup.
 */
export type SuggestionAcceptResult =
  | { kind: 'accepted'; branch: Branch }
  | { kind: 'not_found' }
  | { kind: 'not_pending' };

/**
 * Result of a reject attempt (Meridian IDEA-27, G07 SG3). Mirrors `SuggestionAcceptResult`'s
 * `not_found`/`not_pending` outcomes so the service layer can map them to 404 vs. 409 the same
 * way; rejecting never creates a branch, so there is no analogous success payload beyond the
 * status itself.
 */
export type SuggestionRejectResult =
  | { kind: 'rejected' }
  | { kind: 'not_found' }
  | { kind: 'not_pending' };

/**
 * Postgres-backed repository for the Suggestion aggregate (Meridian IDEA-27/IDEA-28/IDEA-49).
 * G07 SG1 only ever creates 'pending' suggestions submitted by a delegated actor;
 * decidedByStakeholderId/decidedAt are always NULL on create. Every create persists exactly one
 * `suggestion_state_logs` row (old_status=NULL, new_status='pending') in the same transaction as
 * the suggestion insert, all-or-nothing.
 */
@Injectable()
export class SuggestionRepository {
  constructor(@Inject(PG_POOL) private readonly pool: Pool) {}

  /**
   * Persists a newly-constructed Suggestion plus its initial state-log row in one transaction,
   * and returns the persisted entity (round-tripped from the database row, not the in-memory
   * instance).
   */
  async create(suggestion: Suggestion): Promise<Suggestion> {
    const client = await this.pool.connect();
    let transactionOpen = false;

    try {
      await client.query('BEGIN');
      transactionOpen = true;

      const created = await insertSuggestion(client, suggestion);
      await insertInitialStateLog(client, created);

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
   * Looks up a suggestion by id, scoped to a workspace (Meridian IDEA-98/IDEA-100, G11 SG5). A
   * cross-workspace id is indistinguishable from "does not exist", mirroring
   * `ChunkRepository.findById`'s precedent.
   */
  async findById(id: string, workspaceId: string): Promise<Suggestion | undefined> {
    const result: QueryResult<SuggestionRow> = await this.pool.query<SuggestionRow>(
      'SELECT * FROM suggestions WHERE id = $1 AND workspace_id = $2',
      [id, workspaceId],
    );

    const row = result.rows[0];
    return row === undefined ? undefined : toSuggestion(row);
  }

  /**
   * Accepts a pending suggestion, atomically creating a linked draft branch (Meridian IDEA-27's
   * pending -> accepted transition, IDEA-49's suggestion -> branch link). One transaction:
   * SELECT suggestion FOR UPDATE, assert pending (else roll back and report `not_found`/
   * `not_pending`), INSERT the branch (discipline from the suggestion, origin_suggestion_id set,
   * created_by_stakeholder_id = the accepting human), UPDATE the suggestion to accepted with
   * decision attribution, INSERT one `suggestion_state_logs` row (pending -> accepted) — all in
   * the same transaction, all-or-nothing (e.g. a duplicate branch name rolls back the suggestion
   * update too).
   */
  async accept(
    suggestionId: string,
    branchName: string,
    actingStakeholderId: string,
    workspaceId: string,
  ): Promise<SuggestionAcceptResult> {
    const client = await this.pool.connect();
    let transactionOpen = false;

    try {
      await client.query('BEGIN');
      transactionOpen = true;

      const suggestionResult: QueryResult<SuggestionRow> = await client.query<SuggestionRow>(
        'SELECT * FROM suggestions WHERE id = $1 AND workspace_id = $2 FOR UPDATE',
        [suggestionId, workspaceId],
      );
      const suggestionRow = suggestionResult.rows[0];
      if (suggestionRow === undefined) {
        await client.query('ROLLBACK');
        transactionOpen = false;
        return { kind: 'not_found' };
      }
      if (suggestionRow.status !== 'pending') {
        await client.query('ROLLBACK');
        transactionOpen = false;
        return { kind: 'not_pending' };
      }

      const branchResult: QueryResult<BranchRow> = await client.query<BranchRow>(
        `WITH persisted_timestamps AS (
           SELECT clock_timestamp() AS persisted_at
         )
         INSERT INTO branches (
           id, workspace_id, name, discipline, status, diverged_at, origin_suggestion_id,
           created_by_stakeholder_id, created_at, updated_at
         )
         SELECT
           gen_random_uuid(), $1, $2, $3, 'draft', persisted_timestamps.persisted_at, $4, $5,
           persisted_timestamps.persisted_at,
           persisted_timestamps.persisted_at
         FROM persisted_timestamps
         RETURNING *`,
        [workspaceId, branchName, suggestionRow.discipline, suggestionId, actingStakeholderId],
      );
      const branchRow = branchResult.rows[0];
      if (branchRow === undefined) {
        throw new Error('SuggestionRepository.accept: INSERT branches RETURNING * produced no row');
      }

      await client.query(
        `UPDATE suggestions
            SET status = 'accepted', decided_by_stakeholder_id = $2, decided_at = now(), updated_at = now()
          WHERE id = $1`,
        [suggestionId, actingStakeholderId],
      );

      await client.query(
        `INSERT INTO suggestion_state_logs (
           id, suggestion_id, old_status, new_status, updated_by_stakeholder_id, created_at
         ) VALUES (gen_random_uuid(), $1, 'pending', 'accepted', $2, now())`,
        [suggestionId, actingStakeholderId],
      );

      await client.query('COMMIT');
      transactionOpen = false;
      return { kind: 'accepted', branch: toBranch(branchRow) };
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
   * Rejects a pending suggestion (Meridian IDEA-27's pending -> rejected transition, G07 SG3).
   * One transaction: SELECT suggestion FOR UPDATE, assert pending (else roll back and report
   * `not_found`/`not_pending`), UPDATE the suggestion to rejected with decision attribution,
   * INSERT one `suggestion_state_logs` row (pending -> rejected). Never creates a branch.
   */
  async reject(
    suggestionId: string,
    actingStakeholderId: string,
    workspaceId: string,
  ): Promise<SuggestionRejectResult> {
    const client = await this.pool.connect();
    let transactionOpen = false;

    try {
      await client.query('BEGIN');
      transactionOpen = true;

      const suggestionResult: QueryResult<SuggestionRow> = await client.query<SuggestionRow>(
        'SELECT * FROM suggestions WHERE id = $1 AND workspace_id = $2 FOR UPDATE',
        [suggestionId, workspaceId],
      );
      const suggestionRow = suggestionResult.rows[0];
      if (suggestionRow === undefined) {
        await client.query('ROLLBACK');
        transactionOpen = false;
        return { kind: 'not_found' };
      }
      if (suggestionRow.status !== 'pending') {
        await client.query('ROLLBACK');
        transactionOpen = false;
        return { kind: 'not_pending' };
      }

      await client.query(
        `UPDATE suggestions
            SET status = 'rejected', decided_by_stakeholder_id = $2, decided_at = now(), updated_at = now()
          WHERE id = $1`,
        [suggestionId, actingStakeholderId],
      );

      await client.query(
        `INSERT INTO suggestion_state_logs (
           id, suggestion_id, old_status, new_status, updated_by_stakeholder_id, created_at
         ) VALUES (gen_random_uuid(), $1, 'pending', 'rejected', $2, now())`,
        [suggestionId, actingStakeholderId],
      );

      await client.query('COMMIT');
      transactionOpen = false;
      return { kind: 'rejected' };
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
   * Lists suggestions ordered oldest-first by `created_at` (G07 SG3), optionally filtered to a
   * single status, and scoped to `workspaceId` (G11 SG5).
   */
  async findAll(status: string | undefined, workspaceId: string): Promise<Suggestion[]> {
    const result: QueryResult<SuggestionRow> =
      status === undefined
        ? await this.pool.query<SuggestionRow>(
            'SELECT * FROM suggestions WHERE workspace_id = $1 ORDER BY created_at ASC',
            [workspaceId],
          )
        : await this.pool.query<SuggestionRow>(
            'SELECT * FROM suggestions WHERE workspace_id = $1 AND status = $2 ORDER BY created_at ASC',
            [workspaceId, status],
          );

    return result.rows.map(toSuggestion);
  }
}
