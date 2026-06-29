/**
 * Branch lifecycle domain types, invariants, and transition functions.
 *
 * This module provides the domain-level tools for answering:
 * - "Which discipline owns this branch?" (BranchOwnership)
 * - "Can a graph write be performed on this branch?" (assertGraphWriteAllowed)
 * - "Is this actor authorised to submit this branch?" (assertSubmitDiscipline)
 * - "Where did this merged graph record come from?" (BranchGraphProvenance,
 *    MergeLineage, withBranchProvenance)
 *
 * Sources of authority:
 * - Story:          docs/specifications/feature-01-core-domain-model/stories/S03-discipline-owned-branches.md
 * - Technical spec: docs/specifications/feature-01-core-domain-model/technical-specification.md
 *                   §"Branch ownership", §"Branch graph view",
 *                   §"Required lifecycle contracts — Branch",
 *                   §"Protected operation contracts",
 *                   §"Required domain error categories"
 * - Constitution:   docs/constitution.md (Principle IV — Rich Domain Models)
 * - Meridian:       IDEA-17, IDEA-29, IDEA-35, IDEA-40, IDEA-41, IDEA-43
 *
 * Story: S03 — Keep branch work owned by one discipline.
 */

import { VocabularyValidationError, isHumanActor } from './types/index.js';
import type {
  ActorContext,
  BranchId,
  BranchState,
  Discipline,
  HumanActorContext,
  StakeholderId,
  WorkspaceId,
} from './types/index.js';

export type { BranchId, StakeholderId, WorkspaceId };

export type { ActorContext, BranchState, Discipline, HumanActorContext };

// ─── Domain error ─────────────────────────────────────────────────────────────

/**
 * All machine-readable error codes produced by branch lifecycle invariants.
 *
 * Technical spec §"Required domain error categories":
 * - `invalid-state-transition`       — attempted a disallowed lifecycle step
 * - `unauthorized-actor`             — delegated actor attempted a human-only op;
 *                                      also thrown as defense-in-depth where
 *                                      HumanActorContext is required
 * - `write-locked`                   — graph write attempted on a non-draft branch
 * - `discipline-boundary-violation`  — submit attempted by wrong-discipline actor
 * - `branch-isolation-violation`     — graph write targets a different discipline
 * - `tenant-boundary-violation`      — operation crosses workspace boundaries
 */
export type BranchLifecycleErrorCode =
  | 'write-locked'
  | 'unauthorized-actor'
  | 'discipline-boundary-violation'
  | 'invalid-state-transition'
  | 'branch-isolation-violation'
  | 'tenant-boundary-violation';

/**
 * Thrown when a branch lifecycle invariant is violated.
 *
 * The `code` property is stable and machine-readable. Adapters must map domain
 * failures by code, never by inspecting free-form message text.
 *
 * Technical spec §"Required domain error categories".
 */
export class BranchLifecycleError extends Error {
  override readonly name = 'BranchLifecycleError';

  constructor(
    readonly code: BranchLifecycleErrorCode,
    readonly reason: string,
  ) {
    super(reason);
  }
}

// ─── DivergencePoint ─────────────────────────────────────────────────────────

/**
 * Opaque marker recording the mainline state (a `diverged_at` ISO-8601
 * timestamp) at the moment a branch was created.
 *
 * The branch resolves mainline references from this point. Later mainline
 * changes do not silently mutate the branch view; they surface through
 * conflict checks or explicit catch-up.
 *
 * Meridian IDEA-41: "diverged_at timestamp".
 * Technical spec: §"Branch graph view".
 */
export type DivergencePoint = string & { readonly __tag: 'DivergencePoint' };

/**
 * Creates a DivergencePoint from an ISO-8601 timestamp string.
 * Leading/trailing whitespace is trimmed.
 *
 * Validation requires the string to begin with a YYYY-MM-DD pattern and be
 * parseable as a valid date (defense against JavaScript's permissive
 * `Date.parse` accepting non-ISO formats).
 *
 * Throws `VocabularyValidationError` if the value is empty, whitespace-only,
 * does not start with an ISO-8601 date prefix, or is not a valid date.
 */
