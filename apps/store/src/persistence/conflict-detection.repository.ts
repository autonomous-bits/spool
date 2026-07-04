/**
 * Postgres-backed persistence adapter for pre-merge conflict detection
 * (story S06): a branch's divergence marker, the mainline changes that
 * happened since that marker, the subset of those changes that conflict
 * with the branch's own independent changes, and catch-up confirmation.
 *
 * Sources of authority:
 * - Story S06: a stakeholder can determine which mainline changes happened
 *   after a branch diverged (AC1); identify, before merging, whether the
 *   same idea, relationship, or artifact association was changed
 *   independently on both branch and mainline (AC2); and confirm that
 *   catch-up has moved the branch's comparison point forward (AC3).
 * - Technical spec §"Divergence tracking" (`IDEA-41`): every branch persists
 *   a `diverged_at` marker captured at creation; it is updated only after
 *   local overrides are confirmed to integrate the conflicting mainline
 *   change (the caller's responsibility — this adapter performs the
 *   advance the referenced query logic specifies, `SET diverged_at =
 *   NOW()`, and does not itself second-guess whether integration happened).
 * - Technical spec §"Conflict detection scope" (`IDEA-46`): inspects chunk
 *   content changes, edge relationship changes (type or status), and
 *   chunk-artifact association changes made independently on both branch
 *   and mainline since divergence.
 * - Technical spec §"Required domain error categories": conflict-detection
 *   failures map to an existing category (`not-found`, added to
 *   `BranchLifecycleError` — see that file — rather than inventing an ad
 *   hoc conflict error type).
 * - Technical spec §"Tenant isolation": every method takes a `workspaceId`
 *   and every SQL predicate filters on it explicitly.
 * - Meridian (verified live against workspace
 *   `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`): `IDEA-41`, `IDEA-46`, `IDEA-54`.
 * - nestjs-security skill: parameterized queries only, never string
 *   concatenation.
 *
 * Edge changes are grouped by their full identity (`source_label,
 * target_label, relationship_type` — matching `branch_edge_deltas`'
 * primary key and the feature-01 edge-lineage identity) rather than by
 * endpoint pair alone: two different relationship types between the same
 * two ideas are two independent relationships (technical spec §"Edge
 * lineage persistence" — at most one active edge of a given type may exist
 * between two chunks, but different types may coexist), so collapsing them
 * into one identity would report false conflicts between unrelated
 * relationships that merely share endpoints (found during S06 implementation
 * review). A relationship-type *replacement* still surfaces correctly under
 * this identity scheme: it closes out the old type's lineage with a new
 * `deactivated` row (technical spec §"Edge lineage persistence") and opens a
 * new lineage under the new type, so the old identity shows a status change
 * and the new identity shows its own creation — each independently
 * comparable against a branch's delta for that exact identity.
 * Chunk-artifact-association changes are grouped by `(chunk_label,
 * artifact_id)` for the analogous reason: one logical association lineage
 * change (e.g. one deactivation) must be reported once, not once per
 * underlying append-only row.
 *
 * `listMainlineChangesSinceDivergence` and `detectConflicts` each run their
 * constituent queries inside a single `REPEATABLE READ` transaction so all
 * three change-sets are read from one consistent snapshot.
 */

import { Inject, Injectable } from '@nestjs/common';
import { DatabaseError, type Pool, type PoolClient } from 'pg';
import {
  BranchLifecycleError,
  divergencePoint,
  type BranchId,
  type Discipline,
  type DivergencePoint,
  type WorkspaceId,
} from '../domain/branch-lifecycle.js';
import type { ArtifactId, IdeaLabel, RelationshipType, StakeholderId } from '../domain/types/index.js';
import { PG_POOL } from './database-pool.provider.js';

/** Postgres SQLSTATE for a unique-constraint violation. */
const UNIQUE_VIOLATION = '23505';

export interface ChunkChange {
  readonly ideaLabel: IdeaLabel;
  readonly changedAt: string;
}

export interface EdgeChange {
  readonly sourceLabel: IdeaLabel;
  readonly targetLabel: IdeaLabel;
  readonly relationshipType: RelationshipType;
  readonly changedAt: string;
}

