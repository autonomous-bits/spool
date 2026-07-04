/**
 * Postgres-backed persistence adapter for the mainline chunk + edge-lineage
 * graph.
 *
 * Sources of authority:
 * - Story S01: durably store the versioned chunk and edge graph so it
 *   survives process restarts.
 * - Technical spec §"Store owns persistence": persistence code must map to
 *   and preserve Feature 01 domain invariants; it must not redefine
 *   lifecycle or authorization rules. Every read/write below routes through
 *   the feature-01 domain constructors (`chunkLifecycleStatus`, `createEdge`,
 *   `supersedeEdge`, `deactivateEdge`) rather than building raw values.
 * - Technical spec §"Tenant isolation": every method takes a `workspaceId`
 *   and every SQL predicate filters on it explicitly.
 * - nestjs-security skill: parameterized queries only, never string
 *   concatenation.
 */

import { Inject, Injectable } from '@nestjs/common';
import { DatabaseError, type Pool, type PoolClient } from 'pg';
import {
  chunkLifecycleStatus,
  type ChunkLifecycleStatus,
} from '../domain/chunk-lifecycle.js';
import { PG_POOL } from './database-pool.provider.js';
import {
  createEdge,
  deactivateEdge,
  supersedeEdge,
  type EdgeLineage,
  EdgeLineageError,
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

/** Postgres SQLSTATE for a unique-constraint violation. */
const UNIQUE_VIOLATION = '23505';

/**
 * Maps a raw Postgres constraint failure to the technical spec's required
 * domain error categories rather than letting an ad hoc `pg` error escape
 * the persistence boundary. Non-database errors (including
 * `EdgeLineageError`s already thrown by domain/application code above) pass
 * through unchanged.
 */
export function mapPersistenceError(error: unknown): unknown {
  if (!(error instanceof DatabaseError) || error.code !== UNIQUE_VIOLATION) {
    return error;
  }
  if (error.constraint === 'edge_versions_one_active_idx') {
    return new EdgeLineageError(
      'duplicate-active-relationship',
      'a concurrent write already created an active relationship for this workspace, source label, target label, and relationship type',
    );
  }
  return new EdgeLineageError(
    'lineage-violation',
    'a concurrent write already persisted this edge version',
  );
}

export interface PersistedChunk {
  readonly workspaceId: WorkspaceId;
  readonly ideaLabel: IdeaLabel;
  readonly chunkType: ChunkType;
  readonly discipline: Discipline;
  readonly contextKind: ContextKind;
  readonly content: string;
  readonly status: ChunkLifecycleStatus;
  /**
   * The branch whose merge most recently promoted this chunk to mainline
   * (story S07, technical spec §"Pre-merge history reconstruction",
   * `IDEA-69`); `undefined` for a chunk written directly on mainline, or
   * never (re-)promoted by a merge. Last-writer-wins — see
   * `MIGRATE_CHUNKS_ORIGIN_BRANCH_ID` in `schema.ts` for why.
   */
  readonly originBranchId?: BranchId;
}

interface ChunkRow {
  readonly workspace_id: string;
  readonly idea_label: string;
  readonly chunk_type: string;
  readonly discipline: string;
  readonly context_kind: string;
  readonly content: string;
  readonly lifecycle_state: string;
  readonly activity_state: string;
  readonly origin_branch_id?: string | null;
}

interface EdgeVersionRow {
  readonly workspace_id: string;
  readonly source_label: string;
  readonly target_label: string;
  readonly relationship_type: string;
  readonly lineage_seq: number;
  readonly version: number;
  readonly state: string;
  readonly succeeded_by_relationship_type: string | null;
  readonly succeeded_by_lineage_seq: number | null;
  readonly origin_branch_id?: string | null;
}

const CHUNK_COLUMNS = `workspace_id, idea_label, chunk_type, discipline, context_kind,
       content, lifecycle_state, activity_state, origin_branch_id`;

const EDGE_VERSION_COLUMNS = `workspace_id, source_label, target_label, relationship_type,
       lineage_seq, version, state, succeeded_by_relationship_type, succeeded_by_lineage_seq,
       origin_branch_id`;

/**
 * A single generation of an edge lineage, as read back from persistence
 * (story S03). `succeededBy` is set only when this generation's terminal
 * version was replaced by a relationship-type change (technical spec
 * §"Edge lineage persistence"): it precisely identifies the successor
 * lineage's relationship type and generation, since a bare relationship
 * type would be ambiguous across repeated type changes (e.g. A -> B -> A).
 */
export interface EdgeLineageRecord {
  readonly lineage: EdgeLineage;
  readonly lineageSeq: number;
  readonly succeededBy?: {
    readonly relationshipType: RelationshipType;
    readonly lineageSeq: number;
  };
}

function chunkRowToPersistedChunk(row: ChunkRow): PersistedChunk {
  const base: PersistedChunk = {
    workspaceId: row.workspace_id as WorkspaceId,
    ideaLabel: row.idea_label as IdeaLabel,
    chunkType: row.chunk_type as ChunkType,
    discipline: row.discipline as Discipline,
    contextKind: row.context_kind as ContextKind,
    content: row.content,
    status: chunkLifecycleStatus(
      row.lifecycle_state as ChunkLifecycleStatus['lifecycleState'],
      row.activity_state as ChunkLifecycleStatus['activityState'],
    ),
  };
  return row.origin_branch_id == null
    ? base
    : { ...base, originBranchId: row.origin_branch_id as BranchId };
}

/**
 * Reconstructs an `EdgeLineage` from its persisted, version-ordered rows by
 * replaying the same domain transitions (`createEdge` → `supersedeEdge`* →
 * optional `deactivateEdge`) that produced it. This guarantees a read-back
 * lineage can only ever be a value the feature-01 domain would itself
 * produce — invalid or hand-forged lineages cannot round-trip.
 *
 * Rows must belong to a single lineage identity and be sorted by `version`
 * ascending. Throws `EdgeLineageError` (`lineage-violation`) if the version
 * sequence is not contiguous starting at 1, guarding against manually edited
 * or corrupted data.
 */
function rowsToEdgeLineage(rows: readonly EdgeVersionRow[]): EdgeLineage {
  const first = rows[0];
  if (!first) {
    throw new EdgeLineageError(
      'lineage-violation',
      'cannot reconstruct an edge lineage from zero rows',
    );
  }
  rows.forEach((row, index) => {
    if (
      row.workspace_id !== first.workspace_id ||
      row.source_label !== first.source_label ||
      row.target_label !== first.target_label ||
      row.relationship_type !== first.relationship_type ||
      row.lineage_seq !== first.lineage_seq
    ) {
      throw new EdgeLineageError(
        'lineage-violation',
        `all rows passed to rowsToEdgeLineage must share one lineage identity and generation; row at position ${String(index)} does not match the first row's workspace/source/target/relationship type/lineage_seq`,
      );
    }
    if (row.version !== index + 1) {
      throw new EdgeLineageError(
        'lineage-violation',
        `edge lineage versions must be contiguous starting at 1; found gap or out-of-order version '${String(row.version)}' at position ${String(index)}`,
      );
    }
    const isLastRow = index === rows.length - 1;
    const expectedStates = isLastRow ? ['active', 'deactivated'] : ['superseded'];
    if (!expectedStates.includes(row.state)) {
      throw new EdgeLineageError(
        'lineage-violation',
        `persisted edge version ${String(row.version)} has unexpected state '${row.state}'; expected one of [${expectedStates.join(', ')}]`,
      );
    }
  });

  const last = rows[rows.length - 1];
  if (last?.state === 'deactivated' && rows.length === 1) {
    // Deactivation always supersedes a prior active version (feature-01
    // `deactivateEdge` appends rather than mutating in place; technical
    // spec §"Edge lineage persistence"), so a lineage can never be
    // deactivated as its lone, first-ever version.
    throw new EdgeLineageError(
      'lineage-violation',
      "a lineage's first version cannot be 'deactivated'; deactivation must supersede a prior active version, so a lone deactivated row is invalid persisted state",
    );
  }

  let lineage = createEdge(
    first.workspace_id as WorkspaceId,
    first.source_label as IdeaLabel,
    first.target_label as IdeaLabel,
    first.relationship_type as RelationshipType,
  );
  // If the lineage's terminal row is `deactivated`, that row is itself
  // appended by `deactivateEdge`, so replay `supersedeEdge` only up to the
  // row *before* it, then call `deactivateEdge` once as the final step.
  const supersedeCount =
    last?.state === 'deactivated' ? rows.length - 2 : rows.length - 1;
  for (let i = 0; i < supersedeCount; i++) {
    lineage = supersedeEdge(lineage, {
      workspaceId: first.workspace_id as WorkspaceId,
      sourceLabel: first.source_label as IdeaLabel,
      targetLabel: first.target_label as IdeaLabel,
      relationshipType: first.relationship_type as RelationshipType,
    });
  }

  if (last?.state === 'deactivated') {
    lineage = deactivateEdge(lineage);
  }
  return lineage;
}


/**
 * Wraps a reconstructed lineage with its generation number and, if the
 * generation's terminal row carries a successor pointer, the precise
 * relationship type + generation that replaced it.
 */
function rowsToEdgeLineageRecord(rows: readonly EdgeVersionRow[]): EdgeLineageRecord {
  const lineage = rowsToEdgeLineage(rows);
  const last = rows[rows.length - 1];
  const succeededBy =
    last?.succeeded_by_relationship_type != null && last.succeeded_by_lineage_seq != null
      ? {
          relationshipType: last.succeeded_by_relationship_type as RelationshipType,
          lineageSeq: last.succeeded_by_lineage_seq,
        }
      : undefined;
  return succeededBy
    ? { lineage, lineageSeq: rows[0]!.lineage_seq, succeededBy }
    : { lineage, lineageSeq: rows[0]!.lineage_seq };
}

@Injectable()
export class ChunkGraphRepository {
  constructor(@Inject(PG_POOL) private readonly pool: Pool) {}

  /**
   * Upserts a chunk. `options.client` lets a caller compose this write into
   * a larger externally-managed transaction (story S07's merge, in
   * particular); defaults to the repository's own pool otherwise.
   * `options.originBranchId`, when provided, stamps (or overwrites) the
   * mainline row's branch-merge provenance (technical spec §"Pre-merge
   * history reconstruction"); when omitted, any existing provenance is left
   * untouched (`COALESCE`) rather than being cleared by an ordinary,
   * non-merge save.
   */
  async saveChunk(
    chunk: PersistedChunk,
    options?: { readonly client?: PoolClient; readonly originBranchId?: BranchId },
  ): Promise<void> {
    const runner = options?.client ?? this.pool;
    await runner.query(
      `INSERT INTO chunks (
         ${CHUNK_COLUMNS}, updated_at
       ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
       ON CONFLICT (workspace_id, idea_label) DO UPDATE SET
         chunk_type = EXCLUDED.chunk_type,
         discipline = EXCLUDED.discipline,
         context_kind = EXCLUDED.context_kind,
         content = EXCLUDED.content,
         lifecycle_state = EXCLUDED.lifecycle_state,
         activity_state = EXCLUDED.activity_state,
         origin_branch_id = COALESCE(EXCLUDED.origin_branch_id, chunks.origin_branch_id),
         updated_at = now()`,
      [
        chunk.workspaceId,
        chunk.ideaLabel,
        chunk.chunkType,
        chunk.discipline,
        chunk.contextKind,
        chunk.content,
        chunk.status.lifecycleState,
        chunk.status.activityState,
        options?.originBranchId ?? null,
      ],
    );
  }

  /**
   * Reads a mainline chunk. `options.client` composes this read into a
   * larger externally-managed transaction; `options.forUpdate` takes a row
   * lock (`SELECT ... FOR UPDATE`) so a caller holding a transaction (e.g.
   * story S07's merge) can safely read-then-write without racing a
   * concurrent mainline writer — only valid when `options.client` is also
   * provided (a lock taken outside a transaction is released immediately
   * and is meaningless).
   */
  async findChunk(
    workspaceId: WorkspaceId,
    ideaLabel: IdeaLabel,
    options?: { readonly client?: PoolClient; readonly forUpdate?: boolean },
  ): Promise<PersistedChunk | undefined> {
    const runner = options?.client ?? this.pool;
    const lockClause = options?.client && options.forUpdate ? ' FOR UPDATE' : '';
    const result = await runner.query<ChunkRow>(
      `SELECT ${CHUNK_COLUMNS}
       FROM chunks
       WHERE workspace_id = $1 AND idea_label = $2${lockClause}`,
      [workspaceId, ideaLabel],
    );
    const row = result.rows[0];
    return row ? chunkRowToPersistedChunk(row) : undefined;
  }

  async listChunks(workspaceId: WorkspaceId): Promise<PersistedChunk[]> {
    const result = await this.pool.query<ChunkRow>(
      `SELECT ${CHUNK_COLUMNS}
       FROM chunks
       WHERE workspace_id = $1
       ORDER BY idea_label`,
      [workspaceId],
    );
    return result.rows.map(chunkRowToPersistedChunk);
  }

  /**
   * Lists every mainline chunk promoted by a specific branch's merge (story
   * S07, AC3): `origin_branch_id = branchId`. Subject to the last-writer-
   * wins caveat documented on `PersistedChunk.originBranchId`.
   */
  async listChunksByOriginBranch(
    workspaceId: WorkspaceId,
    branchId: BranchId,
  ): Promise<PersistedChunk[]> {
    const result = await this.pool.query<ChunkRow>(
      `SELECT ${CHUNK_COLUMNS}
       FROM chunks
       WHERE workspace_id = $1 AND origin_branch_id = $2
       ORDER BY idea_label`,
      [workspaceId, branchId],
    );
    return result.rows.map(chunkRowToPersistedChunk);
  }

  /**
   * Persists an `EdgeLineage`, appending any versions not yet stored and
   * marking the previously-current stored version's terminal state (e.g.
   * `active` -> `superseded`/`deactivated`) when a new version is appended
   * alongside it. Safe to call repeatedly with the same or a
   * further-evolved lineage — never deletes a row, and never changes a
   * stored row's state *without* also appending the new version that
   * transition produced, so history remains fully readable (technical spec
   * §"Edge lineage persistence": "must never be physically or logically
   * deleted"; deactivation "must supersede it with a new edge version").
   *
   * Every domain transition (`supersedeEdge`, `deactivateEdge`) appends a
   * new version rather than mutating the current one in place, so a
   * legitimate lineage's version count only ever grows strictly between
   * calls whenever its terminal state changes. If the caller's lineage
   * would change an already-stored version's state with *no* new version
   * appended alongside it, that lineage could never have been produced by
   * the current domain and is rejected with `EdgeLineageError`
   * (`lineage-violation`) rather than silently rewriting stored history in
   * place.
   *
   * Always operates on the latest existing `lineage_seq` generation for this
   * identity (or generation 1 if none exists yet). Starting a *new*
   * generation — as happens when a relationship's type is replaced — is the
   * exclusive responsibility of `replaceEdgeRelationshipType`; this method
   * never creates one on its own.
   *
   * Only forward progress is accepted: appending beyond what is stored, or
   * transitioning the current stored version's state from `active` to
   * `superseded`/`deactivated` while also appending at least one new
   * version. A lineage that is behind what is already stored (fewer
   * versions than persisted), that would move a non-`active` stored version
   * backward (e.g. `deactivated` -> `active`), or that would change a
   * stored version's terminal state without appending a new version, is
   * rejected with `EdgeLineageError` (`invalid-state-transition` /
   * `lineage-violation`) rather than silently overwriting stored history
   * with a stale in-memory value.
   *
   * A plain domain `EdgeLineage` value carries no generation tag (story
   * S03's `lineage_seq` is a persistence-only concept), so once an identity
   * has more than one generation on record (it has been retyped away and
   * back at least once via `replaceEdgeRelationshipType`), this method
   * cannot safely tell which generation a caller's in-memory lineage was
   * read from — two different generations can coincidentally have the same
   * version count and terminal state. Rather than risk silently corrupting
   * the wrong generation, this method rejects with `EdgeLineageError`
   * (`lineage-violation`) whenever the identity has multiple generations;
   * callers must use `findEdgeLineageRecord` to read the current generation
   * and `replaceEdgeRelationshipType` to change types.
   */
  async saveEdgeLineage(lineage: EdgeLineage): Promise<void> {
    const client = await this.pool.connect();
    try {
      await client.query('BEGIN');
      await this.saveEdgeLineageOnClient(client, lineage);
      await client.query('COMMIT');
    } catch (error) {
      await client.query('ROLLBACK');
      throw mapPersistenceError(error);
    } finally {
      client.release();
    }
  }

  /**
   * Client-scoped core of `saveEdgeLineage` (story S07): identical
   * behavior and invariants (see `saveEdgeLineage`'s docs above), but
   * takes an externally-supplied `client` and does not open, commit, or
   * roll back a transaction itself — the caller owns the transaction
   * boundary. This lets a larger atomic operation (the merge transaction)
   * compose an edge-lineage write alongside chunk and chunk-artifact
   * writes on one Postgres session, rather than each opening its own
   * (which would defeat atomicity). `originBranchId`, when provided,
   * stamps it permanently on every newly-inserted version row only (story
   * S07, technical spec §"Pre-merge history reconstruction") — existing
   * stored rows, including one whose state is updated in place from
   * `active` to `superseded`/`deactivated` as part of an append, keep
   * whatever `origin_branch_id` they already had.
   *
   * Errors are thrown as-is (domain `EdgeLineageError`s, or raw `pg`
   * errors) so the caller decides how to roll back and map them; unlike
   * the public `saveEdgeLineage` wrapper, this method does not call
   * `mapPersistenceError` itself, since the caller may want to fold that
   * mapping into a larger merge-error-mapping story.
   */
  async saveEdgeLineageOnClient(
    client: PoolClient,
    lineage: EdgeLineage,
    originBranchId?: BranchId,
  ): Promise<void> {
    const first = lineage.versions[0];
    if (!first) {
      throw new EdgeLineageError(
        'lineage-violation',
        'cannot persist an edge lineage with zero versions',
      );
    }

    const generationCountResult = await client.query<{ count: string }>(
      `SELECT COUNT(DISTINCT lineage_seq) AS count FROM edge_versions
       WHERE workspace_id = $1 AND source_label = $2 AND target_label = $3
         AND relationship_type = $4`,
      [first.workspaceId, first.sourceLabel, first.targetLabel, first.relationshipType],
    );
    const generationCount = Number(generationCountResult.rows[0]?.count ?? '0');
    if (generationCount > 1) {
      throw new EdgeLineageError(
        'lineage-violation',
        `cannot save a generation-oblivious lineage for '${first.sourceLabel}' -[${first.relationshipType}]-> '${first.targetLabel}' in workspace '${first.workspaceId}': this identity has ${String(generationCount)} generations on record (it was replaced by a relationship-type change and changed back at least once); use findEdgeLineageRecord/replaceEdgeRelationshipType instead`,
      );
    }

    const existing = await client.query<{ lineage_seq: number; version: number; state: string }>(
      `SELECT lineage_seq, version, state FROM edge_versions
       WHERE workspace_id = $1 AND source_label = $2 AND target_label = $3
         AND relationship_type = $4
       ORDER BY lineage_seq DESC, version DESC
       LIMIT 1`,
      [
        first.workspaceId,
        first.sourceLabel,
        first.targetLabel,
        first.relationshipType,
      ],
    );
    const lineageSeq = existing.rows[0]?.lineage_seq ?? 1;
    const existingCount = existing.rows[0]?.version ?? 0;
    const existingState = existing.rows[0]?.state;

    if (existingCount > lineage.versions.length) {
      throw new EdgeLineageError(
        'lineage-violation',
        `cannot persist a lineage with ${String(lineage.versions.length)} version(s) when ${String(existingCount)} are already stored; the caller's lineage is behind stored history`,
      );
    }

    if (existingCount > 0) {
      const currentLineageVersion = lineage.versions[existingCount - 1];
      if (
        currentLineageVersion &&
        currentLineageVersion.state !== existingState
      ) {
        if (existingState !== 'active') {
          throw new EdgeLineageError(
            'invalid-state-transition',
            `cannot move stored edge version ${String(existingCount)} from '${String(existingState)}' to '${currentLineageVersion.state}'; only an 'active' stored version may transition`,
          );
        }
        if (existingCount === lineage.versions.length) {
          // The stored row's terminal state would change with no new
          // version appended alongside it — every domain transition
          // (`supersedeEdge`, `deactivateEdge`) appends a new version
          // rather than mutating the current one in place (technical
          // spec §"Edge lineage persistence"), so this can only mean the
          // caller's in-memory lineage was never a valid continuation of
          // stored history.
          throw new EdgeLineageError(
            'lineage-violation',
            `cannot persist a lineage that would change stored version ${String(existingCount)} from '${String(existingState)}' to '${currentLineageVersion.state}' without appending a new version; deactivation and supersession must always append`,
          );
        }
        // The caller's lineage appends further versions beyond this one
        // (e.g. it was superseded or deactivated), so mark the
        // previously-current stored row with its new terminal state —
        // this is not an in-place *replacement* of history, it is part of
        // recording the append: the old row becomes 'superseded' (or
        // 'deactivated') and a new row is inserted below.
        await client.query(
          `UPDATE edge_versions SET state = $1
           WHERE workspace_id = $2 AND source_label = $3 AND target_label = $4
             AND relationship_type = $5 AND lineage_seq = $6 AND version = $7`,
          [
            currentLineageVersion.state,
            first.workspaceId,
            first.sourceLabel,
            first.targetLabel,
            first.relationshipType,
            lineageSeq,
            existingCount,
          ],
        );
      }
    }

    for (let v = existingCount; v < lineage.versions.length; v++) {
      const version = lineage.versions[v];
      if (!version) {
        continue;
      }
      await client.query(
        `INSERT INTO edge_versions (
           workspace_id, source_label, target_label, relationship_type, lineage_seq, version, state, origin_branch_id
         ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
        [
          version.workspaceId,
          version.sourceLabel,
          version.targetLabel,
          version.relationshipType,
          lineageSeq,
          v + 1,
          version.state,
          originBranchId ?? null,
        ],
      );
    }
  }

  async findEdgeLineage(
    workspaceId: WorkspaceId,
    sourceLabel: IdeaLabel,
    targetLabel: IdeaLabel,
    relationshipType: RelationshipType,
  ): Promise<EdgeLineage | undefined> {
    const record = await this.findEdgeLineageRecord(
      workspaceId,
      sourceLabel,
      targetLabel,
      relationshipType,
    );
    return record?.lineage;
  }

  /**
   * Like `findEdgeLineage`, but resolves the latest `lineage_seq` generation
   * for this relationship type and also surfaces a precise successor
   * pointer (relationship type + generation) if that generation's terminal
   * version was replaced by a relationship-type change (story S03).
   */
  async findEdgeLineageRecord(
    workspaceId: WorkspaceId,
    sourceLabel: IdeaLabel,
    targetLabel: IdeaLabel,
    relationshipType: RelationshipType,
  ): Promise<EdgeLineageRecord | undefined> {
    const result = await this.pool.query<EdgeVersionRow>(
      `SELECT ${EDGE_VERSION_COLUMNS}
       FROM edge_versions
       WHERE workspace_id = $1 AND source_label = $2 AND target_label = $3
         AND relationship_type = $4
         AND lineage_seq = (
           SELECT MAX(lineage_seq) FROM edge_versions
           WHERE workspace_id = $1 AND source_label = $2 AND target_label = $3
             AND relationship_type = $4
         )
       ORDER BY version ASC`,
      [workspaceId, sourceLabel, targetLabel, relationshipType],
    );
    return result.rows.length > 0 ? rowsToEdgeLineageRecord(result.rows) : undefined;
  }

  /**
   * Like `findEdgeLineageRecord`, but takes an externally-supplied `client`
   * so a caller composing a larger transaction (story S07's merge) reads
   * the current mainline lineage from its own transaction snapshot, not a
   * separate pool connection.
   */
  async findEdgeLineageRecordOnClient(
    client: PoolClient,
    workspaceId: WorkspaceId,
    sourceLabel: IdeaLabel,
    targetLabel: IdeaLabel,
    relationshipType: RelationshipType,
    options?: { readonly forUpdate?: boolean },
  ): Promise<EdgeLineageRecord | undefined> {
    const lockClause = options?.forUpdate ? ' FOR UPDATE' : '';
    const result = await client.query<EdgeVersionRow>(
      `SELECT ${EDGE_VERSION_COLUMNS}
       FROM edge_versions
       WHERE workspace_id = $1 AND source_label = $2 AND target_label = $3
         AND relationship_type = $4
         AND lineage_seq = (
           SELECT MAX(lineage_seq) FROM edge_versions
           WHERE workspace_id = $1 AND source_label = $2 AND target_label = $3
             AND relationship_type = $4
         )
       ORDER BY version ASC${lockClause}`,
      [workspaceId, sourceLabel, targetLabel, relationshipType],
    );
    return result.rows.length > 0 ? rowsToEdgeLineageRecord(result.rows) : undefined;
  }

  /**
   * Lists every distinct edge identity with at least one version stamped
   * `origin_branch_id = branchId` (story S07, AC3) — i.e. every
   * relationship a specific branch's merge created or changed, traceable
   * even after later, unrelated merges (each version row is permanent).
   */
  async listEdgeIdentitiesByOriginBranch(
    workspaceId: WorkspaceId,
    branchId: BranchId,
  ): Promise<
    { sourceLabel: IdeaLabel; targetLabel: IdeaLabel; relationshipType: RelationshipType }[]
  > {
    const result = await this.pool.query<{
      source_label: string;
      target_label: string;
      relationship_type: string;
    }>(
      `SELECT DISTINCT source_label, target_label, relationship_type
       FROM edge_versions
       WHERE workspace_id = $1 AND origin_branch_id = $2
       ORDER BY source_label, target_label, relationship_type`,
      [workspaceId, branchId],
    );
    return result.rows.map((row) => ({
      sourceLabel: row.source_label as IdeaLabel,
      targetLabel: row.target_label as IdeaLabel,
      relationshipType: row.relationship_type as RelationshipType,
    }));
  }

  /**
   * Reverse lookup for backward traceability (story S03, AC1/AC3): finds the
   * lineage generation (of any relationship type) whose successor pointer
   * names `(relationshipType, lineageSeq)` — i.e. "what was this
   * relationship before its type was last changed?" Returns `undefined` if
   * this generation was not produced by a type change (for example, it is
   * the original lineage for these endpoints, or it followed a same-type
   * supersession rather than a retype).
   *
   * The technical specification only requires the *old* row to reference
   * the new one (a forward pointer); this method derives the reverse
   * direction with a query rather than a stored backward pointer, so a
   * caller can still walk from the current relationship all the way back
   * through every type change it has undergone.
   */
  async findPredecessorEdgeLineageRecord(
    workspaceId: WorkspaceId,
    sourceLabel: IdeaLabel,
    targetLabel: IdeaLabel,
    relationshipType: RelationshipType,
    lineageSeq: number,
  ): Promise<EdgeLineageRecord | undefined> {
    const predecessorIdentity = await this.pool.query<{
      relationship_type: string;
      lineage_seq: number;
    }>(
      `SELECT DISTINCT relationship_type, lineage_seq
       FROM edge_versions
       WHERE workspace_id = $1 AND source_label = $2 AND target_label = $3
         AND succeeded_by_relationship_type = $4 AND succeeded_by_lineage_seq = $5`,
      [workspaceId, sourceLabel, targetLabel, relationshipType, lineageSeq],
    );
    const predecessor = predecessorIdentity.rows[0];
    if (!predecessor) {
      return undefined;
    }

    const result = await this.pool.query<EdgeVersionRow>(
      `SELECT ${EDGE_VERSION_COLUMNS}
       FROM edge_versions
       WHERE workspace_id = $1 AND source_label = $2 AND target_label = $3
         AND relationship_type = $4 AND lineage_seq = $5
       ORDER BY version ASC`,
      [
        workspaceId,
        sourceLabel,
        targetLabel,
        predecessor.relationship_type,
        predecessor.lineage_seq,
      ],
    );
    return result.rows.length > 0 ? rowsToEdgeLineageRecord(result.rows) : undefined;
  }

  /**
   * Atomically replaces the relationship type between two ideas (story S03,
   * technical spec §"Edge lineage persistence"): the old type's latest
   * generation is closed by appending a new `deactivated` version — never
   * deleted, and never flipped to `deactivated` in place — tagged with a
   * precise successor pointer, and a brand-new generation is created for
   * the new type with a fresh active version. This mirrors the domain's
   * `deactivateEdge` append behavior, so a closed-out old generation reads
   * back identically whether it was closed by a plain deactivation or by a
   * relationship-type change. Locks the old lineage's current row
   * (`SELECT ... FOR UPDATE`) for the duration of the transaction so a
   * concurrent retype of the same lineage is serialized rather than racing
   * to produce two successors.
   *
   * Throws `EdgeLineageError` (`invalid-state-transition`) if
   * `oldRelationshipType === newRelationshipType` (same-type evolution must
   * go through `saveEdgeLineage`/`supersedeEdge`, not this operation), if no
   * relationship of `oldRelationshipType` exists between these endpoints, or
   * if its latest generation is not currently `active`.
   * Throws `EdgeLineageError` (`duplicate-active-relationship`) if an active
   * lineage already exists for the new type between these endpoints — the
   * whole operation is rolled back and the old lineage is left untouched.
   */
  async replaceEdgeRelationshipType(
    workspaceId: WorkspaceId,
    sourceLabel: IdeaLabel,
    targetLabel: IdeaLabel,
    oldRelationshipType: RelationshipType,
    newRelationshipType: RelationshipType,
  ): Promise<{ oldLineage: EdgeLineage; newLineage: EdgeLineage }> {
    if (oldRelationshipType === newRelationshipType) {
      throw new EdgeLineageError(
        'invalid-state-transition',
        'cannot replace a relationship type with itself; use saveEdgeLineage for same-type supersession',
      );
    }

    const client = await this.pool.connect();
    try {
      await client.query('BEGIN');

      const oldSeqResult = await client.query<{ seq: number }>(
        `SELECT COALESCE(MAX(lineage_seq), 0) AS seq FROM edge_versions
         WHERE workspace_id = $1 AND source_label = $2 AND target_label = $3
           AND relationship_type = $4`,
        [workspaceId, sourceLabel, targetLabel, oldRelationshipType],
      );
      const oldSeq = Number(oldSeqResult.rows[0]?.seq ?? 0);
      if (oldSeq === 0) {
        throw new EdgeLineageError(
          'invalid-state-transition',
          `no relationship of type '${oldRelationshipType}' exists between '${sourceLabel}' and '${targetLabel}' in workspace '${workspaceId}'`,
        );
      }

      // Lock the old lineage's current row so a concurrent retype attempt
      // blocks here rather than racing to produce two successor pointers.
      const currentRowResult = await client.query<EdgeVersionRow>(
        `SELECT ${EDGE_VERSION_COLUMNS}
         FROM edge_versions
         WHERE workspace_id = $1 AND source_label = $2 AND target_label = $3
           AND relationship_type = $4 AND lineage_seq = $5
         ORDER BY version DESC
         LIMIT 1
         FOR UPDATE`,
        [workspaceId, sourceLabel, targetLabel, oldRelationshipType, oldSeq],
      );
      const currentRow = currentRowResult.rows[0];
      if (!currentRow || currentRow.state !== 'active') {
        throw new EdgeLineageError(
          'invalid-state-transition',
          `cannot replace the relationship type of a lineage that is '${currentRow?.state ?? 'missing'}'; only an active lineage may be retyped`,
        );
      }

      const allOldRowsResult = await client.query<EdgeVersionRow>(
        `SELECT ${EDGE_VERSION_COLUMNS}
         FROM edge_versions
         WHERE workspace_id = $1 AND source_label = $2 AND target_label = $3
           AND relationship_type = $4 AND lineage_seq = $5
         ORDER BY version ASC`,
        [workspaceId, sourceLabel, targetLabel, oldRelationshipType, oldSeq],
      );

      // Serialize concurrent retypes that target the same new identity (even
      // from different old relationship types) so `newSeq` allocation and
      // the active-row insert below cannot race against each other; without
      // this, two concurrent retypes could compute the same `newSeq` and one
      // would fail on the primary key instead of the intended
      // `duplicate-active-relationship` unique-index violation.
      await client.query('SELECT pg_advisory_xact_lock(hashtext($1))', [
        `edge-versions:${workspaceId}|${sourceLabel}|${targetLabel}|${newRelationshipType}`,
      ]);

      const newSeqResult = await client.query<{ seq: number }>(
        `SELECT COALESCE(MAX(lineage_seq), 0) AS seq FROM edge_versions
         WHERE workspace_id = $1 AND source_label = $2 AND target_label = $3
           AND relationship_type = $4`,
        [workspaceId, sourceLabel, targetLabel, newRelationshipType],
      );
      const newSeq = Number(newSeqResult.rows[0]?.seq ?? 0) + 1;

      // Close out the old generation by appending a new `deactivated`
      // version — mirroring plain deactivation (`deactivateEdge`) rather
      // than flipping the current row's state in place — so a lineage's
      // shape on disk never depends on *why* it was closed. The successor
      // pointer lives on this newly-appended terminal row.
      await client.query(
        `UPDATE edge_versions SET state = 'superseded'
         WHERE workspace_id = $1 AND source_label = $2 AND target_label = $3
           AND relationship_type = $4 AND lineage_seq = $5 AND version = $6`,
        [
          workspaceId,
          sourceLabel,
          targetLabel,
          oldRelationshipType,
          oldSeq,
          currentRow.version,
        ],
      );

      await client.query(
        `INSERT INTO edge_versions (
           workspace_id, source_label, target_label, relationship_type, lineage_seq, version, state,
           succeeded_by_relationship_type, succeeded_by_lineage_seq
         ) VALUES ($1, $2, $3, $4, $5, $6, 'deactivated', $7, $8)`,
        [
          workspaceId,
          sourceLabel,
          targetLabel,
          oldRelationshipType,
          oldSeq,
          currentRow.version + 1,
          newRelationshipType,
          newSeq,
        ],
      );

      await client.query(
        `INSERT INTO edge_versions (
           workspace_id, source_label, target_label, relationship_type, lineage_seq, version, state
         ) VALUES ($1, $2, $3, $4, $5, 1, 'active')`,
        [workspaceId, sourceLabel, targetLabel, newRelationshipType, newSeq],
      );

      await client.query('COMMIT');

      const oldLineage = rowsToEdgeLineage([
        ...allOldRowsResult.rows.slice(0, -1),
        { ...currentRow, state: 'superseded' },
        {
          ...currentRow,
          version: currentRow.version + 1,
          state: 'deactivated',
          succeeded_by_relationship_type: newRelationshipType,
          succeeded_by_lineage_seq: newSeq,
        },
      ]);
      const newLineage = createEdge(workspaceId, sourceLabel, targetLabel, newRelationshipType);
      return { oldLineage, newLineage };
    } catch (error) {
      await client.query('ROLLBACK');
      throw mapPersistenceError(error);
    } finally {
      client.release();
    }
  }

  async listEdgeLineages(workspaceId: WorkspaceId): Promise<EdgeLineage[]> {
    const result = await this.pool.query<EdgeVersionRow>(
      `SELECT ${EDGE_VERSION_COLUMNS}
       FROM edge_versions
       WHERE workspace_id = $1
       ORDER BY source_label, target_label, relationship_type, lineage_seq ASC, version ASC`,
      [workspaceId],
    );

    // Group by full generation identity first (source, target, type,
    // lineage_seq), then keep only the latest generation per
    // (source, target, type) triple — earlier generations of a type that
    // was itself later retyped away are history, reachable via
    // `findEdgeLineageRecord`/`findPredecessorEdgeLineageRecord`, not via
    // this "current state of the world" listing.
    const byGeneration = new Map<string, EdgeVersionRow[]>();
    for (const row of result.rows) {
      const key = `${row.source_label}\u0000${row.target_label}\u0000${row.relationship_type}\u0000${String(row.lineage_seq)}`;
      const group = byGeneration.get(key);
      if (group) {
        group.push(row);
      } else {
        byGeneration.set(key, [row]);
      }
    }

    const latestByIdentity = new Map<string, EdgeVersionRow[]>();
    for (const rows of byGeneration.values()) {
      const first = rows[0];
      if (!first) {
        continue;
      }
      const identityKey = `${first.source_label}\u0000${first.target_label}\u0000${first.relationship_type}`;
      const existing = latestByIdentity.get(identityKey);
      if (!existing || first.lineage_seq > existing[0]!.lineage_seq) {
        latestByIdentity.set(identityKey, rows);
      }
    }

    return Array.from(latestByIdentity.values()).map(rowsToEdgeLineage);
  }
}