export function divergencePoint(isoTimestamp: string): DivergencePoint {
  const trimmed = isoTimestamp.trim();
  if (!trimmed) {
    throw new VocabularyValidationError(
      'DivergencePoint',
      'timestamp cannot be empty or whitespace',
    );
  }
  if (!/^\d{4}-\d{2}-\d{2}/.test(trimmed)) {
    throw new VocabularyValidationError(
      'DivergencePoint',
      'timestamp must be a valid ISO-8601 date string (YYYY-MM-DD prefix required)',
    );
  }
  if (isNaN(Date.parse(trimmed))) {
    throw new VocabularyValidationError(
      'DivergencePoint',
      'timestamp must be a valid ISO-8601 date string',
    );
  }
  return trimmed as DivergencePoint;
}

// ─── BranchOwnership ─────────────────────────────────────────────────────────

// The brand symbol is module-private: callers cannot name it, so they cannot
// construct a BranchOwnership by structural assignment. The only valid
// entry point is the `branchOwnership` factory below.
const _ownershipBrand: unique symbol = Symbol('BranchOwnership');

/**
 * Immutable record of a branch's single-discipline ownership and divergence
 * point.
 *
 * Created once when the branch is initialized; the `discipline` field cannot
 * change for the branch's lifetime (technical spec §"Branch ownership";
 * Meridian IDEA-17: "owned by a single discipline … for its lifetime").
 *
 * The brand symbol makes this type opaque: only `branchOwnership()` can
 * produce a valid instance, preventing structural forgery.
 */
export type BranchOwnership = {
  readonly branchId: BranchId;
  readonly workspaceId: WorkspaceId;
  readonly discipline: Discipline;
  readonly divergedAt: DivergencePoint;
  readonly createdByStakeholderId: StakeholderId;
  readonly [_ownershipBrand]: never;
};

/**
 * Creates a frozen, validated BranchOwnership record.
 *
 * Technical spec: §"Branch ownership" — "records a divergence point when
 * created".
 */
export function branchOwnership(props: {
  readonly branchId: BranchId;
  readonly workspaceId: WorkspaceId;
  readonly discipline: Discipline;
  readonly divergedAt: DivergencePoint;
  readonly createdByStakeholderId: StakeholderId;
}): BranchOwnership {
  return Object.freeze({ ...props }) as BranchOwnership;
}

// ─── MergeLineage ─────────────────────────────────────────────────────────────

/**
 * Branch-level lineage record produced when a branch is merged into the
 * mainline.
 *
 * Retains full branch provenance (branchId, discipline, divergedAt) alongside
 * the merge event metadata (mergedAt, mergedByStakeholderId).
 *
 * Meridian IDEA-29: "Merged branches are queryable by lineage, enabling
 * traceability from any mainline idea back to the branch that introduced it."
 *
 * AC5: A stakeholder can trace merged work back to the branch that
 * introduced it.
 */
export interface MergeLineage {
  readonly branchId: BranchId;
  readonly workspaceId: WorkspaceId;
  readonly discipline: Discipline;
  readonly divergedAt: DivergencePoint;
  readonly mergedAt: string;
  readonly mergedByStakeholderId: StakeholderId;
}

/**
 * Creates a frozen MergeLineage from a BranchOwnership and merge actor.
 *
 * The `mergedAt` string must be a valid ISO-8601 timestamp (same validation
 * as DivergencePoint). It must also be chronologically at or after
 * `ownership.divergedAt` — a merge cannot precede the branch's divergence.
 *
 * Throws `VocabularyValidationError` if `mergedAt` is invalid.
 * Throws `BranchLifecycleError('invalid-state-transition')` if `mergedAt`
 * is before `divergedAt`.
 */
