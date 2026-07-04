/**
 * Edge lineage domain module: label-based relationship edges, edge
 * determinism, and lineage-preserving supersession/deactivation.
 *
 * Sources of authority:
 * - Story:          docs/specifications/feature-01-core-domain-model/stories/S06-traceable-relationships.md
 * - Technical spec: docs/specifications/feature-01-core-domain-model/technical-specification.md
 *                   §"Logical edge endpoints", §"Edge determinism", §"Edge lineage",
 *                   §"Required lifecycle contracts — Edge", §"Required domain error categories"
 * - Constitution:   docs/constitution.md (Principle IV — Rich Domain Models)
 * - Meridian:        IDEA-36, IDEA-37, IDEA-38
 *
 * Story: S06 — Keep relationships traceable as ideas evolve.
 *
 * Design notes:
 * - A relationship's identity is its (workspaceId, sourceLabel, targetLabel,
 *   relationshipType) tuple. Technical spec §"Logical edge endpoints": "Edges
 *   identify chunks by logical idea labels ... without endpoint rewrites."
 *   This identity is fixed for the lifetime of an `EdgeLineage` — supersession
 *   never rewrites it, it only changes `state` and appends a new version.
 * - `EdgeLineage` is an opaque, branded, frozen aggregate constructible only
 *   through `createEdge`, `supersedeEdge`, and `deactivateEdge`. This mirrors
 *   the `ChunkLifecycleStatus` pattern in `chunk-lifecycle.ts` and prevents
 *   callers from forging invalid or cyclic version chains.
 * - Mainline edges are immutable (technical spec §"Edge lineage"): every
 *   transition returns a *new* `EdgeLineage` value; no prior version is ever
 *   deleted or overwritten in place.
 */

import type {
  EdgeState,
  IdeaLabel,
  RelationshipType,
  WorkspaceId,
} from './types/index.js';
import { EdgeLineageError } from './types/index.js';

export type { EdgeState };
export { EdgeLineageError };
export type { EdgeLineageErrorCode } from './types/index.js';

/**
 * A single version of a relationship edge: the relationship's identity
 * (workspace + source label + target label + relationship type) plus the
 * lifecycle state that version held.
 */
export interface RelationshipEdgeVersion {
  readonly workspaceId: WorkspaceId;
  readonly sourceLabel: IdeaLabel;
  readonly targetLabel: IdeaLabel;
  readonly relationshipType: RelationshipType;
  readonly state: EdgeState;
}

type EdgeIdentity = Pick<
  RelationshipEdgeVersion,
  'workspaceId' | 'sourceLabel' | 'targetLabel' | 'relationshipType'
>;

const _lineageBrand: unique symbol = Symbol('EdgeLineage');

/**
 * Opaque, immutable lineage of a single logical relationship: every version
 * the relationship has ever held, oldest first, with the last element being
 * the current version. Constructible only via `createEdge`, `supersedeEdge`,
 * and `deactivateEdge`.
 *
 * AC3: "A stakeholder can trace a replaced relationship back through its
 * prior versions." AC5: "... does not erase its history."
 */
export type EdgeLineage = {
  readonly versions: readonly RelationshipEdgeVersion[];
  readonly [_lineageBrand]: never;
};

function identityOf(version: RelationshipEdgeVersion): EdgeIdentity {
  return {
    workspaceId: version.workspaceId,
    sourceLabel: version.sourceLabel,
    targetLabel: version.targetLabel,
    relationshipType: version.relationshipType,
  };
}

function sameIdentity(a: EdgeIdentity, b: EdgeIdentity): boolean {
  return (
    a.workspaceId === b.workspaceId &&
    a.sourceLabel === b.sourceLabel &&
    a.targetLabel === b.targetLabel &&
    a.relationshipType === b.relationshipType
  );
}

function lineageOf(versions: readonly RelationshipEdgeVersion[]): EdgeLineage {
  return Object.freeze({
    versions: Object.freeze(versions.map((v) => Object.freeze({ ...v }))),
  }) as EdgeLineage;
}

/**
 * Returns the current (most recent) version of a relationship lineage.
 *
 * AC2: "A stakeholder can tell which relationship is currently active when
 * earlier relationships have been replaced."
 */
