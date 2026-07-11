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
 * Single-tier workspace-scope check (Meridian IDEA-139, superseding IDEA-100's two-tier model).
 * Every workspace-scoped request — with no exception other than the IDEA-101 workspace-less
 * bootstrap token for `POST /workspaces` — must present a verified session token: the request's
 * `X-Workspace-Id` header must match the token's `workspaceId` claim, *and* the token's
 * `stakeholderId` must currently be a member of that workspace. Token validity/claim-match alone
 * is insufficient — a removed member with an unexpired token must still be rejected.
 */
export interface WorkspaceScopeCheck {
  workspaceIdClaim: string | null;
  isMember: boolean;
}

/**
 * Reusable single-tier workspace-scope assertion implementing Meridian IDEA-98/IDEA-139. A
 * missing (or blank) `X-Workspace-Id` header is itself a rejection — every workspace-scoped
 * request must name its workspace explicitly. The async membership lookup
 * (`WorkspaceRepository.isMember`) is the caller's responsibility, mirroring the existing
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

  if (!check.isMember) {
    throw new WorkspaceScopeViolationError(
      `caller is not a current member of workspace ${headerWorkspaceId}`,
    );
  }
}
