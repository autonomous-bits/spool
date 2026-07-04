/**
 * Postgres-backed persistence adapter for the suggestion review queue
 * (story S04) and the minimal branch-origin registration it depends on.
 *
 * Sources of authority:
 * - Story S04: stakeholders must see pending, accepted, and rejected
 *   suggestions; a newly submitted suggestion always starts pending; an
 *   accept/reject decision must be attributed to an authenticated human
 *   stakeholder, never a client-supplied actor claim; an accepted
 *   suggestion's initiated branch must retain a durable link back to it.
 * - Technical spec §"Suggestion persistence" (`IDEA-49`, feature-01
 *   accept-suggestion contract): pending initial status; durable link from
 *   an accepted-suggestion's branch back to the suggestion.
 * - Technical spec §"Protected operation contracts": persisted suggestion
 *   records must retain enough provenance to attribute accept/reject
 *   decisions to a human stakeholder; the persistence layer must not accept
 *   a client-supplied actor claim as a substitute for authenticated
 *   provenance. Enforced here by only accepting the domain-computed
 *   `SuggestionAcceptedDecision`/`SuggestionRejectedDecision` value objects
 *   (constructible only via `acceptSuggestion`/`rejectSuggestion`, both of
 *   which call `assertHumanActor` internally) — no raw stakeholder ID
 *   parameter is accepted from a caller.
 * - Meridian `IDEA-49`/`IDEA-28` (verified via `meridian-get-chunk`/
 *   `meridian-get-neighbourhood` against workspace
 *   `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`): suggestions persist "supporting
 *   chunk and edge details" as JSONB `payload`; "Initiated branches track
 *   their source via an `origin_suggestion_id` foreign key" — the durable
 *   link is owned by the branch record, not the suggestion.
 * - Technical spec §"Tenant isolation": every method takes a `workspaceId`
 *   and every SQL predicate filters on it explicitly.
 * - nestjs-security skill: parameterized queries only, never string
 *   concatenation.
 */

import { Inject, Injectable } from '@nestjs/common';
import type { Pool, PoolClient } from 'pg';
import {
  linkSuggestionToFeedbackBranch,
  SuggestionLifecycleError,
  type FeedbackBranchOwnership,
  type SuggestionAcceptedDecision,
  type SuggestionFeedbackBranchLink,
  type SuggestionRejectedDecision,
} from '../domain/suggestion-lifecycle.js';
import type {
  BranchId,
  Discipline,
  StakeholderId,
  SuggestionId,
  SuggestionState,
  WorkspaceId,
} from '../domain/types/index.js';
import { PG_POOL } from './database-pool.provider.js';

export interface PersistedSuggestion {
  readonly workspaceId: WorkspaceId;
  readonly suggestionId: SuggestionId;
  readonly discipline: Discipline;
  /** Structured chunk/edge modification details (Meridian `IDEA-49`/`IDEA-28`). */
  readonly payload: unknown;
  readonly state: SuggestionState;
  readonly submittedAt: string;
  readonly decidedByStakeholderId?: StakeholderId;
  readonly decidedAt?: string;
}

export interface NewSuggestionInput {
  readonly workspaceId: WorkspaceId;
  readonly suggestionId: SuggestionId;
  readonly discipline: Discipline;
  readonly payload: unknown;
}

export interface PersistedBranchRegistration {
  readonly workspaceId: WorkspaceId;
  readonly branchId: BranchId;
  readonly discipline: Discipline;
  readonly originSuggestionId?: SuggestionId;
  readonly createdAt: string;
}

export interface AcceptedSuggestionResult {
  readonly suggestion: PersistedSuggestion;
  readonly link: SuggestionFeedbackBranchLink;
  readonly branch: PersistedBranchRegistration;
}

interface SuggestionRow {
  readonly workspace_id: string;
  readonly suggestion_id: string;
  readonly discipline: string;
  readonly payload: unknown;
  readonly state: string;
  readonly submitted_at: string | Date;
  readonly decided_by_stakeholder_id: string | null;
  readonly decided_at: string | Date | null;
}

interface BranchRow {
  readonly workspace_id: string;
  readonly branch_id: string;
  readonly discipline: string;
  readonly origin_suggestion_id: string | null;
  readonly created_at: string | Date;
}

const SUGGESTION_COLUMNS = `workspace_id, suggestion_id, discipline, payload, state,
       submitted_at, decided_by_stakeholder_id, decided_at`;

function toIsoString(value: string | Date): string {
  return value instanceof Date ? value.toISOString() : value;
}

