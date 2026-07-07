import type { Pool, PoolClient, QueryResult, QueryResultRow } from 'pg';
import { Inject, Injectable } from '@nestjs/common';
import { Workspace } from '../domain/workspace.js';
import {
  WorkspaceMembership,
  WorkspaceMembershipAlreadyExistsError,
  assertCanAddMember,
  deriveInitialMembership,
} from '../domain/workspace-membership.js';
import { PG_POOL } from './pg-pool.token.js';

interface WorkspaceRow extends QueryResultRow {
  id: string;
  name: string;
  created_by_stakeholder_id: string;
  created_at: Date;
  updated_at: Date;
}

interface WorkspaceMembershipRow extends QueryResultRow {
  workspace_id: string;
  stakeholder_id: string;
  created_at: Date;
}

function toWorkspace(row: WorkspaceRow): Workspace {
  return new Workspace({
    id: row.id,
    name: row.name,
    createdByStakeholderId: row.created_by_stakeholder_id,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  });
}

function toWorkspaceMembership(row: WorkspaceMembershipRow): WorkspaceMembership {
  return new WorkspaceMembership({
    workspaceId: row.workspace_id,
    stakeholderId: row.stakeholder_id,
    createdAt: row.created_at,
  });
}

async function insertMembership(
  client: PoolClient,
  membership: WorkspaceMembership,
): Promise<WorkspaceMembership> {
  const result: QueryResult<WorkspaceMembershipRow> = await client.query<WorkspaceMembershipRow>(
    `INSERT INTO workspace_memberships (workspace_id, stakeholder_id, created_at)
     VALUES ($1, $2, $3)
     RETURNING *`,
    [membership.workspaceId, membership.stakeholderId, membership.createdAt],
  );

  const row = result.rows[0];
  if (row === undefined) {
    throw new Error(
      'WorkspaceRepository: INSERT workspace_memberships ... RETURNING * produced no row',
    );
  }

  return toWorkspaceMembership(row);
}

/**
 * Result of an add-member attempt (Meridian IDEA-88/IDEA-95, G10 SG2). `not_found` and
 * `caller_not_member` are distinct outcomes so the service layer can map them to 404 vs. 403
 * without an extra lookup; `already_member` maps to 409 via SG1's
 * WorkspaceMembershipAlreadyExistsError, thrown rather than returned since it is an exceptional,
 * not-expected-in-normal-flow condition (mirroring SuggestionRepository's pattern of throwing for
 * FK-violation-shaped failures).
 */
export type WorkspaceAddMemberResult =
  | { kind: 'added'; membership: WorkspaceMembership }
  | { kind: 'workspace_not_found' }
  | { kind: 'caller_not_member' };

/**
 * Postgres-backed repository for the Workspace/WorkspaceMembership aggregate (Meridian IDEA-96's
 * ratified schema, IDEA-95's flat no-roles membership, IDEA-88's direct-add bootstrap). G10 only
 * ever creates a workspace with its creator as the sole initial member (SG1's
 * deriveInitialMembership) and adds members one at a time via SG1's assertCanAddMember. Never
 * touches workspace_id on any other table (Meridian IDEA-90 is explicitly deferred).
 */
@Injectable()
export class WorkspaceRepository {
  constructor(@Inject(PG_POOL) private readonly pool: Pool) {}