export function mergeLineage(
  ownership: BranchOwnership,
  mergedAt: string,
  _actor: HumanActorContext,
): MergeLineage {
  const trimmedMergedAt = mergedAt.trim();
  if (!trimmedMergedAt || !/^\d{4}-\d{2}-\d{2}/.test(trimmedMergedAt) || isNaN(Date.parse(trimmedMergedAt))) {
    throw new VocabularyValidationError(
      'MergeLineage.mergedAt',
      'mergedAt must be a valid ISO-8601 date string',
    );
  }
  if (Date.parse(trimmedMergedAt) < Date.parse(ownership.divergedAt)) {
    throw new BranchLifecycleError(
      'invalid-state-transition',
      `mergedAt (${trimmedMergedAt}) cannot be before divergedAt (${ownership.divergedAt})`,
    );
  }
  return Object.freeze({
    branchId: ownership.branchId,
    workspaceId: ownership.workspaceId,
    discipline: ownership.discipline,
    divergedAt: ownership.divergedAt,
    mergedAt: trimmedMergedAt,
    mergedByStakeholderId: _actor.stakeholderId,
  });
}

// ─── BranchSubmittedRecord ────────────────────────────────────────────────────

/**
 * Accountability record produced when a branch is submitted by a human
 * stakeholder.
 *
 * Records the branch identity, workspace, submitting stakeholder, and
 * submission timestamp, so a stakeholder can always tell which human is
 * accountable for the decision.
 *
 * The record is distinct from the state transition: `submitBranch` asserts the
 * transition is allowed; this factory creates the provenance record for the
 * event.
 *
 * Technical spec §"Human accountability": "Every … approval, suggestion
 * decision, verification decision, and merge decision must be attributable to
 * a human stakeholder ID."
 * Technical spec §"Protected operation contracts": "Submit branch".
 * S04 AC1.
 */
export interface BranchSubmittedRecord {
  readonly branchId: BranchId;
  readonly workspaceId: WorkspaceId;
  readonly submittedByStakeholderId: StakeholderId;
  readonly submittedAt: string;
}

/**
 * Creates a frozen BranchSubmittedRecord from a BranchOwnership and the
 * actor who submitted the branch.
 *
 * Throws `BranchLifecycleError('unauthorized-actor')` if the actor is a
 * supervised delegate — defense-in-depth alongside the type-level guard.
 * Throws `VocabularyValidationError` if `submittedAt` is not a valid
 * ISO-8601 date string.
 *
 * S04 AC1; Technical spec §"Protected operation contracts".
 */
export function branchSubmittedRecord(
  ownership: BranchOwnership,
  actor: ActorContext,
  submittedAt: string,
): BranchSubmittedRecord {
  if (!isHumanActor(actor)) {
    throw new BranchLifecycleError(
      'unauthorized-actor',
      'only a direct human stakeholder may produce a branch submission record',
    );
  }
  const trimmedAt = submittedAt.trim();
  if (
    !trimmedAt ||
    !/^\d{4}-\d{2}-\d{2}/.test(trimmedAt) ||
    isNaN(Date.parse(trimmedAt))
  ) {
    throw new VocabularyValidationError(
      'BranchSubmittedRecord.submittedAt',
      'submittedAt must be a valid ISO-8601 date string',
    );
  }
  return Object.freeze({
    branchId: ownership.branchId,
    workspaceId: ownership.workspaceId,
    submittedByStakeholderId: actor.stakeholderId,
    submittedAt: trimmedAt,
  });
}

// ─── BranchVerifiedRecord ─────────────────────────────────────────────────────

/**
 * Accountability record produced when a branch is verified by a human
 * stakeholder.
 *
 * Technical spec §"Human accountability".
 * Technical spec §"Protected operation contracts": "Verify branch".
 * S04 AC1.
 */
export interface BranchVerifiedRecord {
  readonly branchId: BranchId;
  readonly workspaceId: WorkspaceId;
  readonly verifiedByStakeholderId: StakeholderId;
  readonly verifiedAt: string;
}