export interface ArtifactAssociationChange {
  readonly chunkLabel: IdeaLabel;
  readonly artifactId: ArtifactId;
  readonly changedAt: string;
}

export interface MainlineChangesSinceDivergence {
  readonly workspaceId: WorkspaceId;
  readonly branchId: BranchId;
  readonly divergedAt: DivergencePoint;
  readonly chunkChanges: ChunkChange[];
  readonly edgeChanges: EdgeChange[];
  readonly artifactAssociationChanges: ArtifactAssociationChange[];
}

export interface ConflictReport {
  readonly workspaceId: WorkspaceId;
  readonly branchId: BranchId;
  readonly divergedAt: DivergencePoint;
  readonly chunkConflicts: ChunkChange[];
  readonly edgeConflicts: EdgeChange[];
  readonly artifactAssociationConflicts: ArtifactAssociationChange[];
}

interface ChunkChangeRow {
  readonly idea_label: string;
  readonly updated_at: string | Date;
}

interface EdgeChangeRow {
  readonly source_label: string;
  readonly target_label: string;
  readonly relationship_type: string;
  readonly changed_at: string | Date;
}

interface ArtifactAssociationChangeRow {
  readonly chunk_label: string;
  readonly artifact_id: string;
  readonly changed_at: string | Date;
}

function toIsoString(value: string | Date): string {
  return value instanceof Date ? value.toISOString() : value;
}

function isUniqueViolation(error: unknown): boolean {
  return error instanceof DatabaseError && error.code === UNIQUE_VIOLATION;
}

@Injectable()
export class ConflictDetectionRepository {
  constructor(@Inject(PG_POOL) private readonly pool: Pool) {}

  /**
   * Registers a plain branch (no suggestion origin), capturing its
   * divergence marker at creation (technical spec §"Divergence tracking",
   * `IDEA-41`). `SuggestionRepository.acceptSuggestionAndRegisterBranch`
   * remains the entry point for suggestion-originated branches; this is a
   * minimal, direct alternative for branches that do not originate from a
   * suggestion.
   *
   * `authorStakeholderId` is optional (story S09 additive migration —
   * pre-existing callers are unaffected) but, when supplied, is the durable
   * record `NotificationRepository` reads to resolve "the author of the
   * evaluated branch" for notification routing (technical spec §"Feedback
   * notification routing", `IDEA-67`) — never a value trusted from a
   * notification-time caller claim.
   *
   * Throws `BranchLifecycleError('invalid-state-transition')` if a branch
   * with this identity is already registered in this workspace.
   */
  async registerBranch(
    workspaceId: WorkspaceId,
    branchId: BranchId,
    discipline: Discipline,
    authorStakeholderId?: StakeholderId,
  ): Promise<DivergencePoint> {
    try {
      const result = await this.pool.query<{ diverged_at: string | Date }>(
        `INSERT INTO branches (workspace_id, branch_id, discipline, author_stakeholder_id)
         VALUES ($1, $2, $3, $4)
         RETURNING diverged_at`,
        [workspaceId, branchId, discipline, authorStakeholderId ?? null],
      );
      return divergencePoint(toIsoString(result.rows[0]!.diverged_at));
    } catch (error) {
      if (isUniqueViolation(error)) {
        throw new BranchLifecycleError(
          'invalid-state-transition',
          `branch '${branchId}' is already registered in workspace '${workspaceId}'`,
        );
      }
      throw error;
    }
  }

  /**
   * Reads a branch's current divergence marker.
   *
   * Throws `BranchLifecycleError('not-found')` if no branch is registered
   * for this workspace/branch identity.
   */
  async getDivergedAt(workspaceId: WorkspaceId, branchId: BranchId): Promise<DivergencePoint> {
    const result = await this.pool.query<{ diverged_at: string | Date }>(
      `SELECT diverged_at FROM branches WHERE workspace_id = $1 AND branch_id = $2`,
      [workspaceId, branchId],
    );
    const row = result.rows[0];
    if (!row) {
      throw new BranchLifecycleError(
        'not-found',
        `no branch registered for workspace '${workspaceId}' and branch '${branchId}'`,
      );
    }
    return divergencePoint(toIsoString(row.diverged_at));
  }

