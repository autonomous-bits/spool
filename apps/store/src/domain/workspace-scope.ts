/**
 * Distinct domain error for a workspace-scope violation (Meridian IDEA-98 amendment to the
 * Domain Invariant Protection Service, IDEA-53). Kept separate from other domain errors
 * (e.g. `BranchLifecycleError`, `WorkspaceMembershipRejectedError`) so callers can map it to a
 * consistent HTTP status (403) across every workspace-scoped route.
 */
export class WorkspaceScopeViolationError extends Error {
  constructor(reason: string) {
    super(`Workspace scope violation: ${reason}`);
    this.name = 'WorkspaceScopeViolationError';
  }
}

/**
 * Which of the two existing auth tiers (Meridian IDEA-100) a route belongs to:
 *
 * - `token`: the route already requires a human session token (e.g. branch submit/verify/merge,
 *   notification reads). Enforcement compares the request's `X-Workspace-Id` header against the
 *   token's `workspaceId` claim.
 * - `delegated`: the route accepts delegated, tokenless calls (e.g. chunks, edges, artifacts,
 *   suggestions, verification-signals). Enforcement compares the header against a
 *   `workspace_memberships` row for the caller-supplied `stakeholderId` — the same trusted,
 *   caller-declared identity already used for discipline attribution on these routes. No session
 *   token is required or checked here.
 */
export type WorkspaceScopeCheck =
  | { tier: 'token'; workspaceIdClaim: string | null }
  | { tier: 'delegated'; isMember: boolean };

/**
 * Reusable two-tier workspace-scope assertion implementing Meridian IDEA-98/IDEA-100. A missing
 * (or blank) `X-Workspace-Id` header is itself a rejection, regardless of tier — every
 * workspace-scoped request must name its workspace explicitly. Async membership lookups
 * (`WorkspaceRepository.isMember`) are the caller's responsibility, mirroring the existing
 * `assertCanAddMember(actorIsMember: boolean)` pattern: this function stays pure/sync and only
 * enforces the invariant given the already-resolved facts.
 */
export function assertWorkspaceScope(
  headerWorkspaceId: string | null | undefined,
  check: WorkspaceScopeCheck,
): asserts headerWorkspaceId is string {
  if (headerWorkspaceId === null || headerWorkspaceId === undefined || headerWorkspaceId.trim().length === 0) {
    throw new WorkspaceScopeViolationError('missing X-Workspace-Id header');
  }

  if (check.tier === 'token') {
    if (check.workspaceIdClaim === null) {
      throw new WorkspaceScopeViolationError(
        'session token is workspace-less; re-authenticate with a workspaceId to obtain a workspace-bound token',
      );
    }
    if (check.workspaceIdClaim !== headerWorkspaceId) {
      throw new WorkspaceScopeViolationError(
        `X-Workspace-Id ${headerWorkspaceId} does not match the session token's workspaceId claim`,
      );
    }
    return;
  }

  if (!check.isMember) {
    throw new WorkspaceScopeViolationError(
      `caller is not a member of workspace ${headerWorkspaceId}`,
    );
  }
}
