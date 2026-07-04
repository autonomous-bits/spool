/**
 * Postgres-backed persistence adapter for branch-scoped chunk and edge
 * deltas, plus the read-time resolver that combines them with mainline
 * records into a branch's resolved view.
 *
 * Sources of authority:
 * - Story S02: draft work stays separate from approved context. Branch
 *   changes must be stored as branch-scoped delta records, not full copies
 *   of the mainline graph; a branch's resolved view is computed by
 *   combining those deltas with mainline records at read time.
 * - Technical spec §"Delta-based branch storage" (`IDEA-32`, `IDEA-33`).
 * - Technical spec §"Tenant isolation": every method takes a `workspaceId`
 *   and every SQL predicate filters on it explicitly.
 * - Technical spec §"Edge lineage persistence" / store AGENTS: resolved
 *   edge values must only ever be produced via the feature-01 domain
 *   constructors (`createEdge`, `deactivateEdge`) — never hand-built.
 * - Story out-of-scope: no suggestion (S04) or chunk-artifact-association
 *   (S05) tables, no divergence-marker persistence/consultation (S06), no
 *   merge transaction (S07). This adapter only resolves against *current* mainline
 *   state at read time.
 * - Story S11 (technical spec §"Required domain error categories",
 *   feature-01 tech spec §"Required lifecycle contracts — Branch",
 *   §"Discipline boundary"): branch-scoped writes (`saveChunkDelta`,
 *   `saveEdgeDelta`) now enforce the branch's own write-lock and discipline
 *   boundary before persisting a delta, so a stakeholder who attempts to
 *   write against a non-draft branch, or a chunk owned by a different
 *   discipline than the branch, is rejected with the domain's existing
 *   `write-locked` / `branch-isolation-violation` `BranchLifecycleError`
 *   categories rather than silently succeeding. This supersedes the
 *   previous documented boundary decision (store AGENTS.md, S02/S06/S07)
 *   that this enforcement belonged solely to a future API gateway. The
 *   guard check and the delta write happen inside one transaction with a
 *   `SELECT ... FOR UPDATE` on the branch's registration row (mirroring
 *   `MergeRepository.lockBranchRow`), so a concurrent submit/verify cannot
 *   flip the branch out of `draft` between the check and the write.
 * - nestjs-security skill: parameterized queries only, never string
 *   concatenation.
 */

import { Inject, Injectable } from '@nestjs/common';
import type { Pool, PoolClient } from 'pg';
import { chunkLifecycleStatus } from '../domain/chunk-lifecycle.js';
import {
  assertDisciplineBoundaryForWrite,
  assertGraphWriteAllowed,
  BranchLifecycleError,
  type BranchState,
} from '../domain/branch-lifecycle.js';
import {
  createEdge,
  deactivateEdge,
  currentEdgeVersion,
  EdgeLineageError,
  type EdgeLineage,
} from '../domain/edge-lineage.js';
import type {
  BranchId,
  ChunkType,
  ContextKind,
  Discipline,
  IdeaLabel,
  RelationshipType,
  WorkspaceId,
} from '../domain/types/index.js';
import { PG_POOL } from './database-pool.provider.js';
import { ChunkGraphRepository, type PersistedChunk } from './chunk-graph.repository.js';

export type BranchChunkDeltaKind = 'upsert' | 'delete';
export type BranchEdgeDeltaKind = 'upsert' | 'deactivate';

/**
 * A branch's own change to a single idea label. `chunk` is required when
 * `deltaKind` is `'upsert'` and ignored (may be omitted) when `'delete'`.
 */
export interface BranchChunkDelta {
  readonly workspaceId: WorkspaceId;
  readonly branchId: BranchId;
  readonly ideaLabel: IdeaLabel;
  readonly deltaKind: BranchChunkDeltaKind;
  readonly chunk?: PersistedChunk;
}

/**
 * A branch's own change to a single edge identity
 * (workspace, source label, target label, relationship type). `'upsert'`
 * asserts the identity is active in the branch's view; `'deactivate'` hides
 * or deactivates it in the branch's view.
 */
