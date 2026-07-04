/**
 * Chunk-artifact association lineage domain module: append-only versioning
 * of the link between an idea chunk and a supporting artifact, scoped either
 * to mainline or to a single branch.
 *
 * Sources of authority:
 * - Story:          docs/specifications/feature-02-postgres-persistence/stories/S05-artifact-associations-traceable-through-review.md
 * - Technical spec: docs/specifications/feature-02-postgres-persistence/technical-specification.md
 *                   §"Chunk-artifact association lifecycle", §"Pre-merge history reconstruction",
 *                   §"Tenant isolation", §"Required domain error categories"
 * - Constitution:   docs/constitution.md (Principle IV — Rich Domain Models)
 * - Meridian (verified live against workspace dbb786ac-1d61-41c9-a46a-2c279dd50cc3):
 *   IDEA-60, IDEA-62, IDEA-64
 *
 * Design notes:
 * - An association's identity is its (workspaceId, chunkLabel, artifactId,
 *   branchId) tuple. `branchId` is `undefined` for the mainline scope and a
 *   `BranchId` for a branch's own shadow lineage — IDEA-62: "branch_id ...
 *   to allow branches to version associations under a delta-based model
 *   without affecting the mainline." This identity is fixed for the lifetime
 *   of an `AssociationLineage`; no transition ever rewrites it.
 * - `AssociationLineage` is an opaque, branded, frozen aggregate
 *   constructible only through `createAssociation` and
 *   `deactivateAssociation`, mirroring `EdgeLineage` in `edge-lineage.ts`.
 * - Every transition returns a *new* `AssociationLineage` value; no prior
 *   version is ever deleted or overwritten in place (append-only), matching
 *   the edge-lineage invariant: a lineage's first version can only be
 *   `active`, and `deactivated` can only appear as a lineage's *last*
 *   version. A branch deactivating a mainline-only association gets its own
 *   two-version shadow lineage (`active` then `deactivated`) rather than a
 *   lone `deactivated` row, for exactly this reason.
 * - `originBranchId` records which branch first created a lineage, and is
 *   copied unchanged onto every later version in that lineage (technical
 *   spec §"Pre-merge history reconstruction" / `IDEA-69`): a later merge
 *   story may clear a promoted row's branch scope, but must never lose
 *   this provenance. A mainline-created lineage has no `originBranchId`.
 */

import type { ArtifactAssociationState } from './types/index.js';
import type { ArtifactId, BranchId, IdeaLabel, WorkspaceId } from './types/index.js';
import { ArtifactAssociationError } from './types/index.js';

export type { ArtifactAssociationState };
export { ArtifactAssociationError };
export type { ArtifactAssociationErrorCode } from './types/index.js';

/**
 * A single version of a chunk-artifact association: the association's
 * identity (workspace + chunk label + artifact + branch scope) plus the
 * lifecycle state that version held.
 */
export interface ArtifactAssociationVersion {
  readonly workspaceId: WorkspaceId;
  readonly chunkLabel: IdeaLabel;
  readonly artifactId: ArtifactId;
  /** `undefined` = mainline scope; a `BranchId` = that branch's own shadow lineage. */
  readonly branchId?: BranchId;
  /** The branch that first created this lineage, preserved on every version. `undefined` for a mainline-originated lineage. */
  readonly originBranchId?: BranchId;
  readonly state: ArtifactAssociationState;
}

interface AssociationIdentity {
  readonly workspaceId: WorkspaceId;
  readonly chunkLabel: IdeaLabel;
  readonly artifactId: ArtifactId;
  readonly branchId: BranchId | undefined;
}

const _lineageBrand: unique symbol = Symbol('ArtifactAssociationLineage');

/**
 * Opaque, immutable lineage of a single chunk-artifact association: every
 * version it has ever held, oldest first, with the last element being the
 * current version. Constructible only via `createAssociation` and
 * `deactivateAssociation`.
 *
 * AC3: "A stakeholder can tell the current status of an artifact association
 * and see its prior associations rather than having history disappear when
 * the association changes."
 */
export type ArtifactAssociationLineage = {
  readonly versions: readonly ArtifactAssociationVersion[];
  readonly [_lineageBrand]: never;
};

function identityOf(version: ArtifactAssociationVersion): AssociationIdentity {
  return {
    workspaceId: version.workspaceId,
    chunkLabel: version.chunkLabel,
    artifactId: version.artifactId,
    branchId: version.branchId,
  };
}

function sameIdentity(a: AssociationIdentity, b: AssociationIdentity): boolean {
  return (
    a.workspaceId === b.workspaceId &&
    a.chunkLabel === b.chunkLabel &&
    a.artifactId === b.artifactId &&
    a.branchId === b.branchId
  );
}

function lineageOf(
  versions: readonly ArtifactAssociationVersion[],
): ArtifactAssociationLineage {
  return Object.freeze({
    versions: Object.freeze(versions.map((v) => Object.freeze({ ...v }))),
  }) as ArtifactAssociationLineage;
}

/**
 * Returns the current (most recent) version of an association lineage.
 *
 * AC3: "A stakeholder can tell the current status of an artifact
 * association."
 */
