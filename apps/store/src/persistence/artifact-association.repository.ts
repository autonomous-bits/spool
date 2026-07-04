/**
 * Postgres-backed persistence adapter for chunk-artifact association
 * lineages (story S05).
 *
 * Sources of authority:
 * - Story S05: a stakeholder can trace which artifact is associated with an
 *   idea while under branch review (AC1); branch association changes never
 *   affect the mainline association until merge (AC2); current status and
 *   prior associations remain traceable (AC3).
 * - Technical spec §"Chunk-artifact association lifecycle" (`IDEA-62`):
 *   versioned per branch (active, superseded, deactivated), same
 *   delta-based model as chunks and edges.
 * - Technical spec §"Pre-merge history reconstruction" (`IDEA-69`):
 *   `originBranchId` provenance must survive independent of `branchId`.
 * - Technical spec §"Tenant isolation": every method takes a `workspaceId`
 *   and every SQL predicate filters on it explicitly.
 * - Technical spec §"Required domain error categories": no new categories —
 *   `not-found`, `invalid-state-transition`, `duplicate-active-relationship`
 *   only.
 * - Meridian (verified live against workspace
 *   `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`): `IDEA-60`, `IDEA-62`, `IDEA-64`.
 * - nestjs-security skill: parameterized queries only, never string
 *   concatenation.
 *
 * Concurrency: `createAssociation` and `deactivateAssociation` each begin
 * their transaction by taking a Postgres advisory transaction lock
 * (`pg_advisory_xact_lock`, auto-released at COMMIT/ROLLBACK) keyed by the
 * full identity `(workspaceId, chunkLabel, artifactId, branchId)`, so two
 * concurrent calls for the *same* identity+scope — including two concurrent
 * `createAssociation` calls for a brand-new identity, where there is no
 * existing row to `SELECT ... FOR UPDATE` — are fully serialized rather than
 * racing. The `idx_chunk_artifacts_mainline_version` /
 * `idx_chunk_artifacts_branch_version` / `idx_chunk_artifacts_mainline` /
 * `idx_chunk_artifacts_branch_active` partial unique indexes (schema.ts)
 * back this up at the database level for any race that would otherwise slip
 * past the advisory lock (e.g. a lock-key hash collision, astronomically
 * unlikely but not impossible).
 */

import { createHash } from 'node:crypto';
import { Inject, Injectable } from '@nestjs/common';
import type { Pool, PoolClient } from 'pg';
import {
  createAssociation,
  deactivateAssociation,
  currentAssociationVersion,
  ArtifactAssociationError,
  type ArtifactAssociationState,
} from '../domain/artifact-association-lineage.js';
import type {
  ArtifactId,
  BranchId,
  IdeaLabel,
  WorkspaceId,
} from '../domain/types/index.js';
import { PG_POOL } from './database-pool.provider.js';

export interface PersistedArtifactAssociation {
  readonly workspaceId: WorkspaceId;
  readonly chunkLabel: IdeaLabel;
  readonly artifactId: ArtifactId;
  /** `undefined` = mainline scope; a `BranchId` = that branch's own shadow lineage. */
  readonly branchId?: BranchId;
  /** The branch that first created this lineage; `undefined` for a mainline-originated lineage. */
  readonly originBranchId?: BranchId;
  readonly state: ArtifactAssociationState;
  readonly version: number;
  readonly createdAt: string;
  readonly updatedAt: string;
}

export interface NewArtifactAssociationInput {
  readonly workspaceId: WorkspaceId;
  readonly chunkLabel: IdeaLabel;
  readonly artifactId: ArtifactId;
  /** Omit for a mainline association; provide to create a branch's own shadow lineage. */
  readonly branchId?: BranchId;
}

interface ChunkArtifactRow {
  readonly workspace_id: string;
  readonly chunk_label: string;
  readonly artifact_id: string;
  readonly branch_id: string | null;
  readonly origin_branch_id: string | null;
  readonly version: number;
  readonly status: string;
  readonly created_at: string | Date;
  readonly updated_at: string | Date;
}

const ROW_COLUMNS = `workspace_id, chunk_label, artifact_id, branch_id, origin_branch_id,
       version, status, created_at, updated_at`;

const UNIQUE_VIOLATION = '23505';