export interface BranchEdgeDelta {
  readonly workspaceId: WorkspaceId;
  readonly branchId: BranchId;
  readonly sourceLabel: IdeaLabel;
  readonly targetLabel: IdeaLabel;
  readonly relationshipType: RelationshipType;
  readonly deltaKind: BranchEdgeDeltaKind;
}

interface BranchChunkDeltaRow {
  readonly workspace_id: string;
  readonly branch_id: string;
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
  readonly workspace_id: string;
  readonly branch_id: string;
  readonly source_label: string;
  readonly target_label: string;
  readonly relationship_type: string;
  readonly delta_kind: string;
}

function rowToBranchChunkDelta(row: BranchChunkDeltaRow): BranchChunkDelta {
  const base = {
    workspaceId: row.workspace_id as WorkspaceId,
    branchId: row.branch_id as BranchId,
    ideaLabel: row.idea_label as IdeaLabel,
    deltaKind: row.delta_kind as BranchChunkDeltaKind,
  };
  if (row.delta_kind !== 'upsert') {
    return base;
  }
  return {
    ...base,
    chunk: {
      workspaceId: row.workspace_id as WorkspaceId,
      ideaLabel: row.idea_label as IdeaLabel,
      chunkType: row.chunk_type as ChunkType,
      discipline: row.discipline as Discipline,
      contextKind: row.context_kind as ContextKind,
      content: row.content ?? '',
      status: chunkLifecycleStatus(
        row.lifecycle_state as Parameters<typeof chunkLifecycleStatus>[0],
        row.activity_state as Parameters<typeof chunkLifecycleStatus>[1],
      ),
    },
  };
}

function rowToBranchEdgeDelta(row: BranchEdgeDeltaRow): BranchEdgeDelta {
  return {
    workspaceId: row.workspace_id as WorkspaceId,
    branchId: row.branch_id as BranchId,
    sourceLabel: row.source_label as IdeaLabel,
    targetLabel: row.target_label as IdeaLabel,
    relationshipType: row.relationship_type as RelationshipType,
    deltaKind: row.delta_kind as BranchEdgeDeltaKind,
  };
}

/**
 * Resolves a single idea label's effective chunk for a branch view, given
 * the mainline chunk (if any) and the branch's own delta (if any) for that
 * label. Pure function so the merge semantics are unit-testable without a
 * database (technical spec §"Delta-based branch storage").
 *
 * - no delta -> mainline chunk unchanged (or absent).
 * - `upsert` delta -> the branch's own chunk overrides/adds; mainline is
 *   never mutated.
 * - `delete` delta -> omitted from the *resolved* view regardless of
 *   mainline, since that view models "what is currently visible in this
 *   branch". This does not lose the branch's explicit deactivation intent:
 *   `branch_chunk_deltas` still persists a `delta_kind = 'delete'` row for
 *   the idea label (Meridian `IDEA-32`: "branch-specific chunk row with
 *   status set to 'deactivated'"), distinguishable from an idea label the
 *   branch never touched at all (no row). `BranchGraphRepository.
 *   listChunkDeltas`/`listDeactivatedIdeaLabels` surface that explicit
 *   record for a caller that needs to show "this branch deactivates X"
 *   rather than only the net resolved view (fixes an ambiguity flagged
 *   during rubber-duck review of Feature 01/02 against Meridian).
 */
export function resolveChunkDelta(
  mainlineChunk: PersistedChunk | undefined,
  delta: BranchChunkDelta | undefined,
): PersistedChunk | undefined {
  if (!delta) {
    return mainlineChunk;
  }
  if (delta.deltaKind === 'delete') {
    return undefined;
  }
  if (!delta.chunk) {
    throw new Error(
      `branch chunk delta for '${delta.ideaLabel}' has deltaKind 'upsert' but no chunk payload`,
    );
  }
  return delta.chunk;
}

