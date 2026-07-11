import type { Workspace } from '../domain/workspace.js';

/**
 * HTTP-facing shape of a persisted Workspace, per Meridian IDEA-94's request/response contract.
 * Deliberately narrower than the Workspace domain entity (omits `updatedAt`), matching the
 * ratified `POST /workspaces` response shape exactly.
 */
export interface WorkspaceResponse {
  id: string;
  name: string;
  createdByStakeholderId: string;
  createdAt: Date;
}

export function toWorkspaceResponse(workspace: Workspace): WorkspaceResponse {
  return {
    id: workspace.id,
    name: workspace.name,
    createdByStakeholderId: workspace.createdByStakeholderId,
    createdAt: workspace.createdAt,
  } satisfies WorkspaceResponse;
}
