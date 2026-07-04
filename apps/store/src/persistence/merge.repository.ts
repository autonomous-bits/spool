/**
 * Postgres-backed persistence adapter for branch-merge execution (story
 * S07): persisting the feature-01 branch lifecycle status for the first
 * time, and executing the atomic, all-or-nothing promotion of a verified
 * branch's chunk, edge, and chunk-artifact-association changes into
 * mainline, with enough provenance left behind to trace them back to the
 * branch afterward.
 *
 * Sources of authority:
 * - Story S07: a failed merge leaves no partial changes to chunks, edges,
 *   artifact associations, or branch status (AC1); a successful merge
 *   applies all of its changes together, none observable in isolation
 *   before the others (AC2); merged records remain traceable back to the
 *   branch and review process that produced them (AC3).
 * - Technical spec §"Atomic merge" (`IDEA-47`): one transaction covering
 *   chunk updates, edge updates, chunk-artifact association updates, and
 *   branch-status updates; any failure rolls back the whole operation.
 * - Technical spec §"Pre-merge history reconstruction" (`IDEA-69`): chunk,
 *   edge, and chunk-artifact records remain traceable to their originating
 *   branch via `origin_branch_id`.
 * - Technical spec §"Required domain error categories": no new categories;
 *   this repository only ever throws `BranchLifecycleError`,
 *   `EdgeLineageError`, or `ArtifactAssociationError` (all pre-existing).
 * - Technical spec §"Tenant isolation": every method takes a `workspaceId`
 *   and every SQL predicate filters on it explicitly.
 * - store AGENTS.md: persistence must reuse feature-01 domain constructors
 *   (`submitBranch`/`verifyBranch`/`mergeBranch` state guards,
 *   `resolveChunkDelta`/`resolveEdgeDelta`, `createAssociation`/
 *   `deactivateAssociation`) rather than redefining lifecycle rules here.
 * - Meridian (verified live against workspace
 *   `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`): `IDEA-47`, `IDEA-69`, `IDEA-23`
 *   (merged branches are archived, never physically deleted), `IDEA-29`
 *   (audit columns track the accountable human stakeholder).
 * - nestjs-security skill: parameterized queries only, never string
 *   concatenation.
 *
 * Deliberately NOT this class's job:
 * - Write-lock enforcement on branch-scoped writes for non-draft branches is
 *   enforced by `BranchGraphRepository.saveChunkDelta`/`saveEdgeDelta`
 *   (story S11), which reuse the same `domain/branch-lifecycle.ts` guards
 *   this file uses for its own state transitions. (This supersedes an
 *   earlier version of this comment, written during story S07, that
 *   attributed write-lock enforcement solely to a future NestJS API
 *   gateway per Meridian `IDEA-35` — see `apps/store/AGENTS.md`.)
 * - Chunk lifecycle promotion (`draft -> approved -> promoted`) is a
 *   separate protected "approve chunk" operation (feature-01 technical
 *   spec §"Protected operation contracts"); merge preserves whatever
 *   `lifecycleState` the branch's chunk delta already carried.
 * - Pre-merge conflict detection is story S06's `ConflictDetectionRepository`
 *   and is deliberately NOT invoked by this class — `MergeRepository` is a
 *   low-level, unconditional promotion primitive. The canonical, supported
 *   way to merge a branch is `ConflictGatedMergeService.mergeBranch`
 *   (`./conflict-gated-merge.service.ts`), which always runs conflict
 *   detection first and refuses to call this class's `mergeBranch` if any
 *   conflict is reported (found missing during rubber-duck review of
 *   Feature 01/02 against Meridian — the feature-01 "merge branch"
 *   protected-operation contract requires conflict checks, and nothing
 *   previously enforced that on any real path). This class still takes
 *   `FOR UPDATE` row locks on every mainline record it touches purely for
 *   ordinary transactional read-then-write safety against concurrent direct
 *   mainline writes, which is a different concern from pre-merge conflict
 *   semantics.
 */