export function currentEdgeVersion(lineage: EdgeLineage): RelationshipEdgeVersion {
  const last = lineage.versions[lineage.versions.length - 1];
  if (!last) {
    throw new EdgeLineageError(
      'invalid-state-transition',
      'edge lineage has no versions',
    );
  }
  return last;
}

/**
 * Creates a new relationship edge lineage with a single active version.
 *
 * AC1: "A stakeholder can see relationships between ideas using stable idea
 * labels." Endpoints are `IdeaLabel` values, never storage-row identifiers
 * (technical spec §"Logical edge endpoints").
 */
export function createEdge(
  workspaceId: WorkspaceId,
  sourceLabel: IdeaLabel,
  targetLabel: IdeaLabel,
  relationshipType: RelationshipType,
): EdgeLineage {
  return lineageOf([
    { workspaceId, sourceLabel, targetLabel, relationshipType, state: 'active' },
  ]);
}

/**
 * Supersedes the current version of a relationship lineage with a new active
 * version, preserving all prior versions.
 *
 * Technical spec §"Edge lineage": "Mainline edges are immutable. Mainline
 * relationship changes must supersede prior edge versions and preserve
 * lineage; promoted edge history must not be deleted." The prior current
 * version becomes `superseded` (not deleted) and a new `active` version is
 * appended.
 *
 * `next` must carry the same (workspaceId, sourceLabel, targetLabel,
 * relationshipType) identity as the lineage — supersession changes lifecycle
 * state, not the relationship's logical endpoints (technical spec
 * §"Logical edge endpoints": "without endpoint rewrites").
 *
 * Throws `EdgeLineageError` with code `invalid-state-transition` if the
 * lineage's current version is not `active`.
 * Throws `EdgeLineageError` with code `lineage-violation` if `next`'s identity
 * does not match the lineage's fixed identity.
 */
export function supersedeEdge(lineage: EdgeLineage, next: EdgeIdentity): EdgeLineage {
  const current = currentEdgeVersion(lineage);
  if (current.state !== 'active') {
    throw new EdgeLineageError(
      'invalid-state-transition',
      `cannot supersede a relationship edge that is '${current.state}'; only an active edge may be superseded`,
    );
  }
  if (!sameIdentity(identityOf(current), next)) {
    throw new EdgeLineageError(
      'lineage-violation',
      'supersession must preserve the relationship\'s workspace, source label, target label, and relationship type; endpoint rewrites are not permitted',
    );
  }
  return lineageOf([
    ...lineage.versions.slice(0, -1),
    { ...current, state: 'superseded' },
    { ...next, state: 'active' },
  ]);
}

/**
 * Deactivates the current version of a relationship lineage, preserving all
 * prior versions.
 *
 * AC5: "A stakeholder can confirm that replacing or deactivating a mainline
 * relationship does not erase its history." Deactivation does not remove the
 * lineage's earlier versions; it only marks the current version `deactivated`.
 *
 * Deactivation supersedes the current version rather than mutating it in
 * place: the current version becomes `superseded` and a new version — same
 * identity, state `deactivated` — is appended. This mirrors `supersedeEdge`
 * and matches the "Edge lineage" lifecycle contract ("changes create
 * lineage-preserving supersession records") and feature-02's persistence
 * requirement that plain deactivation "must supersede it with a new edge
 * version, preserving an unbroken lineage chain" (clarified during
 * feature-02 story S03's implementation review). A consequence: a lineage's
 * first version can only ever be `active`; `deactivated` can only appear as
 * a lineage's *last* version.
 *
 * Throws `EdgeLineageError` with code `invalid-state-transition` if the
 * lineage's current version is not `active`.
 */
export function deactivateEdge(lineage: EdgeLineage): EdgeLineage {
  const current = currentEdgeVersion(lineage);
  if (current.state !== 'active') {
    throw new EdgeLineageError(
      'invalid-state-transition',
      `cannot deactivate a relationship edge that is '${current.state}'; only an active edge may be deactivated`,
    );
  }
  return lineageOf([
    ...lineage.versions.slice(0, -1),
    { ...current, state: 'superseded' },
    { ...current, state: 'deactivated' },
  ]);
}