/**
 * Resolves a single edge identity's effective lineage for a branch view,
 * given the mainline lineage (if any) and the branch's own delta (if any)
 * for that identity. Pure function so the merge semantics are
 * unit-testable without a database.
 *
 * See the S02 implementation plan's "Edge-delta resolution matrix" for the
 * full behavior table. Every branch is produced exclusively via the
 * feature-01 domain constructors `createEdge`/`deactivateEdge` — never a
 * hand-built `EdgeLineage` — so an invalid lineage can never result.
 *
 * Throws `EdgeLineageError('invalid-state-transition')` if the branch
 * asserts `'upsert'` over a mainline lineage whose current version is
 * already `'deactivated'`: the domain has no public reactivation
 * transition, and inventing one is out of this story's scope.
 */
export function resolveEdgeDelta(
  identity: {
    readonly workspaceId: WorkspaceId;
    readonly sourceLabel: IdeaLabel;
    readonly targetLabel: IdeaLabel;
    readonly relationshipType: RelationshipType;
  },
  mainlineLineage: EdgeLineage | undefined,
  delta: BranchEdgeDelta | undefined,
): EdgeLineage | undefined {
  if (!delta) {
    return mainlineLineage;
  }

  if (delta.deltaKind === 'deactivate') {
    if (!mainlineLineage) {
      // Branch added, then deactivated, its own edge — mainline never saw it.
      return deactivateEdge(
        createEdge(
          identity.workspaceId,
          identity.sourceLabel,
          identity.targetLabel,
          identity.relationshipType,
        ),
      );
    }
    const current = currentEdgeVersion(mainlineLineage);
    return current.state === 'active'
      ? deactivateEdge(mainlineLineage)
      : mainlineLineage; // already inactive: idempotent no-op
  }

  // delta.deltaKind === 'upsert'
  if (!mainlineLineage) {
    return createEdge(
      identity.workspaceId,
      identity.sourceLabel,
      identity.targetLabel,
      identity.relationshipType,
    );
  }
  const current = currentEdgeVersion(mainlineLineage);
  if (current.state === 'active') {
    return mainlineLineage; // already active: idempotent no-op
  }
  throw new EdgeLineageError(
    'invalid-state-transition',
    `branch '${identity.workspaceId}' cannot upsert edge '${identity.sourceLabel}' -[${identity.relationshipType}]-> '${identity.targetLabel}' because the mainline edge is '${current.state}'; reactivating a deactivated mainline edge is out of scope for this story`,
  );
}

@Injectable()
export class BranchGraphRepository {
  constructor(
    @Inject(PG_POOL) private readonly pool: Pool,
    private readonly chunkGraphRepository: ChunkGraphRepository,
  ) {}

  /**
   * Locks the branch's registration row (`status`, `discipline`) with
   * `SELECT ... FOR UPDATE` inside the caller's transaction and enforces
   * write-lock and discipline-boundary guards before a branch-scoped write
   * (story S11, technical spec §"Required domain error categories";
   * feature-01 tech spec §"Required lifecycle contracts — Branch",
   * §"Discipline boundary").
   *
   * The row lock matters, not just the guard logic: without it, a
   * concurrent `submitBranch`/`verifyBranch` (which also locks this row via
   * `MergeRepository.lockBranchRow`) could flip the branch out of `draft`
   * between this check and the delta INSERT that follows in the same
   * transaction, letting a write slip through a branch that is, from that
   * moment on, write-locked. Locking here serializes against that race the
   * same way `MergeRepository` already does for submit/verify/merge.
   *
   * Throws `BranchLifecycleError('not-found')` if no branch is registered
   * for this workspace/branch identity — a branch-scoped write cannot be
   * evaluated against a lifecycle state and discipline that do not exist.
   * Throws `BranchLifecycleError('write-locked')` if the branch is not
   * `draft`. Throws `BranchLifecycleError('branch-isolation-violation')` if
   * `targetDiscipline` is supplied and differs from the branch's own
   * discipline.
   */
  private async assertWritableBranch(
    client: PoolClient,
    workspaceId: WorkspaceId,
    branchId: BranchId,
    targetDiscipline?: Discipline,
  ): Promise<void> {
    const result = await client.query<{ status: string; discipline: string }>(
      `SELECT status, discipline FROM branches WHERE workspace_id = $1 AND branch_id = $2 FOR UPDATE`,
      [workspaceId, branchId],
    );
    const row = result.rows[0];
    if (!row) {
      throw new BranchLifecycleError(
        'not-found',
        `no branch registered for workspace '${workspaceId}' and branch '${branchId}'`,
      );
    }
    assertGraphWriteAllowed(row.status as BranchState);
    if (targetDiscipline) {
      assertDisciplineBoundaryForWrite(row.discipline as Discipline, targetDiscipline);
    }
  }