import { Inject, Injectable } from '@nestjs/common';
import { createHash } from 'node:crypto';
import { DatabaseError, type Pool, type PoolClient } from 'pg';
import {
  BranchLifecycleError,
  mergeBranch as mergeBranchTransition,
  submitBranch as submitBranchTransition,
  verifyBranch as verifyBranchTransition,
  type ActorContext,
  type BranchId,
  type Discipline,
  type HumanActorContext,
  type StakeholderId,
  type WorkspaceId,
} from '../domain/branch-lifecycle.js';
import { chunkLifecycleStatus } from '../domain/chunk-lifecycle.js';
import { currentEdgeVersion } from '../domain/edge-lineage.js';
import {
  createAssociation,
  deactivateAssociation,
  currentAssociationVersion,
  ArtifactAssociationError,
  type ArtifactAssociationState,
} from '../domain/artifact-association-lineage.js';
import type { ArtifactId, IdeaLabel, RelationshipType } from '../domain/types/index.js';
import { PG_POOL } from './database-pool.provider.js';
import {
  ChunkGraphRepository,
  mapPersistenceError,
  type PersistedChunk,
} from './chunk-graph.repository.js';
import { resolveChunkDelta, resolveEdgeDelta } from './branch-graph.repository.js';

export type BranchLifecycleStatus = 'draft' | 'submitted' | 'verified' | 'merged';

const UNIQUE_VIOLATION = '23505';

/**
 * One permanent, merge-provenance history row for a chunk (technical spec
 * §"Pre-merge history reconstruction", `IDEA-69`; see `schema.ts`'s
 * `chunk_history` table doc comment for why this exists alongside the
 * mutable `chunks` row).
 */
export interface ChunkHistoryEntry {
  readonly workspaceId: WorkspaceId;
  readonly ideaLabel: IdeaLabel;
  readonly originBranchId: BranchId;
  readonly chunkType: PersistedChunk['chunkType'];
  readonly discipline: PersistedChunk['discipline'];
  readonly contextKind: PersistedChunk['contextKind'];
  readonly content: string;
  readonly lifecycleState: PersistedChunk['status']['lifecycleState'];
  readonly activityState: PersistedChunk['status']['activityState'];
  readonly mergedAt: string;
}

interface ChunkHistoryRow {
  readonly workspace_id: string;
  readonly idea_label: string;
  readonly origin_branch_id: string;
  readonly chunk_type: string;
  readonly discipline: string;
  readonly context_kind: string;
  readonly content: string;
  readonly lifecycle_state: string;
  readonly activity_state: string;
  readonly merged_at: string | Date;
}

function rowToChunkHistoryEntry(row: ChunkHistoryRow): ChunkHistoryEntry {
  return {
    workspaceId: row.workspace_id as WorkspaceId,
    ideaLabel: row.idea_label as IdeaLabel,
    originBranchId: row.origin_branch_id as BranchId,
    chunkType: row.chunk_type as PersistedChunk['chunkType'],
    discipline: row.discipline as PersistedChunk['discipline'],
    contextKind: row.context_kind as PersistedChunk['contextKind'],
    content: row.content,
    lifecycleState: row.lifecycle_state as PersistedChunk['status']['lifecycleState'],
    activityState: row.activity_state as PersistedChunk['status']['activityState'],
    mergedAt: row.merged_at instanceof Date ? row.merged_at.toISOString() : row.merged_at,
  };
}

export interface MergeOutcome {
  readonly workspaceId: WorkspaceId;
  readonly branchId: BranchId;
  /**
   * The merged branch's discipline (already loaded via the existing branch
   * row lock; no additional query). Added for story S08 so a caller can
   * route post-commit downstream delivery dispatch
   * (`MergeDeliveryDispatcher.dispatchMergeCompleted`) without this
   * repository having to know anything about delivery itself.
   */
  readonly discipline: Discipline;
  readonly mergedAt: string;
  readonly mergedByStakeholderId: StakeholderId;
  readonly mergedChunkLabels: readonly IdeaLabel[];
  readonly mergedEdgeIdentities: readonly {
    readonly sourceLabel: IdeaLabel;
    readonly targetLabel: IdeaLabel;
    readonly relationshipType: RelationshipType;
  }[];
  readonly mergedArtifactAssociations: readonly {
    readonly chunkLabel: IdeaLabel;
    readonly artifactId: ArtifactId;
  }[];
}

interface BranchRow {
  readonly workspace_id: string;
  readonly branch_id: string;
  readonly discipline: string;
  readonly status: string;
}