export function isActiveEdge(version: RelationshipEdgeVersion): boolean {
  return version.state === 'active';
}

export function isSupersededEdge(version: RelationshipEdgeVersion): boolean {
  return version.state === 'superseded';
}

export function isDeactivatedEdge(version: RelationshipEdgeVersion): boolean {
  return version.state === 'deactivated';
}

/**
 * Returns the full version history of a relationship lineage, newest first.
 *
 * AC3: "A stakeholder can trace a replaced relationship back through its
 * prior versions."
 */
export function resolveLineage(
  lineage: EdgeLineage,
): readonly RelationshipEdgeVersion[] {
  return [...lineage.versions].reverse();
}

/**
 * Asserts that a candidate relationship version does not conflict with any
 * existing lineage's current version for the same workspace, source label,
 * target label, and relationship type.
 *
 * Technical spec §"Edge determinism": "After branch overrides and
 * deactivations are applied, a resolved graph view may contain at most one
 * active edge for the same source label, target label, and relationship
 * type." Determinism is scoped per workspace — lineages from other workspaces
 * are not compared (technical spec §"Workspace scoping": cross-workspace
 * edges "must not connect or resolve together").
 *
 * Throws `EdgeLineageError` with code `duplicate-active-relationship` if
 * `candidate` is active and an existing lineage in the same workspace already
 * has an active current version for the same source label, target label, and
 * relationship type.
 */
export function assertNoConflictingActiveEdge(
  candidate: RelationshipEdgeVersion,
  existing: readonly EdgeLineage[],
): void {
  if (!isActiveEdge(candidate)) {
    return;
  }
  for (const lineage of existing) {
    const current = currentEdgeVersion(lineage);
    if (current.workspaceId !== candidate.workspaceId) {
      continue;
    }
    if (isActiveEdge(current) && sameIdentity(identityOf(current), identityOf(candidate))) {
      throw new EdgeLineageError(
        'duplicate-active-relationship',
        `an active relationship already exists for '${candidate.sourceLabel}' -[${candidate.relationshipType}]-> '${candidate.targetLabel}' in workspace '${candidate.workspaceId}'`,
      );
    }
  }
}

/**
 * Asserts that a resolved set of relationship lineages contains at most one
 * active edge per (workspaceId, sourceLabel, targetLabel, relationshipType)
 * triple.
 *
 * AC4: "An implementation agent receives a relationship view that does not
 * contain conflicting active meanings for the same source idea, target idea,
 * and relationship type."
 *
 * A "resolved view" is itself workspace-scoped (technical spec
 * §"Workspace scoping": "every aggregate and graph operation is
 * tenant/workspace scoped ... cross-workspace ... edges ... must not connect
 * or resolve together"), so this function requires every lineage to share the
 * same workspace rather than silently ignoring mismatches — a caller
 * assembling a mixed-workspace "resolved view" is a programming error and
 * must be rejected explicitly.
 *
 * Throws `EdgeLineageError` with code `tenant-boundary-violation` if the
 * lineages do not all share the same workspace.
 * Throws `EdgeLineageError` with code `duplicate-active-relationship` if any
 * two lineages both have an active current version for the same source
 * label, target label, and relationship type.
 */
export function assertDeterministicEdgeSet(lineages: readonly EdgeLineage[]): void {
  const seen: RelationshipEdgeVersion[] = [];
  let workspace: WorkspaceId | undefined;
  for (const lineage of lineages) {
    const current = currentEdgeVersion(lineage);
    if (workspace === undefined) {
      workspace = current.workspaceId;
    } else if (current.workspaceId !== workspace) {
      throw new EdgeLineageError(
        'tenant-boundary-violation',
        `a resolved relationship view must be scoped to a single workspace; found both '${workspace}' and '${current.workspaceId}'`,
      );
    }
    if (!isActiveEdge(current)) {
      continue;
    }
    assertNoConflictingActiveEdge(current, seen.map((v) => lineageOf([v])));
    seen.push(current);
  }
}
