import { trimAndValidateIdentifier } from './identifier-validation.js';

export type BranchId = string & { readonly __tag: 'BranchId' };

export function branchId(value: string): BranchId {
  return trimAndValidateIdentifier('BranchId', value) as BranchId;
}
