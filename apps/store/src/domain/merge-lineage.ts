import type { Discipline } from './types/vocabulary/discipline.js';
import { DivergencePoint } from './divergence-point.js';

function requireNonBlank(value: string, typeName: string, fieldName: string): string {
  if (value.trim().length === 0) {
    throw new TypeError(`${typeName} ${fieldName} must not be empty or blank`);
  }
  return value;
}

export interface MergeLineageProps {
  branchId: string;
  discipline: Discipline;
  divergedAt: DivergencePoint;
  mergedAt: Date;
  mergedByStakeholderId: string;
}

/**
 * MergeLineage: an immutable record produced when a branch merges into mainline, per Meridian
 * IDEA-74's authoritative shape, enabling tracing any mainline idea back to the branch that
 * introduced it (relates to IDEA-29: "queryable by lineage").
 *
 * SCOPE DEVIATION (G06 OQ1, resolved 2026-07-06): IDEA-74 specifies a `workspaceId` field, but
 * this repo has no workspace concept anywhere yet, so `workspaceId` is omitted here. This is a
 * repo-local scoping decision, not a Meridian conflict, deferred to a future goal.
 */
export class MergeLineage {
  readonly branchId: string;
  readonly discipline: Discipline;
  readonly divergedAt: DivergencePoint;
  readonly mergedAt: Date;
  readonly mergedByStakeholderId: string;

  constructor(props: MergeLineageProps) {
    this.branchId = requireNonBlank(props.branchId, 'MergeLineage', 'branchId');
    this.mergedByStakeholderId = requireNonBlank(
      props.mergedByStakeholderId,
      'MergeLineage',
      'mergedByStakeholderId',
    );
    this.discipline = props.discipline;
    this.divergedAt = props.divergedAt;
    this.mergedAt = props.mergedAt;
  }
}

export interface BranchGraphProvenanceProps {
  sourceBranchId: string;
}

/**
 * BranchGraphProvenance: a per-record provenance marker attached to individual chunk/edge rows
 * promoted during a merge, per Meridian IDEA-74's authoritative shape, so any single mainline
 * item can be traced to its source branch (relates to IDEA-29 AC5 and IDEA-69's
 * `origin_branch_id` persistence).
 *
 * SCOPE DEVIATION (G06 OQ1, resolved 2026-07-06): IDEA-74 also specifies `sourceWorkspaceId` and
 * `sourceDiscipline` fields, but this repo's chunks/edges schema only persists
 * `origin_branch_id` (no workspace concept, no per-row origin-discipline column), so both are
 * omitted here. This is a repo-local scoping decision, not a Meridian conflict, deferred to a
 * future goal.
 */
export class BranchGraphProvenance {
  readonly sourceBranchId: string;

  constructor(props: BranchGraphProvenanceProps) {
    this.sourceBranchId = requireNonBlank(
      props.sourceBranchId,
      'BranchGraphProvenance',
      'sourceBranchId',
    );
  }
}
