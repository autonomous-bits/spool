import { describe, expect, it } from 'vitest';
import { WorkspaceScopeViolationError, assertWorkspaceScope } from './workspace-scope.js';

const WORKSPACE_A = '11111111-1111-1111-1111-111111111111';
const WORKSPACE_B = '22222222-2222-2222-2222-222222222222';

describe('assertWorkspaceScope', () => {
  it('rejects a missing X-Workspace-Id header regardless of tier', () => {
    expect(() =>
      { assertWorkspaceScope(undefined, { tier: 'token', workspaceIdClaim: WORKSPACE_A }); },
    ).toThrow(WorkspaceScopeViolationError);
    expect(() =>
      { assertWorkspaceScope(null, { tier: 'delegated', isMember: true }); },
    ).toThrow(WorkspaceScopeViolationError);
  });

  it('rejects a blank X-Workspace-Id header', () => {
    expect(() =>
      { assertWorkspaceScope('   ', { tier: 'delegated', isMember: true }); },
    ).toThrow(WorkspaceScopeViolationError);
  });

  describe('token tier', () => {
    it('accepts a header that matches the token workspaceId claim', () => {
      expect(() =>
        { assertWorkspaceScope(WORKSPACE_A, { tier: 'token', workspaceIdClaim: WORKSPACE_A }); },
      ).not.toThrow();
    });

    it('rejects a header that does not match the token workspaceId claim', () => {
      expect(() =>
        { assertWorkspaceScope(WORKSPACE_A, { tier: 'token', workspaceIdClaim: WORKSPACE_B }); },
      ).toThrow(WorkspaceScopeViolationError);
    });

    it('rejects a workspace-less (null claim) token', () => {
      expect(() =>
        { assertWorkspaceScope(WORKSPACE_A, { tier: 'token', workspaceIdClaim: null }); },
      ).toThrow(WorkspaceScopeViolationError);
    });
  });

  describe('delegated tier', () => {
    it('accepts when the caller-supplied stakeholderId has a membership row', () => {
      expect(() =>
        { assertWorkspaceScope(WORKSPACE_A, { tier: 'delegated', isMember: true }); },
      ).not.toThrow();
    });

    it('rejects when the caller-supplied stakeholderId has no membership row', () => {
      expect(() =>
        { assertWorkspaceScope(WORKSPACE_A, { tier: 'delegated', isMember: false }); },
      ).toThrow(WorkspaceScopeViolationError);
    });
  });

  it('throws a distinct WorkspaceScopeViolationError type', () => {
    let error: unknown;
    try {
      assertWorkspaceScope(undefined, { tier: 'delegated', isMember: false });
    } catch (caught) {
      error = caught;
    }

    expect(error).toBeInstanceOf(Error);
    expect(error).toBeInstanceOf(WorkspaceScopeViolationError);
    expect((error as Error).name).toBe('WorkspaceScopeViolationError');
  });
});