function rowToPersistedSuggestion(row: SuggestionRow): PersistedSuggestion {
  const base: PersistedSuggestion = {
    workspaceId: row.workspace_id as WorkspaceId,
    suggestionId: row.suggestion_id as SuggestionId,
    discipline: row.discipline as Discipline,
    payload: row.payload,
    state: row.state as SuggestionState,
    submittedAt: toIsoString(row.submitted_at),
  };
  if (row.decided_by_stakeholder_id == null || row.decided_at == null) {
    return base;
  }
  return {
    ...base,
    decidedByStakeholderId: row.decided_by_stakeholder_id as StakeholderId,
    decidedAt: toIsoString(row.decided_at),
  };
}

function rowToBranchRegistration(row: BranchRow): PersistedBranchRegistration {
  const base: PersistedBranchRegistration = {
    workspaceId: row.workspace_id as WorkspaceId,
    branchId: row.branch_id as BranchId,
    discipline: row.discipline as Discipline,
    createdAt: toIsoString(row.created_at),
  };
  return row.origin_suggestion_id == null
    ? base
    : { ...base, originSuggestionId: row.origin_suggestion_id as SuggestionId };
}

@Injectable()
export class SuggestionRepository {
  constructor(@Inject(PG_POOL) private readonly pool: Pool) {}

  /**
   * Persists a newly submitted suggestion. Always starts `pending`
   * (technical spec §"Suggestion persistence"; AC3) — there is no
   * parameter through which a caller can create a suggestion in any other
   * state.
   */
  async createSuggestion(input: NewSuggestionInput): Promise<PersistedSuggestion> {
    const result = await this.pool.query<SuggestionRow>(
      `INSERT INTO suggestions (workspace_id, suggestion_id, discipline, payload, state)
       VALUES ($1, $2, $3, $4, 'pending')
       RETURNING ${SUGGESTION_COLUMNS}`,
      [input.workspaceId, input.suggestionId, input.discipline, JSON.stringify(input.payload)],
    );
    return rowToPersistedSuggestion(result.rows[0]!);
  }

  async getSuggestion(
    workspaceId: WorkspaceId,
    suggestionId: SuggestionId,
  ): Promise<PersistedSuggestion | undefined> {
    const result = await this.pool.query<SuggestionRow>(
      `SELECT ${SUGGESTION_COLUMNS} FROM suggestions
       WHERE workspace_id = $1 AND suggestion_id = $2`,
      [workspaceId, suggestionId],
    );
    const row = result.rows[0];
    return row ? rowToPersistedSuggestion(row) : undefined;
  }

  /**
   * Lists every suggestion in a workspace regardless of state — pending,
   * accepted, and rejected are all included (AC1).
   */
  async listSuggestions(workspaceId: WorkspaceId): Promise<PersistedSuggestion[]> {
    const result = await this.pool.query<SuggestionRow>(
      `SELECT ${SUGGESTION_COLUMNS} FROM suggestions
       WHERE workspace_id = $1
       ORDER BY submitted_at, suggestion_id`,
      [workspaceId],
    );
    return result.rows.map(rowToPersistedSuggestion);
  }

  /**
   * Traces a branch back to the suggestion that originated it (AC2), per
   * Meridian `IDEA-49`: "Initiated branches track their source via an
   * `origin_suggestion_id` foreign key." Returns `undefined` if the branch
   * is not registered, or was not initiated from a suggestion, in this
   * workspace.
   */
  async findOriginatingSuggestion(
    workspaceId: WorkspaceId,
    branchId: BranchId,
  ): Promise<PersistedSuggestion | undefined> {
    const result = await this.pool.query<SuggestionRow>(
      `SELECT s.workspace_id, s.suggestion_id, s.discipline, s.payload, s.state,
              s.submitted_at, s.decided_by_stakeholder_id, s.decided_at
       FROM branches b
       JOIN suggestions s
         ON s.workspace_id = b.workspace_id AND s.suggestion_id = b.origin_suggestion_id
       WHERE b.workspace_id = $1 AND b.branch_id = $2`,
      [workspaceId, branchId],
    );
    const row = result.rows[0];
    return row ? rowToPersistedSuggestion(row) : undefined;
  }