/**
 * Creates a frozen BranchVerifiedRecord from a BranchOwnership and the
 * actor who verified the branch.
 *
 * Throws `BranchLifecycleError('unauthorized-actor')` if the actor is a
 * supervised delegate — defense-in-depth alongside the type-level guard.
 * Throws `VocabularyValidationError` if `verifiedAt` is not a valid
 * ISO-8601 date string.
 *
 * S04 AC1; Technical spec §"Protected operation contracts".
 */
export function branchVerifiedRecord(
  ownership: BranchOwnership,
  actor: ActorContext,
  verifiedAt: string,
): BranchVerifiedRecord {
  if (!isHumanActor(actor)) {
    throw new BranchLifecycleError(
      'unauthorized-actor',
      'only a direct human stakeholder may produce a branch verification record',
    );
  }
  const trimmedAt = verifiedAt.trim();
  if (
    !trimmedAt ||
    !/^\d{4}-\d{2}-\d{2}/.test(trimmedAt) ||
    isNaN(Date.parse(trimmedAt))
  ) {
    throw new VocabularyValidationError(
      'BranchVerifiedRecord.verifiedAt',
      'verifiedAt must be a valid ISO-8601 date string',
    );
  }
  return Object.freeze({
    branchId: ownership.branchId,
    workspaceId: ownership.workspaceId,
    verifiedByStakeholderId: actor.stakeholderId,
    verifiedAt: trimmedAt,
  });
}



/**
 * Provenance data that travels with graph records (chunks and edges) promoted
 * during a branch merge.
 *
 * This type is intended to be attached to individual chunk/edge persistence
 * records, enabling a stakeholder to trace any mainline item back to the
 * source branch via `sourceBranchId`.
 *
 * AC5. Meridian IDEA-29.
 */
export interface BranchGraphProvenance {
  readonly sourceBranchId: BranchId;
  readonly sourceWorkspaceId: WorkspaceId;
  readonly sourceDiscipline: Discipline;
}

/**
 * Attaches branch provenance to any graph record being promoted to the
 * mainline.
 *
 * Returns a new frozen object with a `branchProvenance` field derived from
 * the BranchOwnership. The original item fields are preserved unchanged.
 *
 * AC5.
 */
export function withBranchProvenance<T extends object>(
  item: T,
  ownership: BranchOwnership,
): T & { readonly branchProvenance: BranchGraphProvenance } {
  const provenance: BranchGraphProvenance = Object.freeze({
    sourceBranchId: ownership.branchId,
    sourceWorkspaceId: ownership.workspaceId,
    sourceDiscipline: ownership.discipline,
  });
  return Object.freeze({
    ...item,
    branchProvenance: provenance,
  }) as T & { readonly branchProvenance: BranchGraphProvenance };
}

// ─── State predicates ─────────────────────────────────────────────────────────

/**
 * Returns true when the branch is in the draft stage — work is still editable.
 *
 * AC2. Technical spec: "Draft to Submitted; … Return-to-Draft transitions are
 * never automated." Draft is the only editable state.
 */
export function isDraftBranch(state: BranchState): boolean {
  return state === 'draft';
}

/**
 * Returns true when the branch is graph-write locked: submitted, verified, or
 * merged.
 *
 * Write-locked branches block all graph-structure modifications (chunks, edges,
 * and associations). Metadata writes (verification signals, status logs, audit
 * metadata) are still permitted.
 *
 * AC4. Technical spec: "Submitted, Verified, and Merged branches are
 * graph-write locked, though verification signals, status logs, and audit
 * metadata may still be appended."
 * Meridian IDEA-35.
 */
export function isWriteLocked(state: BranchState): boolean {
  return state === 'submitted' || state === 'verified' || state === 'merged';
}

/**
 * Returns true when the branch has been merged — the terminal state.
 *
 * Merged branches retain their boundary, provenance, history, and merge
 * lineage. No further lifecycle transitions are possible.
 *
 * AC5. Meridian IDEA-17.
 */
export function isMergedBranch(state: BranchState): boolean {
  return state === 'merged';
}

// ─── Transition guards ────────────────────────────────────────────────────────

