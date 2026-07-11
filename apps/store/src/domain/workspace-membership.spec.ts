import { describe, expect, it } from 'vitest';
import { Workspace } from './workspace.js';
import {
  WorkspaceMembership,
  WorkspaceMembershipAlreadyExistsError,
  WorkspaceMembershipRejectedError,
  assertCanAddMember,
  deriveInitialMembership,
} from './workspace-membership.js';

function createWorkspace(): Workspace {
  return new Workspace({
    name: 'Acme Product Line',
    createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
  });
}

describe('WorkspaceMembership', () => {
  it('rejects a blank workspaceId', () => {
    expect(
      () =>
        new WorkspaceMembership({
          workspaceId: '',
          stakeholderId: '00000000-0000-0000-0000-000000000001',
        }),
    ).toThrow(TypeError);
  });

  it('rejects a blank stakeholderId', () => {
    expect(
      () =>
        new WorkspaceMembership({
          workspaceId: '11111111-1111-1111-1111-111111111111',
          stakeholderId: '',
        }),
    ).toThrow(TypeError);
  });
});

describe('deriveInitialMembership', () => {
  it('yields the creator-only membership row for a newly created workspace', () => {
    const workspace = createWorkspace();

    const membership = deriveInitialMembership(workspace);

    expect(membership).toBeInstanceOf(WorkspaceMembership);
    expect(membership.workspaceId).toBe(workspace.id);
    expect(membership.stakeholderId).toBe(workspace.createdByStakeholderId);
    expect(membership.createdAt).toBeInstanceOf(Date);
  });
});

describe('assertCanAddMember', () => {
  it('rejects a non-member caller', () => {
    expect(() => { assertCanAddMember(false); }).toThrow(WorkspaceMembershipRejectedError);
  });

  it('allows any existing member to add another member', () => {
    expect(() => { assertCanAddMember(true); }).not.toThrow();
  });
});

describe('WorkspaceMembershipAlreadyExistsError', () => {
  it('is a distinct error type from WorkspaceMembershipRejectedError', () => {
    const error = new WorkspaceMembershipAlreadyExistsError(
      '11111111-1111-1111-1111-111111111111',
      '00000000-0000-0000-0000-000000000002',
    );

    expect(error).toBeInstanceOf(Error);
    expect(error).not.toBeInstanceOf(WorkspaceMembershipRejectedError);
    expect(error.name).toBe('WorkspaceMembershipAlreadyExistsError');
  });
});
