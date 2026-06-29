import type { WorkspaceId } from '../identifiers/workspace-id.js';

export interface WorkspaceScoped<T> {
  readonly workspaceId: WorkspaceId;
  readonly value: T;
}

export function inWorkspace<T>(workspaceId: WorkspaceId, value: T): WorkspaceScoped<T> {
  return { workspaceId, value };
}
