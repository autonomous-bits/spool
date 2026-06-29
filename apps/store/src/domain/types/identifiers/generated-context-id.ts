import { trimAndValidateIdentifier } from './identifier-validation.js';

export type GeneratedContextId = string & {
  readonly __tag: 'GeneratedContextId';
};

export function generatedContextId(value: string): GeneratedContextId {
  return trimAndValidateIdentifier(
    'GeneratedContextId',
    value,
  ) as GeneratedContextId;
}
