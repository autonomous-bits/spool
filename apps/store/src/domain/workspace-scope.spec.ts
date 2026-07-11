import { describe, expect, it } from 'vitest';
import { WorkspaceScopeViolationError, assertWorkspaceScope } from './workspace-scope.js';

const WORKSPACE_A = '11111111-1111-1111-1111-111111111111';
const WORKSPACE_B = '22222222-2222-2222-2222-222222222222';

describe('assertWorkspaceScope', () => {
  it('rejects a missing X-Workspace-Id header', () => {
    expect(() =>
      { assertWorkspaceScope(undefined, { workspaceIdClaim: WORKSPACE_A, isMember: true }); },
    ).toThrow(WorkspaceScopeViolationError);
    expect(() =>
      { assertWorkspaceScope(null, { workspaceIdClaim: WORKSPACE_A, isMember: true }); },
    ).toThrow(WorkspaceScopeViolationError);
  });

  it('rejects a blank X-Workspace-Id header', () => {
    expect(() =>
      { assertWorkspaceScope('   ', { workspaceIdClaim: WORKSPACE_A, isMember: true }); },
    ).toThrow(WorkspaceScopeViolationError);
  });

  it('rejects a workspace-less (null claim) token', () => {
    expect(() =>
      { assertWorkspaceScope(WORKSPACE_A, { workspaceIdClaim: null, isMember: true }); },
    ).toThrow(WorkspaceScopeViolationError);
  });

  it('rejects a header that does not match the token workspaceId claim (claim-mismatch)', () => {
    expect(() =>
      { assertWorkspaceScope(WORKSPACE_A, { workspaceIdClaim: WORKSPACE_B, isMember: true }); },
    ).toThrow(WorkspaceScopeViolationError);
  });

  it('rejects a claim-matched header when the stakeholder is not a current member (403)', () => {
    // A removed member with an unexpired, otherwise-matching token must still be rejected
    // (Meridian IDEA-139) — token validity/claim-match alone is never sufficient.
    expect(() =>
      { assertWorkspaceScope(WORKSPACE_A, { workspaceIdClaim: WORKSPACE_A, isMember: false }); },
    ).toThrow(WorkspaceScopeViolationError);
  });

  it('accepts a claim-matched header when the stakeholder is a current member', () => {
    expect(() =>
      { assertWorkspaceScope(WORKSPACE_A, { workspaceIdClaim: WORKSPACE_A, isMember: true }); },
    ).not.toThrow();
  });

  it('throws a distinct WorkspaceScopeViolationError type', () => {
    let error: unknown;
    try {
      assertWorkspaceScope(undefined, { workspaceIdClaim: WORKSPACE_A, isMember: false });
    } catch (caught) {
      error = caught;
    }

    expect(error).toBeInstanceOf(Error);
    expect(error).toBeInstanceOf(WorkspaceScopeViolationError);
    expect((error as Error).name).toBe('WorkspaceScopeViolationError');
  });
});