  private async fetchDivergedAt(
    client: PoolClient,
    workspaceId: WorkspaceId,
    branchId: BranchId,
  ): Promise<DivergencePoint> {
    const result = await client.query<{ diverged_at: string | Date }>(
      `SELECT diverged_at FROM branches WHERE workspace_id = $1 AND branch_id = $2`,
      [workspaceId, branchId],
    );
    const row = result.rows[0];
    if (!row) {
      throw new BranchLifecycleError(
        'not-found',
        `no branch registered for workspace '${workspaceId}' and branch '${branchId}'`,
      );
    }
    return divergencePoint(toIsoString(row.diverged_at));
  }

  /**
   * Reports every mainline chunk, edge, and chunk-artifact-association
   * change made after the branch's divergence point (AC1) — regardless of
   * whether the branch itself touched the same identity. See
   * `detectConflicts` for the narrower, branch-intersected conflict set
   * (AC2).
   *
   * Throws `BranchLifecycleError('not-found')` if the branch is not
   * registered.
   */
  async listMainlineChangesSinceDivergence(
    workspaceId: WorkspaceId,
    branchId: BranchId,
  ): Promise<MainlineChangesSinceDivergence> {
    const client = await this.pool.connect();
    try {
      await client.query('BEGIN ISOLATION LEVEL REPEATABLE READ');
      const divergedAt = await this.fetchDivergedAt(client, workspaceId, branchId);

      const chunkResult = await client.query<ChunkChangeRow>(
        `SELECT idea_label, updated_at
         FROM chunks
         WHERE workspace_id = $1 AND updated_at > $2
         ORDER BY idea_label`,
        [workspaceId, divergedAt],
      );

      const edgeResult = await client.query<EdgeChangeRow>(
        `SELECT source_label, target_label, relationship_type, MAX(created_at) AS changed_at
         FROM edge_versions
         WHERE workspace_id = $1 AND created_at > $2
         GROUP BY source_label, target_label, relationship_type
         ORDER BY source_label, target_label, relationship_type`,
        [workspaceId, divergedAt],
      );

      const artifactResult = await client.query<ArtifactAssociationChangeRow>(
        `SELECT chunk_label, artifact_id, MAX(updated_at) AS changed_at
         FROM chunk_artifacts
         WHERE workspace_id = $1 AND branch_id IS NULL AND updated_at > $2
         GROUP BY chunk_label, artifact_id
         ORDER BY chunk_label, artifact_id`,
        [workspaceId, divergedAt],
      );

      await client.query('COMMIT');

      return {
        workspaceId,
        branchId,
        divergedAt,
        chunkChanges: chunkResult.rows.map((row) => ({
          ideaLabel: row.idea_label as IdeaLabel,
          changedAt: toIsoString(row.updated_at),
        })),
        edgeChanges: edgeResult.rows.map((row) => ({
          sourceLabel: row.source_label as IdeaLabel,
          targetLabel: row.target_label as IdeaLabel,
          relationshipType: row.relationship_type as RelationshipType,
          changedAt: toIsoString(row.changed_at),
        })),
        artifactAssociationChanges: artifactResult.rows.map((row) => ({
          chunkLabel: row.chunk_label as IdeaLabel,
          artifactId: row.artifact_id as ArtifactId,
          changedAt: toIsoString(row.changed_at),
        })),
      };
    } catch (error) {
      await this.rollbackQuietly(client);
      throw error;
    } finally {
      client.release();
    }
  }