  /**
   * Resolves the discipline that "owns" an idea label for branch-isolation
   * purposes (story S11, technical spec §"Discipline boundary": "A branch
   * may modify chunks and edges owned by its discipline"): the branch's own
   * pending delta for this label if one already exists (a branch's own
   * prior upsert is authoritative over stale mainline state within the same
   * branch), otherwise the mainline chunk's discipline if one exists.
   * Returns `undefined` if the idea label has no known chunk anywhere yet
   * (a brand-new idea introduced entirely within this delta), in which case
   * ownership cannot be checked against anything but the delta's own
   * declared discipline.
   *
   * Read on the same transactional `client` as the caller so this sees a
   * consistent snapshot with the write that follows.
   */
  private async resolveChunkDiscipline(
    client: PoolClient,
    workspaceId: WorkspaceId,
    branchId: BranchId,
    ideaLabel: IdeaLabel,
  ): Promise<Discipline | undefined> {
    const branchRow = await client.query<{ discipline: string | null }>(
      `SELECT discipline FROM branch_chunk_deltas
       WHERE workspace_id = $1 AND branch_id = $2 AND idea_label = $3`,
      [workspaceId, branchId, ideaLabel],
    );
    const branchDiscipline = branchRow.rows[0]?.discipline;
    if (branchDiscipline) {
      return branchDiscipline as Discipline;
    }
    const mainlineRow = await client.query<{ discipline: string }>(
      `SELECT discipline FROM chunks WHERE workspace_id = $1 AND idea_label = $2`,
      [workspaceId, ideaLabel],
    );
    return mainlineRow.rows[0]?.discipline as Discipline | undefined;
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

  async saveChunkDelta(delta: BranchChunkDelta): Promise<void> {
    if (delta.deltaKind === 'upsert' && !delta.chunk) {
      throw new Error("saveChunkDelta requires 'chunk' when deltaKind is 'upsert'");
    }
    const chunk = delta.chunk;
    const client = await this.pool.connect();
    try {
      await client.query('BEGIN');
      // Ownership for isolation purposes is the *existing* chunk's
      // discipline (branch's own prior delta, else mainline) when this
      // idea label already has one; otherwise fall back to the delta
      // payload's own declared discipline (a brand-new idea, or a
      // 'delete' delta with no payload — `chunk?.discipline` is undefined
      // there and existingDiscipline carries the check instead).
      const existingDiscipline = await this.resolveChunkDiscipline(
        client,
        delta.workspaceId,
        delta.branchId,
        delta.ideaLabel,
      );
      await this.assertWritableBranch(
        client,
        delta.workspaceId,
        delta.branchId,
        existingDiscipline ?? chunk?.discipline,
      );
      await client.query(
        `INSERT INTO branch_chunk_deltas (
           workspace_id, branch_id, idea_label, delta_kind, chunk_type,
           discipline, context_kind, content, lifecycle_state, activity_state,
           updated_at
         ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
         ON CONFLICT (workspace_id, branch_id, idea_label) DO UPDATE SET
           delta_kind = EXCLUDED.delta_kind,
           chunk_type = EXCLUDED.chunk_type,
           discipline = EXCLUDED.discipline,
           context_kind = EXCLUDED.context_kind,
           content = EXCLUDED.content,
           lifecycle_state = EXCLUDED.lifecycle_state,
           activity_state = EXCLUDED.activity_state,
           updated_at = now()`,
        [
          delta.workspaceId,
          delta.branchId,
          delta.ideaLabel,
          delta.deltaKind,
          chunk?.chunkType ?? null,
          chunk?.discipline ?? null,
          chunk?.contextKind ?? null,
          chunk?.content ?? null,
          chunk?.status.lifecycleState ?? null,
          chunk?.status.activityState ?? null,
        ],
      );
      await client.query('COMMIT');
    } catch (error) {
      await this.rollbackQuietly(client);
      throw error;
    } finally {
      client.release();
    }
  }

  async listChunkDeltas(
    workspaceId: WorkspaceId,
    branchId: BranchId,
  ): Promise<BranchChunkDelta[]> {
    const result = await this.pool.query<BranchChunkDeltaRow>(
      `SELECT workspace_id, branch_id, idea_label, delta_kind, chunk_type,
              discipline, context_kind, content, lifecycle_state, activity_state
       FROM branch_chunk_deltas
       WHERE workspace_id = $1 AND branch_id = $2
       ORDER BY idea_label`,
      [workspaceId, branchId],
    );
    return result.rows.map(rowToBranchChunkDelta);
  }

  /**
   * Lists the idea labels this branch explicitly deactivates
   * (`delta_kind = 'delete'` rows in `branch_chunk_deltas`), distinguishing
   * "this branch proposes deactivating this idea" from "this idea simply
   * isn't in the branch's resolved view" — the latter is also true for an
   * idea label the branch never touched at all. `resolveChunks` (the net
   * "what's currently visible" view) omits deactivated labels entirely by
   * design; this method is the explicit, purpose-built counterpart for a
   * caller (e.g. branch review UI) that needs to show deactivation intent
   * rather than only the net resolved view (fixes an ambiguity flagged
   * during rubber-duck review of Feature 01/02 against Meridian `IDEA-32`).
   */
  async listDeactivatedIdeaLabels(
    workspaceId: WorkspaceId,
    branchId: BranchId,
  ): Promise<IdeaLabel[]> {
    const deltas = await this.listChunkDeltas(workspaceId, branchId);
    return deltas
      .filter((delta) => delta.deltaKind === 'delete')
      .map((delta) => delta.ideaLabel)
      .sort((a, b) => a.localeCompare(b));
  }

  async saveEdgeDelta(delta: BranchEdgeDelta): Promise<void> {
    // Story S11, technical spec §"Discipline boundary": "A branch may
    // modify chunks and edges owned by its discipline. It may create
    // cross-disciplinary edges to other disciplines' chunks only when it
    // does not modify those target chunks." An edge's own "owning"
    // discipline is not stored anywhere directly (edges carry no
    // discipline column), but the spec's own phrasing — permitting the
    // *target* to cross into another discipline while treating the
    // relationship as something the branch "creates" from its own side —
    // grounds the narrowest defensible rule: ownership is the *source*
    // chunk's discipline (branch's own prior delta, else mainline), the
    // same resolution `saveChunkDelta` already uses for chunk ownership.
    // The target side is deliberately left unchecked here, matching the
    // spec's explicit cross-disciplinary-target allowance. If the source
    // label has no known chunk anywhere yet, ownership cannot be
    // evaluated and the write is allowed (an edge may legitimately
    // reference a label that has no persisted chunk yet).
    const client = await this.pool.connect();
    try {
      await client.query('BEGIN');
      const sourceDiscipline = await this.resolveChunkDiscipline(
        client,
        delta.workspaceId,
        delta.branchId,
        delta.sourceLabel,
      );
      await this.assertWritableBranch(client, delta.workspaceId, delta.branchId, sourceDiscipline);
      await client.query(
        `INSERT INTO branch_edge_deltas (
           workspace_id, branch_id, source_label, target_label,
           relationship_type, delta_kind, updated_at
         ) VALUES ($1, $2, $3, $4, $5, $6, now())
         ON CONFLICT (workspace_id, branch_id, source_label, target_label, relationship_type)
         DO UPDATE SET delta_kind = EXCLUDED.delta_kind, updated_at = now()`,
        [
          delta.workspaceId,
          delta.branchId,
          delta.sourceLabel,
          delta.targetLabel,
          delta.relationshipType,
          delta.deltaKind,
        ],
      );
      await client.query('COMMIT');
    } catch (error) {
      await this.rollbackQuietly(client);
      throw error;
    } finally {
      client.release();
    }
  }

  async listEdgeDeltas(
    workspaceId: WorkspaceId,
    branchId: BranchId,
  ): Promise<BranchEdgeDelta[]> {
    const result = await this.pool.query<BranchEdgeDeltaRow>(
      `SELECT workspace_id, branch_id, source_label, target_label,
              relationship_type, delta_kind
       FROM branch_edge_deltas
       WHERE workspace_id = $1 AND branch_id = $2
       ORDER BY source_label, target_label, relationship_type`,
      [workspaceId, branchId],
    );
    return result.rows.map(rowToBranchEdgeDelta);
  }

  /**
   * Computes a branch's resolved chunk view: mainline chunks (read via
   * `ChunkGraphRepository`, untouched by this method) overridden or added to
   * by this branch's own deltas, with `'delete'` deltas omitted. Never
   * writes to the mainline `chunks` table (AC2). Pair with
   * `listDeactivatedIdeaLabels` when a caller needs to distinguish "this
   * branch deactivates idea X" from "idea X isn't in this branch's view" —
   * this method alone only reflects the net result, by design.
   */
  async resolveChunks(
    workspaceId: WorkspaceId,
    branchId: BranchId,
  ): Promise<PersistedChunk[]> {
    const [mainlineChunks, deltas] = await Promise.all([
      this.chunkGraphRepository.listChunks(workspaceId),
      this.listChunkDeltas(workspaceId, branchId),
    ]);

    const mainlineByLabel = new Map(
      mainlineChunks.map((chunk) => [chunk.ideaLabel, chunk]),
    );
    const deltaByLabel = new Map(deltas.map((delta) => [delta.ideaLabel, delta]));

    const labels = new Set<IdeaLabel>([
      ...mainlineByLabel.keys(),
      ...deltaByLabel.keys(),
    ]);

    const resolved: PersistedChunk[] = [];
    for (const label of labels) {
      const chunk = resolveChunkDelta(
        mainlineByLabel.get(label),
        deltaByLabel.get(label),
      );
      if (chunk) {
        resolved.push(chunk);
      }
    }
    return resolved.sort((a, b) => a.ideaLabel.localeCompare(b.ideaLabel));
  }

  /**
   * Computes a branch's resolved edge-lineage view: mainline lineages (read
   * via `ChunkGraphRepository`, untouched by this method) overridden or
   * added to by this branch's own deltas. Never writes to the mainline
   * `edge_versions` table (AC2). See `resolveEdgeDelta` for the full
   * per-identity resolution matrix.
   */
  async resolveEdgeLineages(
    workspaceId: WorkspaceId,
    branchId: BranchId,
  ): Promise<EdgeLineage[]> {
    const [mainlineLineages, deltas] = await Promise.all([
      this.chunkGraphRepository.listEdgeLineages(workspaceId),
      this.listEdgeDeltas(workspaceId, branchId),
    ]);

    const identityKey = (id: {
      sourceLabel: IdeaLabel;
      targetLabel: IdeaLabel;
      relationshipType: RelationshipType;
    }): string => `${id.sourceLabel}\u0000${id.targetLabel}\u0000${id.relationshipType}`;

    const mainlineByIdentity = new Map(
      mainlineLineages.map((lineage) => [
        identityKey(currentEdgeVersion(lineage)),
        lineage,
      ]),
    );
    const deltaByIdentity = new Map(
      deltas.map((delta) => [identityKey(delta), delta]),
    );

    const identities = new Set<string>([
      ...mainlineByIdentity.keys(),
      ...deltaByIdentity.keys(),
    ]);

    const resolved: EdgeLineage[] = [];
    for (const key of identities) {
      const delta = deltaByIdentity.get(key);
      const mainline = mainlineByIdentity.get(key);
      const identity = delta ?? currentEdgeVersion(mainline!);
      const lineage = resolveEdgeDelta(
        {
          workspaceId,
          sourceLabel: identity.sourceLabel,
          targetLabel: identity.targetLabel,
          relationshipType: identity.relationshipType,
        },
        mainline,
        delta,
      );
      if (lineage) {
        resolved.push(lineage);
      }
    }
    return resolved;
  }
}