function toIsoString(value: string | Date): string {
  return value instanceof Date ? value.toISOString() : value;
}

function rowToPersistedAssociation(row: ChunkArtifactRow): PersistedArtifactAssociation {
  const base: PersistedArtifactAssociation = {
    workspaceId: row.workspace_id as WorkspaceId,
    chunkLabel: row.chunk_label as IdeaLabel,
    artifactId: row.artifact_id as ArtifactId,
    state: row.status as ArtifactAssociationState,
    version: row.version,
    createdAt: toIsoString(row.created_at),
    updatedAt: toIsoString(row.updated_at),
  };
  return {
    ...base,
    ...(row.branch_id == null ? {} : { branchId: row.branch_id as BranchId }),
    ...(row.origin_branch_id == null
      ? {}
      : { originBranchId: row.origin_branch_id as BranchId }),
  };
}

function isUniqueViolation(error: unknown): boolean {
  return (
    typeof error === 'object' &&
    error !== null &&
    'code' in error &&
    (error as { code?: unknown }).code === UNIQUE_VIOLATION
  );
}

/**
 * Deterministic 63-bit signed integer key for `pg_advisory_xact_lock`,
 * derived from an association's full identity (workspace, chunk label,
 * artifact, branch scope). `pg_advisory_xact_lock` takes a `bigint`; a
 * SHA-256 digest's first 8 bytes, masked to 63 bits, gives an even
 * distribution while staying within Postgres `bigint` range (signed).
 */