  /**
   * Rejects a pending suggestion (AC4: `decision.decidedByStakeholderId` can
   * only have been produced by `rejectSuggestion`, which requires a human
   * actor). Must not modify any other graph state (technical spec
   * §"Protected operation contracts" — "Reject suggestion ... must not
   * modify graph state").
   *
   * The `WHERE state = 'pending'` guard makes this safe under concurrency:
   * if another decision already committed, zero rows match and this throws
   * `SuggestionLifecycleError('invalid-state-transition')` rather than
   * silently overwriting the prior decision.
   */
  async rejectSuggestion(decision: SuggestionRejectedDecision): Promise<PersistedSuggestion> {
    const result = await this.pool.query<SuggestionRow>(
      `UPDATE suggestions
       SET state = 'rejected',
           decided_by_stakeholder_id = $3,
           decided_at = $4,
           updated_at = now()
       WHERE workspace_id = $1 AND suggestion_id = $2 AND state = 'pending'
       RETURNING ${SUGGESTION_COLUMNS}`,
      [decision.workspaceId, decision.suggestionId, decision.decidedByStakeholderId, decision.decidedAt],
    );
    const row = result.rows[0];
    if (!row) {
      throw new SuggestionLifecycleError(
        'invalid-state-transition',
        `suggestion '${decision.suggestionId}' in workspace '${decision.workspaceId}' is no longer pending (already decided, or does not exist); cannot reject`,
      );
    }
    return rowToPersistedSuggestion(row);
  }

  /**
   * Accepts a pending suggestion and durably registers the feedback branch
   * it initiates, atomically, in a single database transaction (AC2, AC4).
   *
   * `linkSuggestionToFeedbackBranch` (existing domain function) validates
   * the branch's workspace/discipline against the decision *before* any SQL
   * runs, so a mismatch throws the existing `tenant-boundary-violation`/
   * `discipline-boundary-violation` `SuggestionLifecycleError` without
   * touching the database.
   *
   * Inside the transaction: the suggestion's state only flips to `accepted`
   * if it is still `pending` (same concurrency guard as `rejectSuggestion`),
   * and the branch is registered with `origin_suggestion_id` set to the
   * decided suggestion. The branch's durable `author_stakeholder_id` (story
   * S09) defaults to `decision.decidedByStakeholderId` — the human
   * stakeholder who accepted the suggestion is the correct author of the
   * feedback branch it initiates; `NotificationRepository` later reads this
   * column, never a caller-supplied claim, to resolve who must be notified
   * of new feedback/verification activity on the branch. If either step
   * fails — including a `branchId` that
   * is already registered in this workspace — the whole transaction rolls
   * back, so a suggestion can never be observed as `accepted` without its
   * branch registration, or vice versa.
   */
  async acceptSuggestionAndRegisterBranch(
    decision: SuggestionAcceptedDecision,
    branchOwnership: FeedbackBranchOwnership,
  ): Promise<AcceptedSuggestionResult> {
    const link = linkSuggestionToFeedbackBranch(decision, branchOwnership);

    const client = await this.pool.connect();
    try {
      await client.query('BEGIN');

      const suggestionResult = await client.query<SuggestionRow>(
        `UPDATE suggestions
         SET state = 'accepted',
             decided_by_stakeholder_id = $3,
             decided_at = $4,
             updated_at = now()
         WHERE workspace_id = $1 AND suggestion_id = $2 AND state = 'pending'
         RETURNING ${SUGGESTION_COLUMNS}`,
        [decision.workspaceId, decision.suggestionId, decision.decidedByStakeholderId, decision.decidedAt],
      );
      const suggestionRow = suggestionResult.rows[0];
      if (!suggestionRow) {
        throw new SuggestionLifecycleError(
          'invalid-state-transition',
          `suggestion '${decision.suggestionId}' in workspace '${decision.workspaceId}' is no longer pending (already decided, or does not exist); cannot accept`,
        );
      }

      const branchResult = await client.query<BranchRow>(
        `INSERT INTO branches (workspace_id, branch_id, discipline, origin_suggestion_id, author_stakeholder_id)
         VALUES ($1, $2, $3, $4, $5)
         RETURNING workspace_id, branch_id, discipline, origin_suggestion_id, created_at`,
        [
          branchOwnership.workspaceId,
          branchOwnership.branchId,
          branchOwnership.discipline,
          decision.suggestionId,
          decision.decidedByStakeholderId,
        ],
      );

      await client.query('COMMIT');
      return {
        suggestion: rowToPersistedSuggestion(suggestionRow),
        link,
        branch: rowToBranchRegistration(branchResult.rows[0]!),
      };
    } catch (error) {
      await this.rollbackQuietly(client);
      throw error;
    } finally {
      client.release();
    }
  }

  private async rollbackQuietly(client: PoolClient): Promise<void> {
    try {
      await client.query('ROLLBACK');
    } catch {
      // Connection is already broken (e.g. the BEGIN itself failed); the
      // pool will discard it on release. Swallowing here preserves the
      // original error thrown from the try block above.
    }
  }
}