  /**
   * Reports the subset of mainline changes since divergence that conflict
   * with this branch's own independent changes (AC2) — chunk content, edge
   * relationship (type or status), and chunk-artifact association changes
   * made on *both* sides since the branch diverged (technical spec
   * §"Conflict detection scope", `IDEA-46`).
   *
   * Throws `BranchLifecycleError('not-found')` if the branch is not
   * registered.
   */
  async detectConflicts(workspaceId: WorkspaceId, branchId: BranchId): Promise<ConflictReport> {
    const client = await this.pool.connect();
    try {
      await client.query('BEGIN ISOLATION LEVEL REPEATABLE READ');
      const divergedAt = await this.fetchDivergedAt(client, workspaceId, branchId);

      const chunkResult = await client.query<ChunkChangeRow>(
        `SELECT c.idea_label, c.updated_at
         FROM chunks c
         WHERE c.workspace_id = $1 AND c.updated_at > $2
           AND EXISTS (
             SELECT 1 FROM branch_chunk_deltas d
             WHERE d.workspace_id = $1 AND d.branch_id = $3 AND d.idea_label = c.idea_label
           )
         ORDER BY c.idea_label`,
        [workspaceId, divergedAt, branchId],
      );

      const edgeResult = await client.query<EdgeChangeRow>(
        `SELECT ev.source_label, ev.target_label, ev.relationship_type, MAX(ev.created_at) AS changed_at
         FROM edge_versions ev
         WHERE ev.workspace_id = $1 AND ev.created_at > $2
           AND EXISTS (
             SELECT 1 FROM branch_edge_deltas d
             WHERE d.workspace_id = $1 AND d.branch_id = $3
               AND d.source_label = ev.source_label AND d.target_label = ev.target_label
               AND d.relationship_type = ev.relationship_type
           )
         GROUP BY ev.source_label, ev.target_label, ev.relationship_type
         ORDER BY ev.source_label, ev.target_label, ev.relationship_type`,
        [workspaceId, divergedAt, branchId],
      );

      const artifactResult = await client.query<ArtifactAssociationChangeRow>(
        `SELECT ca.chunk_label, ca.artifact_id, MAX(ca.updated_at) AS changed_at
         FROM chunk_artifacts ca
         WHERE ca.workspace_id = $1 AND ca.branch_id IS NULL AND ca.updated_at > $2
           AND EXISTS (
             SELECT 1 FROM chunk_artifacts b
             WHERE b.workspace_id = $1 AND b.branch_id = $3
               AND b.chunk_label = ca.chunk_label AND b.artifact_id = ca.artifact_id
           )
         GROUP BY ca.chunk_label, ca.artifact_id
         ORDER BY ca.chunk_label, ca.artifact_id`,
        [workspaceId, divergedAt, branchId],
      );

      await client.query('COMMIT');

      return {
        workspaceId,
        branchId,
        divergedAt,
        chunkConflicts: chunkResult.rows.map((row) => ({
          ideaLabel: row.idea_label as IdeaLabel,
          changedAt: toIsoString(row.updated_at),
        })),
        edgeConflicts: edgeResult.rows.map((row) => ({
          sourceLabel: row.source_label as IdeaLabel,
          targetLabel: row.target_label as IdeaLabel,
          relationshipType: row.relationship_type as RelationshipType,
          changedAt: toIsoString(row.changed_at),
        })),
        artifactAssociationConflicts: artifactResult.rows.map((row) => ({
          chunkLabel: row.chunk_label as IdeaLabel,
          artifactId: row.artifact_id as ArtifactId,
          changedAt: toIsoString(row.changed_at),
        })),
      };
    } catch (error) {
      await this.rollbackQuietly(client);
      throw error;
    } finally {
      client.release();
    }
  }

  /**
   * Confirms catch-up: advances the branch's divergence marker to now
   * (AC3), so future conflict checks compare against the new point.
   *
   * Technical spec §"Divergence tracking" / `IDEA-41`: the marker "must be
   * updated only after local overrides are confirmed to integrate the
   * conflicting mainline change" — that confirmation is the caller's
   * responsibility (this story's deliverable is a persistence-level
   * capability, not merge/rebase orchestration); this method performs
   * exactly the advance the referenced query logic specifies
   * (`SET diverged_at = NOW()`).
   *
   * Throws `BranchLifecycleError('not-found')` if the branch is not
   * registered.
   */
  async confirmCatchUp(workspaceId: WorkspaceId, branchId: BranchId): Promise<DivergencePoint> {
    const result = await this.pool.query<{ diverged_at: string | Date }>(
      `UPDATE branches
       SET diverged_at = now(), updated_at = now()
       WHERE workspace_id = $1 AND branch_id = $2
       RETURNING diverged_at`,
      [workspaceId, branchId],
    );
    const row = result.rows[0];
    if (!row) {
      throw new BranchLifecycleError(
        'not-found',
        `no branch registered for workspace '${workspaceId}' and branch '${branchId}'`,
      );
    }
    return divergencePoint(toIsoString(row.diverged_at));
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
