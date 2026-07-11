import { randomUUID } from 'node:crypto';
import type { VerificationSignalStatus } from './types/vocabulary/verification-signal-status.js';
import { parseVerificationSignalStatus } from './types/vocabulary/verification-signal-status.js';

export interface VerificationSignalProps {
  id?: string;
  workspaceId: string;
  branchId: string;
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
 * VerificationSignal entity: a pass/fail evaluation logged by a dedicated agent, tool, or human
 * reviewer against a submitted/verified branch (Meridian IDEA-21/IDEA-31). `verifierName` is
 * free text, not a stakeholder FK (IDEA-21's "agents, tools, or other humans" is broader than the
 * registered-stakeholder set). Recorded as feedback only -- never mutates the branch's own status
 * (Meridian IDEA-43's explicit no-auto-transition rule); that invariant is enforced by
 * `assertReviewableStatus` in `branch-lifecycle.ts`, not here.
 */
export class VerificationSignal {
  readonly id: string;
  readonly workspaceId: string;
  readonly branchId: string;
  readonly verifierName: string;
  readonly status: VerificationSignalStatus;
  readonly reason?: string;
  readonly createdAt: Date;

  constructor(props: VerificationSignalProps) {
    if (props.branchId.trim().length === 0) {
      throw new TypeError('VerificationSignal requires a non-blank branchId');
    }

    this.workspaceId = requireNonBlank(props.workspaceId, 'workspaceId');
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
