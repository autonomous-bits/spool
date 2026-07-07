import { describe, expect, it } from 'vitest';
import { Workspace, type WorkspaceProps } from './workspace.js';

function validProps(overrides: Partial<WorkspaceProps> = {}): WorkspaceProps {
  return {
    name: 'Acme Product Line',
    createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
    ...overrides,
  };
}

describe('Workspace', () => {
  it('constructs a workspace with defaulted id and timestamps', () => {
    const workspace = new Workspace(validProps());

    expect(workspace.name).toBe('Acme Product Line');
    expect(workspace.createdByStakeholderId).toBe('00000000-0000-0000-0000-000000000001');
    expect(workspace.id).toBeTruthy();
    expect(workspace.createdAt).toBeInstanceOf(Date);
    expect(workspace.updatedAt).toEqual(workspace.createdAt);
  });

  it('round-trips an explicit id, createdAt, and updatedAt when provided', () => {
    const createdAt = new Date('2026-07-05T12:00:00.000Z');
    const updatedAt = new Date('2026-07-06T09:00:00.000Z');

    const workspace = new Workspace(
      validProps({ id: '11111111-1111-1111-1111-111111111111', createdAt, updatedAt }),
    );

    expect(workspace.id).toBe('11111111-1111-1111-1111-111111111111');
    expect(workspace.createdAt).toEqual(createdAt);
    expect(workspace.updatedAt).toEqual(updatedAt);
  });

  it('rejects a blank name', () => {
    expect(() => new Workspace(validProps({ name: '   ' }))).toThrow(TypeError);
  });

  it('rejects a blank createdByStakeholderId', () => {
    expect(() => new Workspace(validProps({ createdByStakeholderId: '' }))).toThrow(TypeError);
  });
});
