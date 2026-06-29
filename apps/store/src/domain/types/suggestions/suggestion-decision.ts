import type { SuggestionAcceptedDecision } from './suggestion-accepted-decision.js';
import type { SuggestionRejectedDecision } from './suggestion-rejected-decision.js';

export type SuggestionDecision =
  | SuggestionAcceptedDecision
  | SuggestionRejectedDecision;
