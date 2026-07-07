import type { WorkspaceMembership } from '../domain/workspace-membership.js';

/**
 * HTTP-facing shape of a persisted WorkspaceMembership, per Meridian IDEA-94/IDEA-95's flat,
 * no-roles membership contract.
 */
export interface WorkspaceMembershipResponse {
  workspaceId: string;
  stakeholderId: string;
  createdAt: Date;
}

export function toWorkspaceMembershipResponse(
  membership: WorkspaceMembership,
): WorkspaceMembershipResponse {
  return {
    workspaceId: membership.workspaceId,
    stakeholderId: membership.stakeholderId,
    createdAt: membership.createdAt,
  } satisfies WorkspaceMembershipResponse;
}