interface BranchChunkDeltaRow {
  readonly idea_label: string;
  readonly delta_kind: string;
  readonly chunk_type: string | null;
  readonly discipline: string | null;
  readonly context_kind: string | null;
  readonly content: string | null;
  readonly lifecycle_state: string | null;
  readonly activity_state: string | null;
}

interface BranchEdgeDeltaRow {
  readonly source_label: string;
  readonly target_label: string;
  readonly relationship_type: string;
  readonly delta_kind: string;
}

interface ChunkArtifactRow {
  readonly workspace_id: string;
  readonly chunk_label: string;
  readonly artifact_id: string;
  readonly branch_id: string | null;
  readonly origin_branch_id: string | null;
  readonly version: number;
  readonly status: string;
}

const CHUNK_ARTIFACT_ROW_COLUMNS = `workspace_id, chunk_label, artifact_id, branch_id,
       origin_branch_id, version, status`;

function isUniqueViolation(error: unknown): boolean {
  return error instanceof DatabaseError && error.code === UNIQUE_VIOLATION;
}

/**
 * Deterministic 63-bit advisory-lock key for a chunk-artifact identity,
 * mirroring `ArtifactAssociationRepository`'s derivation so the merge
 * transaction serializes against any concurrent direct association write
 * for the same identity+scope.
 */
function artifactIdentityLockKey(
  workspaceId: WorkspaceId,
  chunkLabel: IdeaLabel,
  artifactId: ArtifactId,
  branchId: BranchId | undefined,
): bigint {
  const raw = `${workspaceId}\u0000${chunkLabel}\u0000${artifactId}\u0000${branchId ?? ''}`;
  const digest = createHash('sha256').update(raw).digest();
  const unsigned = digest.readBigUInt64BE(0);
  // eslint-disable-next-line no-bitwise
  return unsigned & 0x7fffffffffffffffn;
}

/**
 * Decides how a branch's chunk-artifact shadow lineage's terminal state
 * promotes onto mainline's current lineage state, given the branch's
 * association actually exists (merge only visits identities the branch has
 * a row for). Pure, so the append-vs-seed-vs-noop matrix is unit-testable
 * without a database — mirrors `resolveEdgeDelta`'s equivalent cases for
 * edges (technical spec §"Chunk-artifact association lifecycle").
 *
 * Throws `ArtifactAssociationError('invalid-state-transition')` if the
 * branch asserts an active association over a mainline lineage that is
 * already inactive: this domain has no reactivation transition (same
 * precedent as `ArtifactAssociationRepository.createAssociation` and
 * `resolveEdgeDelta`).
 */
export function resolveArtifactAssociationPromotion(
  mainlineState: ArtifactAssociationState | undefined,
  branchState: ArtifactAssociationState,
  identity: { readonly chunkLabel: IdeaLabel; readonly artifactId: ArtifactId },
):
  | { readonly action: 'noop' }
  | { readonly action: 'seed'; readonly states: readonly ['active', 'deactivated'] }
  | { readonly action: 'append'; readonly state: ArtifactAssociationState } {
  if (branchState === 'active') {
    if (mainlineState === undefined) {
      return { action: 'append', state: 'active' };
    }
    if (mainlineState === 'active') {
      // Mainline already reflects this exact state — mirrors
      // `resolveEdgeDelta`'s "already active: idempotent no-op" case for
      // an 'upsert' delta over an active mainline edge. Attempting an
      // insert here would needlessly collide with
      // `idx_chunk_artifacts_mainline` even though nothing has actually
      // changed.
      return { action: 'noop' };
    }
    throw new ArtifactAssociationError(
      'invalid-state-transition',
      `cannot promote branch's active chunk-artifact association for chunk '${identity.chunkLabel}' and artifact '${identity.artifactId}' because the mainline association is already '${mainlineState}'; reactivating an inactive mainline association is out of scope`,
    );
  }
  // branchState is 'deactivated' (the only reachable inactive terminal state).
  if (mainlineState === undefined) {
    return { action: 'seed', states: ['active', 'deactivated'] };
  }
  if (mainlineState === 'active') {
    return { action: 'append', state: 'deactivated' };
  }
  return { action: 'noop' };
}

