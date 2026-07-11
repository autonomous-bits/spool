import { randomUUID } from 'node:crypto';
import type { VerificationSignalStatus } from './types/vocabulary/verification-signal-status.js';
import { parseVerificationSignalStatus } from './types/vocabulary/verification-signal-status.js';

export interface VerificationSignalProps {
  id?: string;
  workspaceId: string;
  branchId: string;
  reportedByStakeholderId: string;
  verifierName: string;
  status: VerificationSignalStatus;
  reason?: string;
  createdAt?: Date;
}

function requireNonBlank(value: string, fieldName: string): string {
  if (value.trim().length === 0) {
    throw new TypeError(`VerificationSignal ${fieldName} must not be empty or blank`);
  }
  return value;
}

/**
 * VerificationSignal entity: a pass/fail evaluation logged against a submitted/verified branch
 * (Meridian IDEA-21/IDEA-31). `verifierName` remains untrusted free text describing whichever
 * agent, tool, or human produced the evaluation; it is intentionally not a stakeholder FK.
 *
 * Meridian IDEA-139 adds `reportedByStakeholderId` as the authenticated caller identity derived
 * from verified session-token claims at submission time. Recorded as feedback only -- never mutates
 * the branch's own status (Meridian IDEA-43's explicit no-auto-transition rule); that invariant is
 * enforced by `assertReviewableStatus` in `branch-lifecycle.ts`, not here.
 */
export class VerificationSignal {
  readonly id: string;
  readonly workspaceId: string;
  readonly branchId: string;
  readonly reportedByStakeholderId: string;
  readonly verifierName: string;
  readonly status: VerificationSignalStatus;
  readonly reason?: string;
  readonly createdAt: Date;

  constructor(props: VerificationSignalProps) {
    if (props.branchId.trim().length === 0) {
      throw new TypeError('VerificationSignal requires a non-blank branchId');
    }

    this.workspaceId = requireNonBlank(props.workspaceId, 'workspaceId');
    this.reportedByStakeholderId = requireNonBlank(
      props.reportedByStakeholderId,
      'reportedByStakeholderId',
    );
    this.verifierName = requireNonBlank(props.verifierName, 'verifierName');
    this.status = parseVerificationSignalStatus(props.status);
    this.id = props.id ?? randomUUID();
    this.branchId = props.branchId;
    if (props.reason !== undefined) {
      this.reason = props.reason;
    }
    this.createdAt = props.createdAt ?? new Date();
  }
}
