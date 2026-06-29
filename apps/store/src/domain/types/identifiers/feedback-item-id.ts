import { trimAndValidateIdentifier } from './identifier-validation.js';

export type FeedbackItemId = string & { readonly __tag: 'FeedbackItemId' };

export function feedbackItemId(value: string): FeedbackItemId {
  return trimAndValidateIdentifier('FeedbackItemId', value) as FeedbackItemId;
}
