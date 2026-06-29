import { trimAndValidateIdentifier } from './identifier-validation.js';

export type IdeaLabel = string & { readonly __tag: 'IdeaLabel' };

export function ideaLabel(value: string): IdeaLabel {
  return trimAndValidateIdentifier('IdeaLabel', value) as IdeaLabel;
}
