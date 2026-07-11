import type { Workspace } from './workspace.js';

export interface WorkspaceMembershipProps {
  workspaceId: string;
  stakeholderId: string;
  createdAt?: Date;
}

/**
 * WorkspaceMembership: a flat, no-roles membership row per Meridian IDEA-95 — every member has
 * equal read/write standing, including the workspace creator.
 */
export class WorkspaceMembership {
  readonly workspaceId: string;
  readonly stakeholderId: string;
  readonly createdAt: Date;

  constructor(props: WorkspaceMembershipProps) {
    if (props.workspaceId.trim().length === 0) {
      throw new TypeError('WorkspaceMembership requires a non-blank workspaceId');
    }
    if (props.stakeholderId.trim().length === 0) {
      throw new TypeError('WorkspaceMembership requires a non-blank stakeholderId');
    }

    this.workspaceId = props.workspaceId;
    this.stakeholderId = props.stakeholderId;
    this.createdAt = props.createdAt ?? new Date();
  }
}

/**
 * Rejection of an add-member attempt because the caller is not an existing member of the
 * workspace. Kept distinct from WorkspaceMembershipAlreadyExistsError, which signals the
 * unrelated duplicate-add condition.
 */
export class WorkspaceMembershipRejectedError extends Error {
  constructor(reason: string) {
    super(`Cannot add workspace member: ${reason}`);
    this.name = 'WorkspaceMembershipRejectedError';
  }
}

/**
 * Distinct domain error for duplicate-add: the target stakeholder is already a member of the
 * workspace. Separate from WorkspaceMembershipRejectedError (non-member-caller rejection) so
 * callers (e.g. the API layer) can map each to a different HTTP status.
 */
export class WorkspaceMembershipAlreadyExistsError extends Error {
  constructor(workspaceId: string, stakeholderId: string) {
    super(`Stakeholder ${stakeholderId} is already a member of workspace ${workspaceId}`);
    this.name = 'WorkspaceMembershipAlreadyExistsError';
  }
}

/**
 * Meridian IDEA-95: the workspace creator is simply the first row inserted into
 * workspace_memberships, carrying no special privilege beyond that. This derives the single
 * creator membership row implied by workspace creation; SG2's persistence layer persists it
 * verbatim and must not re-derive it independently.
 */
export function deriveInitialMembership(
  workspace: Pick<Workspace, 'id' | 'createdByStakeholderId'>,
): WorkspaceMembership {
  return new WorkspaceMembership({
    workspaceId: workspace.id,
    stakeholderId: workspace.createdByStakeholderId,
  });
}

/**
 * Meridian IDEA-88/95: only an existing member may add another member to a workspace
 * (direct-add, no roles, no pending/acceptance step). The caller (persistence layer) determines
 * membership and passes the result in; this function only enforces the invariant.
 */
export function assertCanAddMember(actorIsMember: boolean): void {
  if (!actorIsMember) {
    throw new WorkspaceMembershipRejectedError(
      'caller is not an existing member of the workspace',
    );
  }
}