/**
 * Asserts that a graph-structure write (chunk/edge/association change) is
 * permitted for the given branch state.
 *
 * Throws `BranchLifecycleError` with code `'write-locked'` if the branch is
 * submitted, verified, or merged.
 *
 * AC4. Technical spec §"Required lifecycle contracts — Branch".
 * Meridian IDEA-35.
 */
export function assertGraphWriteAllowed(state: BranchState): void {
  if (isWriteLocked(state)) {
    throw new BranchLifecycleError(
      'write-locked',
      `graph writes are not permitted on a ${state} branch`,
    );
  }
}

/**
 * Asserts that the actor's discipline matches the branch's discipline,
 * satisfying the submit authorization rule.
 *
 * Throws `BranchLifecycleError` with code `'discipline-boundary-violation'`
 * if the disciplines do not match.
 *
 * AC3. Technical spec: §"Protected operation contracts" — "Submit branch:
 * Requires a direct human-authenticated stakeholder from the branch discipline."
 * Meridian IDEA-35.
 */
export function assertSubmitDiscipline(
  actorDiscipline: Discipline,
  branchDiscipline: Discipline,
): void {
  if (actorDiscipline !== branchDiscipline) {
    throw new BranchLifecycleError(
      'discipline-boundary-violation',
      `submit requires a stakeholder from the branch discipline '${branchDiscipline}'; actor is '${actorDiscipline}'`,
    );
  }
}

/**
 * Asserts that a branch write targets a chunk or edge owned by the same
 * discipline as the branch, enforcing branch isolation.
 *
 * Throws `BranchLifecycleError` with code `'branch-isolation-violation'` if
 * the target belongs to a different discipline.
 *
 * Note: cross-disciplinary *read* references (e.g. edges pointing to another
 * discipline's chunk without modifying it) are not subject to this guard.
 *
 * Technical spec §"Discipline boundary":
 * "A branch may modify chunks and edges owned by its discipline."
 * Meridian IDEA-35.
 */
export function assertDisciplineBoundaryForWrite(
  branchDiscipline: Discipline,
  targetDiscipline: Discipline,
): void {
  if (branchDiscipline !== targetDiscipline) {
    throw new BranchLifecycleError(
      'branch-isolation-violation',
      `branch discipline '${branchDiscipline}' cannot modify content owned by '${targetDiscipline}'`,
    );
  }
}

/**
 * Asserts that an operation does not cross workspace boundaries.
 *
 * Throws `BranchLifecycleError` with code `'tenant-boundary-violation'` if
 * the two workspace IDs differ.
 *
 * Technical spec §"Workspace scoping": every aggregate and graph operation is
 * workspace scoped; cross-workspace operations must not connect or resolve.
 * Technical spec §"Required domain error categories".
 */
export function assertWorkspaceMatch(
  branchWorkspaceId: WorkspaceId,
  operationWorkspaceId: WorkspaceId,
): void {
  if (branchWorkspaceId !== operationWorkspaceId) {
    throw new BranchLifecycleError(
      'tenant-boundary-violation',
      `operation workspace '${operationWorkspaceId}' does not match branch workspace '${branchWorkspaceId}'`,
    );
  }
}

// ─── State-machine transition functions ───────────────────────────────────────
//
// All transitions accept an `ActorContext` and include a runtime guard that
// throws `unauthorized-actor` if the actor is not a direct human stakeholder.
//
// **Defense-in-depth:** the runtime `isHumanActor` check complements session-
// layer authentication. The domain does not treat `actor.kind` alone as proof
// of identity; real authentication is the responsibility of the application
// boundary (technical spec §"Protected operation contracts"):
//   "Human authentication for protected operations must come from human-scoped
//    session credentials. Self-reported delegation … must never be accepted as
//    proof of direct human authentication."
//
// The `actor` parameter also carries `stakeholderId` for application-layer
// audit trail construction.