@Injectable()
export class MergeRepository {
  constructor(
    @Inject(PG_POOL) private readonly pool: Pool,
    private readonly chunkGraphRepository: ChunkGraphRepository,
  ) {}

  async getBranchStatus(
    workspaceId: WorkspaceId,
    branchId: BranchId,
  ): Promise<BranchLifecycleStatus> {
    const result = await this.pool.query<{ status: string }>(
      `SELECT status FROM branches WHERE workspace_id = $1 AND branch_id = $2`,
      [workspaceId, branchId],
    );
    const row = result.rows[0];
    if (!row) {
      throw new BranchLifecycleError(
        'not-found',
        `no branch registered for workspace '${workspaceId}' and branch '${branchId}'`,
      );
    }
    return row.status as BranchLifecycleStatus;
  }

  /**
   * Transitions a draft branch to submitted (feature-01 `submitBranch`
   * guard), persisting the status and submission provenance for the first
   * time (no prior story tracked branch status at all).
   */
  async submitBranch(
    workspaceId: WorkspaceId,
    branchId: BranchId,
    actor: ActorContext,
    actorDiscipline: Discipline,
  ): Promise<BranchLifecycleStatus> {
    const client = await this.pool.connect();
    try {
      await client.query('BEGIN');
      const branch = await this.lockBranchRow(client, workspaceId, branchId);
      const nextStatus = submitBranchTransition(
        branch.status as BranchLifecycleStatus,
        actor,
        actorDiscipline,
        branch.discipline as Discipline,
      );
      await client.query(
        `UPDATE branches
         SET status = $1, submitted_at = now(), submitted_by_stakeholder_id = $2, updated_at = now()
         WHERE workspace_id = $3 AND branch_id = $4`,
        [nextStatus, actor.stakeholderId, workspaceId, branchId],
      );
      await client.query('COMMIT');
      return nextStatus;
    } catch (error) {
      await this.rollbackQuietly(client);
      throw error;
    } finally {
      client.release();
    }
  }

  /**
   * Transitions a submitted branch to verified (feature-01 `verifyBranch`
   * guard, human-initiated only).
   */
  async verifyBranch(
    workspaceId: WorkspaceId,
    branchId: BranchId,
    actor: HumanActorContext,
  ): Promise<BranchLifecycleStatus> {
    const client = await this.pool.connect();
    try {
      await client.query('BEGIN');
      const branch = await this.lockBranchRow(client, workspaceId, branchId);
      const nextStatus = verifyBranchTransition(branch.status as BranchLifecycleStatus, actor);
      await client.query(
        `UPDATE branches
         SET status = $1, verified_at = now(), verified_by_stakeholder_id = $2, updated_at = now()
         WHERE workspace_id = $3 AND branch_id = $4`,
        [nextStatus, actor.stakeholderId, workspaceId, branchId],
      );
      await client.query('COMMIT');
      return nextStatus;
    } catch (error) {
      await this.rollbackQuietly(client);
      throw error;
    } finally {
      client.release();
    }
  }

