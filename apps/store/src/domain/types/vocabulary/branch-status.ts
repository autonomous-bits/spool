/**
 * Vocabulary: BranchStatus enum, per Meridian IDEA-31 (authoritative Postgres schema) and
 * IDEA-40 (submission/verification/merge transitions require direct human authentication). This
 * goal's write path only ever produces 'draft'; the remaining three values are modeled here so
 * later reads/goals can round-trip them, but their transitions are out of scope for G02.
 */
export type BranchStatus = 'draft' | 'submitted' | 'verified' | 'merged';

const BRANCH_STATUSES: readonly BranchStatus[] = ['draft', 'submitted', 'verified', 'merged'];

export function isBranchStatus(value: unknown): value is BranchStatus {
  return typeof value === 'string' && (BRANCH_STATUSES as readonly string[]).includes(value);
}

export function parseBranchStatus(value: unknown): BranchStatus {
  if (!isBranchStatus(value)) {
    throw new TypeError(`Invalid BranchStatus: ${JSON.stringify(value)}`);
  }
  return value;
}
