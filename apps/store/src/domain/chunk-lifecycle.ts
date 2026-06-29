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

import type {
  ChunkActivityState,
  ChunkLifecycleState,
} from './types/index.js';

export type { ChunkActivityState, ChunkLifecycleState };

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

const _statusBrand: unique symbol = Symbol('ChunkLifecycleStatus');

export type ChunkLifecycleStatus = {
  readonly lifecycleState: ChunkLifecycleState;
  readonly activityState: ChunkActivityState;
  readonly [_statusBrand]: never;
};

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

export function isDraftChunk(status: ChunkLifecycleStatus): boolean {
  return status.lifecycleState === 'draft';
}

export function isApprovedChunk(status: ChunkLifecycleStatus): boolean {
  return status.lifecycleState === 'approved';
}

export function isPromotedChunk(status: ChunkLifecycleStatus): boolean {
  return status.lifecycleState === 'promoted';
}

export function isActiveChunk(status: ChunkLifecycleStatus): boolean {
  return status.activityState === 'active';
}

export function isSupersededChunk(status: ChunkLifecycleStatus): boolean {
  return status.activityState === 'superseded';
}

export function isInactiveChunk(status: ChunkLifecycleStatus): boolean {
  return status.activityState === 'inactive';
}

export function isSafeForImplementationUse(status: ChunkLifecycleStatus): boolean {
  return (
    (status.lifecycleState === 'approved' ||
      status.lifecycleState === 'promoted') &&
    status.activityState === 'active'
  );
}