  /**
   * Executes a verified branch's merge into mainline as a single atomic
   * transaction (AC1, AC2): every chunk, edge, and chunk-artifact-
   * association change the branch made is promoted, and the branch's
   * status flips to `merged`, all in one Postgres transaction. Any error at
   * any step rolls back the entire operation — nothing is left partially
   * applied (AC1). Every promoted record is stamped with `origin_branch_id
   * = branchId` so it remains traceable afterward (AC3).
   *
   * Low-level primitive: this method performs no pre-merge conflict
   * detection. Production callers (and `MergeDeliveryOrchestrator`) must go
   * through `ConflictGatedMergeService.mergeBranch` instead, which enforces
   * the feature-01 "merge branch" protected-operation contract's conflict
   * checks before ever calling this method.
   *
   * Throws `BranchLifecycleError('not-found')` if the branch is not
   * registered. Throws `BranchLifecycleError('unauthorized-actor')` if
   * `actor` is not a direct human stakeholder. Throws
   * `BranchLifecycleError('invalid-state-transition')` if the branch is not
   * currently `verified`. Throws `EdgeLineageError`/`ArtifactAssociationError`
   * if any individual promoted change is itself invalid (e.g. the branch
   * asserts an edge/association change that conflicts with mainline's
   * current state) — any such failure rolls back every other change this
   * merge attempted, including ones that would otherwise have succeeded.
   */
  async mergeBranch(
    workspaceId: WorkspaceId,
    branchId: BranchId,
    actor: HumanActorContext,
  ): Promise<MergeOutcome> {
    const client = await this.pool.connect();
    try {
      await client.query('BEGIN');

      const branch = await this.lockBranchRow(client, workspaceId, branchId);
      mergeBranchTransition(branch.status as BranchLifecycleStatus, actor);

      const mergedChunkLabels: IdeaLabel[] = [];
      const chunkDeltaRows = await client.query<BranchChunkDeltaRow>(
        `SELECT idea_label, delta_kind, chunk_type, discipline, context_kind,
                content, lifecycle_state, activity_state
         FROM branch_chunk_deltas
         WHERE workspace_id = $1 AND branch_id = $2
         ORDER BY idea_label`,
        [workspaceId, branchId],
      );
      for (const deltaRow of chunkDeltaRows.rows) {
        await this.promoteChunkDelta(client, workspaceId, branchId, deltaRow);
        mergedChunkLabels.push(deltaRow.idea_label as IdeaLabel);
      }

      const mergedEdgeIdentities: {
        sourceLabel: IdeaLabel;
        targetLabel: IdeaLabel;
        relationshipType: RelationshipType;
      }[] = [];
      const edgeDeltaRows = await client.query<BranchEdgeDeltaRow>(
        `SELECT source_label, target_label, relationship_type, delta_kind
         FROM branch_edge_deltas
         WHERE workspace_id = $1 AND branch_id = $2
         ORDER BY source_label, target_label, relationship_type`,
        [workspaceId, branchId],
      );
      for (const deltaRow of edgeDeltaRows.rows) {
        await this.promoteEdgeDelta(client, workspaceId, branchId, deltaRow);
        mergedEdgeIdentities.push({
          sourceLabel: deltaRow.source_label as IdeaLabel,
          targetLabel: deltaRow.target_label as IdeaLabel,
          relationshipType: deltaRow.relationship_type as RelationshipType,
        });
      }

      const mergedArtifactAssociations: { chunkLabel: IdeaLabel; artifactId: ArtifactId }[] = [];
      const branchAssociationRows = await client.query<{ chunk_label: string; artifact_id: string }>(
        `SELECT DISTINCT chunk_label, artifact_id
         FROM chunk_artifacts
         WHERE workspace_id = $1 AND branch_id = $2
         ORDER BY chunk_label, artifact_id`,
        [workspaceId, branchId],
      );
      for (const identityRow of branchAssociationRows.rows) {
        const chunkLabel = identityRow.chunk_label as IdeaLabel;
        const artifactId = identityRow.artifact_id as ArtifactId;
        await this.promoteArtifactAssociation(client, workspaceId, branchId, chunkLabel, artifactId);
        mergedArtifactAssociations.push({ chunkLabel, artifactId });
      }

      const mergedAtResult = await client.query<{ merged_at: string | Date }>(
        `UPDATE branches
         SET status = 'merged', merged_at = now(), merged_by_stakeholder_id = $1, updated_at = now()
         WHERE workspace_id = $2 AND branch_id = $3
         RETURNING merged_at`,
        [actor.stakeholderId, workspaceId, branchId],
      );
      const mergedAtRaw = mergedAtResult.rows[0]!.merged_at;
      const mergedAt = mergedAtRaw instanceof Date ? mergedAtRaw.toISOString() : mergedAtRaw;

      await client.query('COMMIT');
      return {
        workspaceId,
        branchId,
        discipline: branch.discipline as Discipline,
        mergedAt,
        mergedByStakeholderId: actor.stakeholderId,
        mergedChunkLabels,
        mergedEdgeIdentities,
        mergedArtifactAssociations,
      };
    } catch (error) {
      await this.rollbackQuietly(client);
      throw mapPersistenceError(error);
    } finally {
      client.release();
    }
  }

  async listChunksByOriginBranch(
    workspaceId: WorkspaceId,
    branchId: BranchId,
  ): Promise<PersistedChunk[]> {
    return this.chunkGraphRepository.listChunksByOriginBranch(workspaceId, branchId);
  }

