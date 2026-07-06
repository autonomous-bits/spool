/**
 * Vocabulary: SuggestionStatus enum, per Meridian IDEA-27 (promoted): feedback suggestions
 * transition pending -> accepted/rejected, with every state change logged. This goal (G07 SG1)
 * only ever produces 'pending'; the other two values are modeled here so later goals (accept/
 * reject) can round-trip them.
 */
export type SuggestionStatus = 'pending' | 'accepted' | 'rejected';

const SUGGESTION_STATUSES: readonly SuggestionStatus[] = ['pending', 'accepted', 'rejected'];

export function isSuggestionStatus(value: unknown): value is SuggestionStatus {
  return typeof value === 'string' && (SUGGESTION_STATUSES as readonly string[]).includes(value);
}

export function parseSuggestionStatus(value: unknown): SuggestionStatus {
  if (!isSuggestionStatus(value)) {
    throw new TypeError(`Invalid SuggestionStatus: ${JSON.stringify(value)}`);
  }
  return value;
}
