import { trimAndValidateIdentifier } from './identifier-validation.js';

export type VerificationSignalId = string & { readonly __tag: 'VerificationSignalId' };

export function verificationSignalId(value: string): VerificationSignalId {
  return trimAndValidateIdentifier('VerificationSignalId', value) as VerificationSignalId;
}
