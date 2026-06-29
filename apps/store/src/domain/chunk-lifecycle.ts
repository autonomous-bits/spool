/**
 * Chunk lifecycle value objects and predicates.
 *
 * This module provides the domain-level tools for answering "is this idea
 * chunk safe to use?" — combining lifecycle stage and activity state into a
 * single inspectable value object with named predicates.
 *
 * Sources of authority:
 * - Story:              docs/specifications/feature-01-core-domain-model/stories/S02-chunk-lifecycle-clarity.md
 * - Technical spec:     docs/specifications/feature-01-core-domain-model/technical-specification.md
 *                       §"Required lifecycle contracts — Chunk"
 * - Constitution:       docs/constitution.md (Principle IV — Rich Domain Models)
 * - Meridian IDEA-38:   superseded state is lineage, not deletion
 * - Meridian IDEA-35:   domain boundary enforces lifecycle invariants
 *
 * Story: S02 — See whether idea context is safe to use.
 */

import type { ChunkActivityState, ChunkLifecycleState } from './vocabulary.js';

export type { ChunkActivityState, ChunkLifecycleState };

// ─── Domain error ─────────────────────────────────────────────────────────────

/**
 * Thrown when a `ChunkLifecycleStatus` is constructed from an invalid
 * combination of lifecycle stage and activity state.
 *
 * Maps to the "Invalid state transition" domain error category from
 * the technical specification §"Required domain error categories".
 * The `code` property allows adapters to identify the category without
 * inspecting free-form message text.
 */
export class ChunkLifecycleValidationError extends Error {
  override readonly name = 'ChunkLifecycleValidationError';
  readonly code = 'invalid-state-transition' as const;

  constructor(
    readonly lifecycleState: ChunkLifecycleState,
    readonly activityState: ChunkActivityState,
    readonly reason: string,
  ) {
    super(
      `Invalid chunk lifecycle status [${lifecycleState}+${activityState}]: ${reason}`,
    );
  }
}

// ─── Value object ─────────────────────────────────────────────────────────────

// The brand symbol is module-private: callers cannot name it, so they cannot
// construct a ChunkLifecycleStatus by structural assignment. The only valid
// entry point is the `chunkLifecycleStatus` factory below.
const _statusBrand: unique symbol = Symbol('ChunkLifecycleStatus');

/**
 * The combined lifecycle status of an idea chunk: its progression stage
 * and its activity state.
 *
 * These two dimensions are independent — approval state and activity state
 * are tracked separately — but not all combinations are valid. The brand
 * symbol makes this type opaque: only `chunkLifecycleStatus()` can produce a
 * valid instance, preventing callers from bypassing factory invariants via
 * structural assignment.
 *
 * Technical spec: "Approval state and activity state are separate."
 */
export type ChunkLifecycleStatus = {
  readonly lifecycleState: ChunkLifecycleState;
  readonly activityState: ChunkActivityState;
  readonly [_statusBrand]: never;
};

/**
 * Creates a frozen, validated `ChunkLifecycleStatus`.
 *
 * The technical specification states that only approved or promoted chunks may
 * become superseded or inactive. Draft chunks are always active — a draft has
 * not yet reached the approval gate at which later-stage activity transitions
 * become meaningful.
 *
 * Throws `ChunkLifecycleValidationError` for:
 * - `draft + superseded`
 * - `draft + inactive`
 *
 * @param lifecycleState  The progression stage: draft, approved, or promoted.
 * @param activityState   The activity state: active, superseded, or inactive.
 */
export function chunkLifecycleStatus(
  lifecycleState: ChunkLifecycleState,
  activityState: ChunkActivityState,
): ChunkLifecycleStatus {
  if (lifecycleState === 'draft' && activityState !== 'active') {
    throw new ChunkLifecycleValidationError(
      lifecycleState,
      activityState,
      'only approved or promoted chunks may become superseded or inactive',
    );
  }
  return Object.freeze({ lifecycleState, activityState }) as ChunkLifecycleStatus;
}

// ─── Lifecycle stage predicates ───────────────────────────────────────────────

/** Returns true when the chunk is still in the draft stage (not yet approved). */
export function isDraftChunk(status: ChunkLifecycleStatus): boolean {
  return status.lifecycleState === 'draft';
}

/**
 * Returns true when the chunk has been approved by a human stakeholder
 * and has not yet been promoted to the mainline.
 */
export function isApprovedChunk(status: ChunkLifecycleStatus): boolean {
  return status.lifecycleState === 'approved';
}

/** Returns true when the chunk has been promoted to the mainline context. */
export function isPromotedChunk(status: ChunkLifecycleStatus): boolean {
  return status.lifecycleState === 'promoted';
}

// ─── Activity state predicates ────────────────────────────────────────────────

/** Returns true when the chunk is active — neither superseded nor inactive. */
export function isActiveChunk(status: ChunkLifecycleStatus): boolean {
  return status.activityState === 'active';
}

/**
 * Returns true when the chunk has been superseded by a later version.
 *
 * A superseded chunk is not deleted. It retains its lifecycle stage and remains
 * inspectable, preserving history and provenance (Meridian IDEA-38).
 *
 * AC4: A stakeholder can tell when an approved or promoted idea has been
 * replaced without losing the fact that it previously existed.
 */
export function isSupersededChunk(status: ChunkLifecycleStatus): boolean {
  return status.activityState === 'superseded';
}

/** Returns true when the chunk has been deactivated and is no longer in use. */
export function isInactiveChunk(status: ChunkLifecycleStatus): boolean {
  return status.activityState === 'inactive';
}

// ─── Composite safety predicate ───────────────────────────────────────────────

/**
 * Returns true when the chunk is safe to include in implementation context
 * packages delivered to agents.
 *
 * Safe means: the chunk has been **approved or promoted** (lifecycle stage) AND
 * is **active** (activity state). Both conditions must hold simultaneously.
 *
 * Technical spec "Generated context": "Generated context packages are
 * projections from approved or promoted chunks and active resolved
 * relationships."
 *
 * AC2: Draft context is never safe for implementation use.
 * AC3: An agent must receive only context that passes this predicate.
 */
export function isSafeForImplementationUse(status: ChunkLifecycleStatus): boolean {
  return (
    (status.lifecycleState === 'approved' || status.lifecycleState === 'promoted') &&
    status.activityState === 'active'
  );
}
