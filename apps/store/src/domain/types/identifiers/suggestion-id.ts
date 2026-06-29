import { trimAndValidateIdentifier } from './identifier-validation.js';

export type SuggestionId = string & { readonly __tag: 'SuggestionId' };

export function suggestionId(value: string): SuggestionId {
  return trimAndValidateIdentifier('SuggestionId', value) as SuggestionId;
}
