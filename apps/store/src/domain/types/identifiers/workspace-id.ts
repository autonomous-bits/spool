import { trimAndValidateIdentifier } from './identifier-validation.js';

export type WorkspaceId = string & { readonly __tag: 'WorkspaceId' };

export function workspaceId(value: string): WorkspaceId {
  return trimAndValidateIdentifier('WorkspaceId', value) as WorkspaceId;
}
