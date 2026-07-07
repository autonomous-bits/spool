import { randomUUID } from 'node:crypto';
import type { Discipline } from './types/vocabulary/discipline.js';
import { parseDiscipline } from './types/vocabulary/discipline.js';
import type { BranchStatus } from './types/vocabulary/branch-status.js';
import { parseBranchStatus } from './types/vocabulary/branch-status.js';
import { DivergencePoint } from './divergence-point.js';

export interface BranchProps {
  id?: string;
  workspaceId: string;
  name: string;
  discipline: Discipline;
  status?: BranchStatus;
  createdByStakeholderId: string;
  divergedAt?: DivergencePoint;
  submittedAt?: Date;
  verifiedAt?: Date;
  mergedAt?: Date;
  mergedByStakeholderId?: string;
  originSuggestionId?: string;
  createdAt?: Date;
  updatedAt?: Date;
}

function requireNonBlank(value: string, fieldName: string): string {
  if (value.trim().length === 0) {
    throw new TypeError(`Branch ${fieldName} must not be empty or blank`);
  }
  return value;
}

/**
 * Branch entity: the authoring surface for iterative, multi-chunk, cross-disciplinary work, per
 * Meridian IDEA-24/IDEA-17. Enforces this goal's write-path invariants: non-blank name, a valid
 * closed discipline vocabulary, and a required authoring stakeholder. This goal's write path only
 * ever produces `status: 'draft'`; submission/verification/merge transitions (IDEA-40) are out of
 * scope here.
 */
export class Branch {
  readonly id: string;
  readonly workspaceId: string;
  readonly name: string;
  readonly discipline: Discipline;
  readonly status: BranchStatus;
  readonly createdByStakeholderId: string;
  readonly divergedAt: DivergencePoint;
  readonly submittedAt?: Date;
  readonly verifiedAt?: Date;
  readonly mergedAt?: Date;
  readonly mergedByStakeholderId?: string;
  readonly originSuggestionId?: string;
  readonly createdAt: Date;
  readonly updatedAt: Date;

  constructor(props: BranchProps) {
    this.workspaceId = requireNonBlank(props.workspaceId, 'workspaceId');
    this.name = requireNonBlank(props.name, 'name');
    this.discipline = parseDiscipline(props.discipline);

    if (props.createdByStakeholderId.trim().length === 0) {
      throw new TypeError('Branch requires a non-blank createdByStakeholderId');
    }

    this.id = props.id ?? randomUUID();
    this.status = props.status === undefined ? 'draft' : parseBranchStatus(props.status);
    this.createdByStakeholderId = props.createdByStakeholderId;
    this.divergedAt = props.divergedAt ?? new DivergencePoint();
    if (props.submittedAt !== undefined) {
      this.submittedAt = props.submittedAt;
    }
    if (props.verifiedAt !== undefined) {
      this.verifiedAt = props.verifiedAt;
    }
    if (props.mergedAt !== undefined) {
      this.mergedAt = props.mergedAt;
    }
    if (props.mergedByStakeholderId !== undefined) {
      this.mergedByStakeholderId = props.mergedByStakeholderId;
    }
    if (props.originSuggestionId !== undefined) {
      this.originSuggestionId = props.originSuggestionId;
    }
    this.createdAt = props.createdAt ?? new Date();
    this.updatedAt = props.updatedAt ?? this.createdAt;
  }
}