  async listEdgeIdentitiesByOriginBranch(
    workspaceId: WorkspaceId,
    branchId: BranchId,
  ): Promise<
    { sourceLabel: IdeaLabel; targetLabel: IdeaLabel; relationshipType: RelationshipType }[]
  > {
    return this.chunkGraphRepository.listEdgeIdentitiesByOriginBranch(workspaceId, branchId);
  }

  /**
   * Lists every mainline chunk-artifact association promoted by a specific
   * branch's merge (AC3), by `origin_branch_id`.
   */
  async listArtifactAssociationsByOriginBranch(
    workspaceId: WorkspaceId,
    branchId: BranchId,
  ): Promise<{ chunkLabel: IdeaLabel; artifactId: ArtifactId; status: ArtifactAssociationState }[]> {
    const result = await this.pool.query<{
      chunk_label: string;
      artifact_id: string;
      status: string;
    }>(
      `SELECT DISTINCT ON (chunk_label, artifact_id) chunk_label, artifact_id, status
       FROM chunk_artifacts
       WHERE workspace_id = $1 AND branch_id IS NULL AND origin_branch_id = $2
       ORDER BY chunk_label, artifact_id, version DESC`,
      [workspaceId, branchId],
    );
    return result.rows.map((row) => ({
      chunkLabel: row.chunk_label as IdeaLabel,
      artifactId: row.artifact_id as ArtifactId,
      status: row.status as ArtifactAssociationState,
    }));
  }

  private async lockBranchRow(
    client: PoolClient,
    workspaceId: WorkspaceId,
    branchId: BranchId,
  ): Promise<BranchRow> {
    const result = await client.query<BranchRow>(
      `SELECT workspace_id, branch_id, discipline, status
       FROM branches
       WHERE workspace_id = $1 AND branch_id = $2
       FOR UPDATE`,
      [workspaceId, branchId],
    );
    const row = result.rows[0];
    if (!row) {
      throw new BranchLifecycleError(
        'not-found',
        `no branch registered for workspace '${workspaceId}' and branch '${branchId}'`,
      );
    }
    return row;
  }

  private async promoteChunkDelta(
    client: PoolClient,
    workspaceId: WorkspaceId,
    branchId: BranchId,
    deltaRow: BranchChunkDeltaRow,
  ): Promise<void> {
    const ideaLabel = deltaRow.idea_label as IdeaLabel;
    const mainlineChunk = await this.chunkGraphRepository.findChunk(workspaceId, ideaLabel, {
      client,
      forUpdate: true,
    });

    const delta =
      deltaRow.delta_kind === 'upsert'
        ? {
            workspaceId,
            branchId,
            ideaLabel,
            deltaKind: 'upsert' as const,
            chunk: {
              workspaceId,
              ideaLabel,
              chunkType: deltaRow.chunk_type as PersistedChunk['chunkType'],
              discipline: deltaRow.discipline as PersistedChunk['discipline'],
              contextKind: deltaRow.context_kind as PersistedChunk['contextKind'],
              content: deltaRow.content ?? '',
              status: chunkLifecycleStatus(
                deltaRow.lifecycle_state as Parameters<typeof chunkLifecycleStatus>[0],
                deltaRow.activity_state as Parameters<typeof chunkLifecycleStatus>[1],
              ),
            },
          }
        : {
            workspaceId,
            branchId,
            ideaLabel,
            deltaKind: 'delete' as const,
          };

    const resolved = resolveChunkDelta(mainlineChunk, delta);
    if (resolved) {
      await this.chunkGraphRepository.saveChunk(resolved, { client, originBranchId: branchId });
      await this.appendChunkHistory(client, branchId, resolved);
      return;
    }
    // 'delete' delta: never a physical row delete (no precedent anywhere in
    // this schema) — deactivate the mainline row instead, preserving
    // lifecycleState and origin provenance. No-op if mainline never had it.
    if (mainlineChunk) {
      const deactivated = {
        ...mainlineChunk,
        status: chunkLifecycleStatus(mainlineChunk.status.lifecycleState, 'inactive'),
      };
      await this.chunkGraphRepository.saveChunk(deactivated, { client, originBranchId: branchId });
      await this.appendChunkHistory(client, branchId, deactivated);
    }
  }

