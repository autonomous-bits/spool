import { trimAndValidateIdentifier } from './identifier-validation.js';

export type StakeholderId = string & { readonly __tag: 'StakeholderId' };

export function stakeholderId(value: string): StakeholderId {
  return trimAndValidateIdentifier('StakeholderId', value) as StakeholderId;
}