function identityLockKey(
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

@Injectable()
export class ArtifactAssociationRepository {
  constructor(@Inject(PG_POOL) private readonly pool: Pool) {}

  /**
   * Creates a new chunk-artifact association lineage — mainline
   * (`branchId` omitted) or a branch's own shadow lineage (`branchId`
   * provided) — with a single `active` version (AC1).
   *
   * Runs inside a transaction serialized by an advisory lock on the target
   * identity+scope (see module docs), then checks whether any row already
   * exists for that identity+scope before inserting: a fresh identity gets
   * `version = 1`; an identity whose most recent version is still `active`
   * is rejected as a duplicate; an identity whose most recent version is
   * `superseded`/`deactivated` is rejected as an unsupported reactivation
   * (this domain has no reactivation transition, mirroring
   * `edge-lineage.ts`/`resolveEdgeDelta`'s precedent) rather than silently
   * starting a second, ambiguous `version = 1` lineage for the same
   * identity.
   *
   * Throws `ArtifactAssociationError` with code `duplicate-active-relationship`
   * if an active association already exists for the same identity in the
   * same scope.
   * Throws `ArtifactAssociationError` with code `invalid-state-transition`
   * if an inactive (superseded/deactivated) association already exists for
   * the same identity in the same scope — recreating/reactivating it is out
   * of this story's scope.
   */
  async createAssociation(
    input: NewArtifactAssociationInput,
  ): Promise<PersistedArtifactAssociation> {
    const lineage = createAssociation(
      input.workspaceId,
      input.chunkLabel,
      input.artifactId,
      input.branchId,
    );
    const version = currentAssociationVersion(lineage);
    const scope = input.branchId ? `branch '${input.branchId}'` : 'mainline';

    const client = await this.pool.connect();
    try {
      await client.query('BEGIN');
      await client.query('SELECT pg_advisory_xact_lock($1)', [
        identityLockKey(input.workspaceId, input.chunkLabel, input.artifactId, input.branchId),
      ]);

      const existing = await this.lockCurrentRow(
        client,
        input.workspaceId,
        input.chunkLabel,
        input.artifactId,
        input.branchId,
      );
      if (existing) {
        if (existing.status === 'active') {
          throw new ArtifactAssociationError(
            'duplicate-active-relationship',
            `an active chunk-artifact association already exists for chunk '${input.chunkLabel}' and artifact '${input.artifactId}' in workspace '${input.workspaceId}' (${scope})`,
          );
        }
        throw new ArtifactAssociationError(
          'invalid-state-transition',
          `a chunk-artifact association for chunk '${input.chunkLabel}' and artifact '${input.artifactId}' in workspace '${input.workspaceId}' (${scope}) already has history ending in '${existing.status}'; recreating/reactivating an inactive association is out of scope`,
        );
      }

      let result: { rows: ChunkArtifactRow[] };
      try {
        result = await client.query<ChunkArtifactRow>(
          `INSERT INTO chunk_artifacts (
             workspace_id, chunk_label, artifact_id, branch_id, origin_branch_id, version, status
           ) VALUES ($1, $2, $3, $4, $5, 1, $6)
           RETURNING ${ROW_COLUMNS}`,
          [
            version.workspaceId,
            version.chunkLabel,
            version.artifactId,
            version.branchId ?? null,
            version.originBranchId ?? null,
            version.state,
          ],
        );
      } catch (error) {
        if (isUniqueViolation(error)) {
          throw new ArtifactAssociationError(
            'duplicate-active-relationship',
            `an active chunk-artifact association already exists for chunk '${input.chunkLabel}' and artifact '${input.artifactId}' in workspace '${input.workspaceId}' (${scope})`,
          );
        }
        throw error;
      }

      await client.query('COMMIT');
      return rowToPersistedAssociation(result.rows[0]!);
    } catch (error) {
      await this.rollbackQuietly(client);
      throw error;
    } finally {
      client.release();
    }
  }

  /**
   * Deactivates the current version of an association lineage for the given
   * identity and scope, appending a new `deactivated` version rather than
   * mutating the current row in place (AC2, AC3).
   *
   * Locks the current-version row with `SELECT ... FOR UPDATE` before
   * appending, so two concurrent deactivations of the same lineage cannot
   * both succeed.
   *
   * If `branchId` is provided and the branch has no shadow lineage of its
   * own yet for this identity, but mainline has an active association,
   * this creates the branch's own two-version shadow lineage in one step
   * (`active` then `deactivated`) rather than throwing `not-found` —
   * mirroring `resolveEdgeDelta`'s "branch added, then deactivated, its own
   * edge — mainline never saw it" case, applied to a branch that instead
   * deactivates a mainline-visible identity. Mainline's own row is never
   * read for update or mutated by this path.
   *
   * Throws `ArtifactAssociationError` with code `not-found` if no
   * association exists for the identity in this scope, and (for a branch
   * scope) mainline has none active to shadow either.
   * Throws `ArtifactAssociationError` with code `invalid-state-transition`
   * if the current version in scope is not `active`.
   */
  async deactivateAssociation(
    workspaceId: WorkspaceId,
    chunkLabel: IdeaLabel,
    artifactId: ArtifactId,
    branchId?: BranchId,
  ): Promise<PersistedArtifactAssociation> {
    const scope = branchId ? `branch '${branchId}'` : 'mainline';
    const client = await this.pool.connect();
    try {
      await client.query('BEGIN');
      // Serializes on the target scope's identity: a concurrent
      // create/deactivate for this exact (workspace, chunk, artifact,
      // branch) cannot interleave with the seed-then-append sequence below.
      await client.query('SELECT pg_advisory_xact_lock($1)', [
        identityLockKey(workspaceId, chunkLabel, artifactId, branchId),
      ]);

      let currentRow = await this.lockCurrentRow(
        client,
        workspaceId,
        chunkLabel,
        artifactId,
        branchId,
      );

      if (!currentRow && branchId !== undefined) {
        // No branch-scoped row yet: fall back to mainline's current active
        // row (locked too, so a concurrent mainline change can't race this
        // read) to seed the branch's own shadow lineage. Mainline is only
        // read here, never mutated.
        const mainlineRow = await this.lockCurrentRow(
          client,
          workspaceId,
          chunkLabel,
          artifactId,
          undefined,
        );
        if (mainlineRow && mainlineRow.status === 'active') {
          try {
            const seeded = await client.query<ChunkArtifactRow>(
              `INSERT INTO chunk_artifacts (
                 workspace_id, chunk_label, artifact_id, branch_id, origin_branch_id, version, status
               ) VALUES ($1, $2, $3, $4, $5, 1, 'active')
               RETURNING ${ROW_COLUMNS}`,
              [workspaceId, chunkLabel, artifactId, branchId, branchId],
            );
            currentRow = seeded.rows[0];
          } catch (error) {
            if (isUniqueViolation(error)) {
              // The advisory lock above already serializes same-scope
              // callers, so this can only mean a stale in-flight
              // transaction from before this lock existed, or a hash
              // collision; map to a domain error either way rather than
              // leaking the raw `pg` error.
              throw new ArtifactAssociationError(
                'duplicate-active-relationship',
                `an active chunk-artifact association already exists for chunk '${chunkLabel}' and artifact '${artifactId}' in workspace '${workspaceId}' (${scope})`,
              );
            }
            throw error;
          }
        }
      }

      if (!currentRow) {
        throw new ArtifactAssociationError(
          'not-found',
          `no chunk-artifact association exists for chunk '${chunkLabel}' and artifact '${artifactId}' in workspace '${workspaceId}' (${scope})`,
        );
      }

      const lineage = createAssociationLineageFromCurrentRow(currentRow);
      const deactivated = deactivateAssociation(lineage);
      const [superseded, terminal] = deactivated.versions.slice(-2);

      await client.query(
        `UPDATE chunk_artifacts SET status = $1, updated_at = now() WHERE workspace_id = $2 AND chunk_label = $3 AND artifact_id = $4 AND branch_id IS NOT DISTINCT FROM $5 AND version = $6`,
        [superseded!.state, workspaceId, chunkLabel, artifactId, branchId ?? null, currentRow.version],
      );
      let inserted: { rows: ChunkArtifactRow[] };
      try {
        inserted = await client.query<ChunkArtifactRow>(
          `INSERT INTO chunk_artifacts (
             workspace_id, chunk_label, artifact_id, branch_id, origin_branch_id, version, status
           ) VALUES ($1, $2, $3, $4, $5, $6, $7)
           RETURNING ${ROW_COLUMNS}`,
          [
            workspaceId,
            chunkLabel,
            artifactId,
            branchId ?? null,
            currentRow.origin_branch_id,
            currentRow.version + 1,
            terminal!.state,
          ],
        );
      } catch (error) {
        if (isUniqueViolation(error)) {
          throw new ArtifactAssociationError(
            'duplicate-active-relationship',
            `an active chunk-artifact association already exists for chunk '${chunkLabel}' and artifact '${artifactId}' in workspace '${workspaceId}' (${scope})`,
          );
        }
        throw error;
      }

      await client.query('COMMIT');
      return rowToPersistedAssociation(inserted.rows[0]!);
    } catch (error) {
      await this.rollbackQuietly(client);
      throw error;
    } finally {
      client.release();
    }
  }

  /**
   * Returns the full version history of an association lineage for the
   * given identity and scope, newest first (AC3: "see its prior
   * associations rather than having history disappear").
   */
  async listAssociationHistory(
    workspaceId: WorkspaceId,
    chunkLabel: IdeaLabel,
    artifactId: ArtifactId,
    branchId?: BranchId,
  ): Promise<PersistedArtifactAssociation[]> {
    const result = await this.pool.query<ChunkArtifactRow>(
      `SELECT ${ROW_COLUMNS} FROM chunk_artifacts
       WHERE workspace_id = $1 AND chunk_label = $2 AND artifact_id = $3
         AND branch_id IS NOT DISTINCT FROM $4
       ORDER BY version DESC`,
      [workspaceId, chunkLabel, artifactId, branchId ?? null],
    );
    return result.rows.map(rowToPersistedAssociation);
  }

  /**
   * Lists every currently active mainline association for a workspace,
   * optionally filtered to a single chunk label.
   */
  async listMainlineActiveAssociations(
    workspaceId: WorkspaceId,
    chunkLabel?: IdeaLabel,
  ): Promise<PersistedArtifactAssociation[]> {
    const params: unknown[] = [workspaceId];
    let filter = 'workspace_id = $1 AND branch_id IS NULL AND status = \'active\'';
    if (chunkLabel !== undefined) {
      params.push(chunkLabel);
      filter += ' AND chunk_label = $2';
    }
    const result = await this.pool.query<ChunkArtifactRow>(
      `SELECT ${ROW_COLUMNS} FROM chunk_artifacts WHERE ${filter} ORDER BY chunk_label, artifact_id`,
      params,
    );
    return result.rows.map(rowToPersistedAssociation);
  }

  /**
   * Resolves a branch's view of chunk-artifact associations: mainline
   * active associations, overridden by this branch's own current row for
   * the same (chunk label, artifact) identity when one exists (AC1, AC2).
   * A branch row whose current state is `deactivated` hides the identity
   * from the resolved view entirely, even if mainline still has it active.
   * Never writes to, or otherwise mutates, any mainline row.
   */
  async resolveAssociationsForBranch(
    workspaceId: WorkspaceId,
    branchId: BranchId,
    chunkLabel?: IdeaLabel,
  ): Promise<PersistedArtifactAssociation[]> {
    const [mainline, branchRows] = await Promise.all([
      this.listMainlineActiveAssociations(workspaceId, chunkLabel),
      this.listCurrentBranchAssociations(workspaceId, branchId, chunkLabel),
    ]);

    const identityKey = (a: {
      chunkLabel: IdeaLabel;
      artifactId: ArtifactId;
    }): string => `${a.chunkLabel}\u0000${a.artifactId}`;

    const branchByIdentity = new Map(branchRows.map((row) => [identityKey(row), row]));
    const mainlineByIdentity = new Map(mainline.map((row) => [identityKey(row), row]));

    const identities = new Set<string>([
      ...mainlineByIdentity.keys(),
      ...branchByIdentity.keys(),
    ]);

    const resolved: PersistedArtifactAssociation[] = [];
    for (const key of identities) {
      const branchRow = branchByIdentity.get(key);
      if (branchRow) {
        if (branchRow.state === 'active') {
          resolved.push(branchRow);
        }
        // A branch row with state 'deactivated'/'superseded' hides the
        // identity from this branch's resolved view, mainline notwithstanding.
        continue;
      }
      const mainlineRow = mainlineByIdentity.get(key);
      if (mainlineRow) {
        resolved.push(mainlineRow);
      }
    }
    return resolved.sort(
      (a, b) => a.chunkLabel.localeCompare(b.chunkLabel) || a.artifactId.localeCompare(b.artifactId),
    );
  }

  /**
   * Lists this branch's own current-version rows (any state), one per
   * distinct (chunk label, artifact) identity — the branch-scoped
   * counterpart to `listMainlineActiveAssociations`, used by
   * `resolveAssociationsForBranch` to determine per-identity overrides.
   */
  private async listCurrentBranchAssociations(
    workspaceId: WorkspaceId,
    branchId: BranchId,
    chunkLabel?: IdeaLabel,
  ): Promise<PersistedArtifactAssociation[]> {
    const params: unknown[] = [workspaceId, branchId];
    let filter = 'workspace_id = $1 AND branch_id = $2';
    if (chunkLabel !== undefined) {
      params.push(chunkLabel);
      filter += ' AND chunk_label = $3';
    }
    const result = await this.pool.query<ChunkArtifactRow>(
      `SELECT DISTINCT ON (chunk_label, artifact_id) ${ROW_COLUMNS}
       FROM chunk_artifacts
       WHERE ${filter}
       ORDER BY chunk_label, artifact_id, version DESC`,
      params,
    );
    return result.rows.map(rowToPersistedAssociation);
  }

  private async lockCurrentRow(
    client: PoolClient,
    workspaceId: WorkspaceId,
    chunkLabel: IdeaLabel,
    artifactId: ArtifactId,
    branchId: BranchId | undefined,
  ): Promise<ChunkArtifactRow | undefined> {
    const result = await client.query<ChunkArtifactRow>(
      `SELECT ${ROW_COLUMNS} FROM chunk_artifacts
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

/**
 * Reconstructs a single-version `ArtifactAssociationLineage` from a
 * currently-stored row, suitable as input to the domain's
 * `deactivateAssociation`. Only the current version matters for computing
 * the next transition; prior history is untouched by this reconstruction.
 */
function createAssociationLineageFromCurrentRow(
  row: ChunkArtifactRow,
): ReturnType<typeof createAssociation> {
  if (row.status !== 'active') {
    throw new ArtifactAssociationError(
      'invalid-state-transition',
      `cannot deactivate a chunk-artifact association that is '${row.status}'; only an active association may be deactivated`,
    );
  }
  return createAssociation(
    row.workspace_id as WorkspaceId,
    row.chunk_label as IdeaLabel,
    row.artifact_id as ArtifactId,
    row.branch_id === null ? undefined : (row.branch_id as BranchId),
    row.origin_branch_id === null ? undefined : (row.origin_branch_id as BranchId),
  );
}