export function currentAssociationVersion(
  lineage: ArtifactAssociationLineage,
): ArtifactAssociationVersion {
  const last = lineage.versions[lineage.versions.length - 1];
  if (!last) {
    throw new ArtifactAssociationError(
      'lineage-violation',
      'artifact association lineage has no versions',
    );
  }
  return last;
}

/**
 * Creates a new chunk-artifact association lineage with a single active
 * version, scoped either to mainline (`branchId` omitted) or to a single
 * branch's own shadow lineage.
 *
 * AC1: "A stakeholder can trace which supporting artifact is associated
 * with an idea while that association is still under branch review" — a
 * branch-scoped lineage is exactly this branch-review-time association.
 *
 * `originBranchId` defaults to `branchId` when omitted, so a branch-created
 * association is provenance-tagged to itself from its first version
 * (technical spec §"Pre-merge history reconstruction").
 */
export function createAssociation(
  workspaceId: WorkspaceId,
  chunkLabel: IdeaLabel,
  artifactId: ArtifactId,
  branchId?: BranchId,
  originBranchId?: BranchId,
): ArtifactAssociationLineage {
  const resolvedOriginBranchId = originBranchId ?? branchId;
  const version: ArtifactAssociationVersion = {
    workspaceId,
    chunkLabel,
    artifactId,
    state: 'active',
    ...(branchId !== undefined ? { branchId } : {}),
    ...(resolvedOriginBranchId !== undefined ? { originBranchId: resolvedOriginBranchId } : {}),
  };
  return lineageOf([version]);
}

/**
 * Deactivates the current version of an association lineage, preserving all
 * prior versions.
 *
 * AC2: "A stakeholder can confirm that changes to an artifact association
 * made on a branch do not affect the mainline association until the branch
 * is merged" — deactivation never mutates another scope's lineage; it only
 * ever operates on the lineage passed in.
 * AC3: "... see its prior associations rather than having history disappear
 * when the association changes."
 *
 * Deactivation supersedes the current version rather than mutating it in
 * place: the current version becomes `superseded` and a new version — same
 * identity, state `deactivated` — is appended. This mirrors
 * `deactivateEdge` and guarantees a lineage's first version can only ever be
 * `active`; `deactivated` can only appear as a lineage's *last* version.
 *
 * Throws `ArtifactAssociationError` with code `invalid-state-transition` if
 * the lineage's current version is not `active`.
 */
export function deactivateAssociation(
  lineage: ArtifactAssociationLineage,
): ArtifactAssociationLineage {
  const current = currentAssociationVersion(lineage);
  if (current.state !== 'active') {
    throw new ArtifactAssociationError(
      'invalid-state-transition',
      `cannot deactivate a chunk-artifact association that is '${current.state}'; only an active association may be deactivated`,
    );
  }
  return lineageOf([
    ...lineage.versions.slice(0, -1),
    { ...current, state: 'superseded' },
    { ...current, state: 'deactivated' },
  ]);
}

export function isActiveAssociation(version: ArtifactAssociationVersion): boolean {
  return version.state === 'active';
}

export function isSupersededAssociation(version: ArtifactAssociationVersion): boolean {
  return version.state === 'superseded';
}

export function isDeactivatedAssociation(version: ArtifactAssociationVersion): boolean {
  return version.state === 'deactivated';
}

/**
 * Returns the full version history of an association lineage, newest first.
 *
 * AC3: "A stakeholder can ... see its prior associations rather than having
 * history disappear when the association changes."
 */
export function resolveAssociationHistory(
  lineage: ArtifactAssociationLineage,
): readonly ArtifactAssociationVersion[] {
  return [...lineage.versions].reverse();
}

/**
 * Asserts that a candidate association version does not conflict with any
 * existing lineage's current version for the same workspace, chunk label,
 * artifact, and scope (mainline or a specific branch).
 *
 * Mirrors `IDEA-64`'s mainline partial unique index
 * (`idx_chunk_artifacts_mainline`) at the domain level, extended to also
 * cover a single branch's own scope so a branch cannot accumulate two active
 * associations for the same chunk+artifact pair either.
 *
 * Throws `ArtifactAssociationError` with code `duplicate-active-relationship`
 * if `candidate` is active and an existing lineage in the same
 * workspace+scope already has an active current version for the same chunk
 * label and artifact.
 */
export function assertNoConflictingActiveAssociation(
  candidate: ArtifactAssociationVersion,
  existing: readonly ArtifactAssociationLineage[],
): void {
  if (!isActiveAssociation(candidate)) {
    return;
  }
  for (const lineage of existing) {
    const current = currentAssociationVersion(lineage);
    if (
      current.workspaceId === candidate.workspaceId &&
      isActiveAssociation(current) &&
      sameIdentity(identityOf(current), identityOf(candidate))
    ) {
      const scope = candidate.branchId ? `branch '${candidate.branchId}'` : 'mainline';
      throw new ArtifactAssociationError(
        'duplicate-active-relationship',
        `an active chunk-artifact association already exists for chunk '${candidate.chunkLabel}' and artifact '${candidate.artifactId}' in workspace '${candidate.workspaceId}' (${scope})`,
      );
    }
  }
}