  /**
   * Persists a newly-constructed Workspace plus its creator's initial membership row (SG1's
   * deriveInitialMembership, persisted verbatim, never re-derived here) in one atomic
   * transaction, and returns the persisted workspace (round-tripped from the database row, not
   * the in-memory instance).
   */
  async createWithFirstMember(workspace: Workspace): Promise<Workspace> {
    const client = await this.pool.connect();
    let transactionOpen = false;

    try {
      await client.query('BEGIN');
      transactionOpen = true;

      const result: QueryResult<WorkspaceRow> = await client.query<WorkspaceRow>(
        `INSERT INTO workspaces (id, name, created_by_stakeholder_id, created_at, updated_at)
         VALUES ($1, $2, $3, $4, $5)
         RETURNING *`,
        [
          workspace.id,
          workspace.name,
          workspace.createdByStakeholderId,
          workspace.createdAt,
          workspace.updatedAt,
        ],
      );

      const row = result.rows[0];
      if (row === undefined) {
        throw new Error(
          'WorkspaceRepository.createWithFirstMember: INSERT workspaces ... RETURNING * produced no row',
        );
      }

      const created = toWorkspace(row);
      await insertMembership(client, deriveInitialMembership(created));

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
   * Looks up a workspace by id. Returns `undefined` as an explicit not-found result rather than
   * throwing, so callers can distinguish "not found" from an actual persistence error.
   */
  async findById(id: string): Promise<Workspace | undefined> {
    const result: QueryResult<WorkspaceRow> = await this.pool.query<WorkspaceRow>(
      'SELECT * FROM workspaces WHERE id = $1',
      [id],
    );

    const row = result.rows[0];
    return row === undefined ? undefined : toWorkspace(row);
  }

  /**
   * Reports whether a stakeholder is a member of a workspace. Used both by the API layer's
   * caller-membership check (SG3) and internally by addMember's own caller-membership check.
   */
  async isMember(workspaceId: string, stakeholderId: string): Promise<boolean> {
    const result = await this.pool.query(
      'SELECT 1 FROM workspace_memberships WHERE workspace_id = $1 AND stakeholder_id = $2',
      [workspaceId, stakeholderId],
    );

    return result.rows.length > 0;
  }

  /**
   * Reports whether a stakeholder currently belongs to any workspace at all. Used by G11 SG2's
   * OAuth callback bootstrap path (Meridian IDEA-101): a stakeholder with zero memberships may
   * mint a workspace-less bootstrap token; a stakeholder with at least one membership may not.
   */
  async hasAnyMembership(stakeholderId: string): Promise<boolean> {
    const result = await this.pool.query(
      'SELECT 1 FROM workspace_memberships WHERE stakeholder_id = $1 LIMIT 1',
      [stakeholderId],
    );

    return result.rows.length > 0;
  }

  /**
   * Adds a target stakeholder as a member of a workspace, on behalf of an acting stakeholder who
   * must already be a member (SG1's assertCanAddMember, Meridian IDEA-88's direct-add-only-by-
   * existing-member rule). One transaction: SELECT workspace FOR UPDATE (report
   * `workspace_not_found` if missing), check the caller's membership (report `caller_not_member`
   * if not a member — SG1's assertCanAddMember enforces this invariant), then INSERT the new
   * membership row. A duplicate-add (target already a member) violates the composite primary key
   * and is surfaced as SG1's WorkspaceMembershipAlreadyExistsError, distinct from the
   * caller-not-member rejection.
   */
  async addMember(
    workspaceId: string,
    actingStakeholderId: string,
    targetStakeholderId: string,
  ): Promise<WorkspaceAddMemberResult> {
    const client = await this.pool.connect();
    let transactionOpen = false;

    try {
      await client.query('BEGIN');
      transactionOpen = true;

      const workspaceResult: QueryResult<WorkspaceRow> = await client.query<WorkspaceRow>(
        'SELECT * FROM workspaces WHERE id = $1 FOR UPDATE',
        [workspaceId],
      );
      if (workspaceResult.rows[0] === undefined) {
        await client.query('ROLLBACK');
        transactionOpen = false;
        return { kind: 'workspace_not_found' };
      }

      const membershipResult = await client.query(
        'SELECT 1 FROM workspace_memberships WHERE workspace_id = $1 AND stakeholder_id = $2',
        [workspaceId, actingStakeholderId],
      );
      const actorIsMember = membershipResult.rows.length > 0;

      try {
        assertCanAddMember(actorIsMember);
      } catch {
        await client.query('ROLLBACK');
        transactionOpen = false;
        return { kind: 'caller_not_member' };
      }

      const duplicateResult = await client.query(
        'SELECT 1 FROM workspace_memberships WHERE workspace_id = $1 AND stakeholder_id = $2',
        [workspaceId, targetStakeholderId],
      );
      if (duplicateResult.rows.length > 0) {
        await client.query('ROLLBACK');
        transactionOpen = false;
        throw new WorkspaceMembershipAlreadyExistsError(workspaceId, targetStakeholderId);
      }

      const membership = await insertMembership(
        client,
        new WorkspaceMembership({ workspaceId, stakeholderId: targetStakeholderId }),
      );

      await client.query('COMMIT');
      transactionOpen = false;
      return { kind: 'added', membership };
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