  /**
   * Appends one permanent `chunk_history` row for this merge's promotion of
   * `chunk`, so this branch's contribution to `chunk.ideaLabel` remains
   * independently reconstructable even if a later, unrelated branch merges
   * the same idea label and overwrites `chunks.origin_branch_id` (technical
   * spec §"Pre-merge history reconstruction", `IDEA-69`). Insert-only: never
   * updates or deletes an existing row.
   */
  private async appendChunkHistory(
    client: PoolClient,
    branchId: BranchId,
    chunk: PersistedChunk,
  ): Promise<void> {
    await client.query(
      `INSERT INTO chunk_history
         (workspace_id, idea_label, origin_branch_id, chunk_type, discipline,
          context_kind, content, lifecycle_state, activity_state)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
      [
        chunk.workspaceId,
        chunk.ideaLabel,
        branchId,
        chunk.chunkType,
        chunk.discipline,
        chunk.contextKind,
        chunk.content,
        chunk.status.lifecycleState,
        chunk.status.activityState,
      ],
    );
  }

  /**
   * Reconstructs the full sequence of merges that have ever promoted a
   * chunk delta for `ideaLabel`, oldest first, independent of what the
   * current mutable `chunks` row shows (technical spec §"Pre-merge history
   * reconstruction", `IDEA-69`).
   */
  async listChunkHistoryByIdeaLabel(
    workspaceId: WorkspaceId,
    ideaLabel: IdeaLabel,
  ): Promise<ChunkHistoryEntry[]> {
    const result = await this.pool.query<ChunkHistoryRow>(
      `SELECT workspace_id, idea_label, origin_branch_id, chunk_type, discipline,
              context_kind, content, lifecycle_state, activity_state, merged_at
       FROM chunk_history
       WHERE workspace_id = $1 AND idea_label = $2
       ORDER BY merged_at ASC, id ASC`,
      [workspaceId, ideaLabel],
    );
    return result.rows.map(rowToChunkHistoryEntry);
  }

  private async promoteEdgeDelta(
    client: PoolClient,
    workspaceId: WorkspaceId,
    branchId: BranchId,
    deltaRow: BranchEdgeDeltaRow,
  ): Promise<void> {
    const sourceLabel = deltaRow.source_label as IdeaLabel;
    const targetLabel = deltaRow.target_label as IdeaLabel;
    const relationshipType = deltaRow.relationship_type as RelationshipType;

    const mainlineRecord = await this.chunkGraphRepository.findEdgeLineageRecordOnClient(
      client,
      workspaceId,
      sourceLabel,
      targetLabel,
      relationshipType,
      { forUpdate: true },
    );

    const resolved = resolveEdgeDelta(
      { workspaceId, sourceLabel, targetLabel, relationshipType },
      mainlineRecord?.lineage,
      {
        workspaceId,
        branchId,
        sourceLabel,
        targetLabel,
        relationshipType,
        deltaKind: deltaRow.delta_kind as 'upsert' | 'deactivate',
      },
    );

    if (!resolved) {
      return;
    }
    await this.chunkGraphRepository.saveEdgeLineageOnClient(client, resolved, branchId);
    // Defensive: `currentEdgeVersion` throws if `resolved` somehow has zero
    // versions, which would indicate a domain invariant violation rather
    // than a legitimate no-op — surfacing it here rather than silently
    // continuing.
    currentEdgeVersion(resolved);
  }

  private async promoteArtifactAssociation(
    client: PoolClient,
    workspaceId: WorkspaceId,
    branchId: BranchId,
    chunkLabel: IdeaLabel,
    artifactId: ArtifactId,
  ): Promise<void> {
    await client.query('SELECT pg_advisory_xact_lock($1)', [
      artifactIdentityLockKey(workspaceId, chunkLabel, artifactId, undefined),
    ]);

    const branchRow = await this.lockCurrentAssociationRow(
      client,
      workspaceId,
      chunkLabel,
      artifactId,
      branchId,
    );
    if (!branchRow) {
      // Nothing to promote: the earlier DISTINCT scan found this identity,
      // but a concurrent process cleared it — nothing to do.
      return;
    }
    const mainlineRow = await this.lockCurrentAssociationRow(
      client,
      workspaceId,
      chunkLabel,
      artifactId,
      undefined,
    );

    const originBranchId = (branchRow.origin_branch_id as BranchId | null) ?? branchId;
    const decision = resolveArtifactAssociationPromotion(
      mainlineRow ? (mainlineRow.status as ArtifactAssociationState) : undefined,
      branchRow.status as ArtifactAssociationState,
      { chunkLabel, artifactId },
    );

    // Reuse the domain constructors purely to validate the terminal states
    // being written are ones the domain would actually produce, rather than
    // hand-building raw status strings (constitution Principle IV).
    if (decision.action === 'seed') {
      const seeded = deactivateAssociation(
        createAssociation(workspaceId, chunkLabel, artifactId, undefined, originBranchId),
      );
      currentAssociationVersion(seeded); // validates the lineage is well-formed
      try {
        await client.query(
          `INSERT INTO chunk_artifacts (${CHUNK_ARTIFACT_ROW_COLUMNS})
           VALUES ($1, $2, $3, NULL, $4, 1, 'active')`,
          [workspaceId, chunkLabel, artifactId, originBranchId],
        );
        await client.query(
          `INSERT INTO chunk_artifacts (${CHUNK_ARTIFACT_ROW_COLUMNS})
           VALUES ($1, $2, $3, NULL, $4, 2, 'deactivated')`,
          [workspaceId, chunkLabel, artifactId, originBranchId],
        );
      } catch (error) {
        if (isUniqueViolation(error)) {
          throw new ArtifactAssociationError(
            'duplicate-active-relationship',
            `a concurrent write already created a mainline chunk-artifact association for chunk '${chunkLabel}' and artifact '${artifactId}' in workspace '${workspaceId}'`,
          );
        }
        throw error;
      }
      return;
    }

    if (decision.action === 'append') {
      const nextVersion = (mainlineRow?.version ?? 0) + 1;
      // The newly-appended version's provenance is stamped with the
      // *currently merging* branch (mirrors `saveEdgeLineageOnClient`'s
      // per-version `origin_branch_id` stamping for newly-inserted edge
      // versions), not the mainline lineage's original creator — AC3 asks
      // for traceability to "the branch ... that produced" each promoted
      // change, and this version is produced by this merge.
      if (decision.state === 'deactivated' && mainlineRow) {
        await client.query(
          `UPDATE chunk_artifacts SET status = 'superseded', updated_at = now()
           WHERE workspace_id = $1 AND chunk_label = $2 AND artifact_id = $3
             AND branch_id IS NULL AND version = $4`,
          [workspaceId, chunkLabel, artifactId, mainlineRow.version],
        );
      }
      try {
        await client.query(
          `INSERT INTO chunk_artifacts (${CHUNK_ARTIFACT_ROW_COLUMNS})
           VALUES ($1, $2, $3, NULL, $4, $5, $6)`,
          [workspaceId, chunkLabel, artifactId, originBranchId, nextVersion, decision.state],
        );
      } catch (error) {
        if (isUniqueViolation(error)) {
          throw new ArtifactAssociationError(
            'duplicate-active-relationship',
            `an active chunk-artifact association already exists for chunk '${chunkLabel}' and artifact '${artifactId}' in workspace '${workspaceId}' (mainline)`,
          );
        }
        throw error;
      }
    }
    // decision.action === 'noop': mainline already reflects the branch's
    // terminal state (already inactive); nothing to write.
  }

  private async lockCurrentAssociationRow(
    client: PoolClient,
    workspaceId: WorkspaceId,
    chunkLabel: IdeaLabel,
    artifactId: ArtifactId,
    branchId: BranchId | undefined,
  ): Promise<ChunkArtifactRow | undefined> {
    const result = await client.query<ChunkArtifactRow>(
      `SELECT ${CHUNK_ARTIFACT_ROW_COLUMNS} FROM chunk_artifacts
       WHERE workspace_id = $1 AND chunk_label = $2 AND artifact_id = $3
         AND branch_id IS NOT DISTINCT FROM $4
       ORDER BY version DESC
       LIMIT 1
       FOR UPDATE`,
      [workspaceId, chunkLabel, artifactId, branchId ?? null],
    );
    return result.rows[0];
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