/**
 * Transitions a draft branch to submitted, locking graph writes.
 *
 * Requires:
 * - Current state is draft.
 * - Actor is a direct human stakeholder (runtime-enforced).
 * - Actor's discipline matches the branch discipline.
 *
 * AC3. Technical spec §"Protected operation contracts": "Submit branch —
 * Requires a direct human-authenticated stakeholder from the branch discipline
 * and locks graph writes."
 * Meridian IDEA-35, IDEA-40.
 */
export function submitBranch(
  state: BranchState,
  actor: ActorContext,
  actorDiscipline: Discipline,
  branchDiscipline: Discipline,
): 'submitted' {
  if (!isHumanActor(actor)) {
    throw new BranchLifecycleError(
      'unauthorized-actor',
      'only a direct human stakeholder may submit a branch',
    );
  }
  if (state !== 'draft') {
    throw new BranchLifecycleError(
      'invalid-state-transition',
      `cannot submit a branch that is already '${state}'; only draft branches may be submitted`,
    );
  }
  assertSubmitDiscipline(actorDiscipline, branchDiscipline);
  return 'submitted';
}

/**
 * Transitions a submitted branch to verified (human-initiated only).
 *
 * Requires:
 * - Current state is submitted.
 * - Actor is a direct human stakeholder (runtime-enforced).
 *
 * Verification signals are advisory — they never trigger this transition
 * automatically (Meridian IDEA-43).
 *
 * Technical spec §"Required lifecycle contracts — Branch": "Submitted to
 * Verified or human-initiated return to Draft."
 */
export function verifyBranch(
  state: BranchState,
  actor: ActorContext,
): 'verified' {
  if (!isHumanActor(actor)) {
    throw new BranchLifecycleError(
      'unauthorized-actor',
      'only a direct human stakeholder may verify a branch',
    );
  }
  if (state !== 'submitted') {
    throw new BranchLifecycleError(
      'invalid-state-transition',
      `cannot verify a branch that is '${state}'; only submitted branches may be verified`,
    );
  }
  return 'verified';
}

/**
 * Transitions a verified branch to merged (terminal state, human-initiated
 * only).
 *
 * Requires:
 * - Current state is verified.
 * - Actor is a direct human stakeholder (runtime-enforced).
 *
 * The merged state is terminal. Merged branches retain their boundary,
 * provenance, and merge lineage.
 *
 * Technical spec §"Required lifecycle contracts — Branch":
 * "Verified to Merged … Merged is terminal."
 * Meridian IDEA-42, IDEA-57.
 */
export function mergeBranch(
  state: BranchState,
  actor: ActorContext,
): 'merged' {
  if (!isHumanActor(actor)) {
    throw new BranchLifecycleError(
      'unauthorized-actor',
      'only a direct human stakeholder may merge a branch',
    );
  }
  if (state !== 'verified') {
    throw new BranchLifecycleError(
      'invalid-state-transition',
      `cannot merge a branch that is '${state}'; only verified branches may be merged`,
    );
  }
  return 'merged';
}

/**
 * Returns a submitted or verified branch to draft (human-initiated only,
 * never automated).
 *
 * Requires:
 * - Current state is submitted or verified (not draft, not merged).
 * - Actor is a direct human stakeholder (runtime-enforced).
 *
 * The merged state is terminal — return-to-draft is not permitted.
 *
 * Technical spec §"Required lifecycle contracts — Branch":
 * "Return-to-Draft transitions are never automated."
 * Meridian IDEA-35, IDEA-43.
 */
export function returnToDraft(
  state: BranchState,
  actor: ActorContext,
): 'draft' {
  if (!isHumanActor(actor)) {
    throw new BranchLifecycleError(
      'unauthorized-actor',
      'only a direct human stakeholder may return a branch to draft',
    );
  }
  if (state === 'merged') {
    throw new BranchLifecycleError(
      'invalid-state-transition',
      `cannot return a merged branch to draft; merged is a terminal state`,
    );
  }
  if (state === 'draft') {
    throw new BranchLifecycleError(
      'invalid-state-transition',
      `branch is already in draft state`,
    );
  }
  return 'draft';
}
