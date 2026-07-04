import type { ActorContext } from '../actor/actor-context.js';
import { assertHumanActor } from '../errors/human-control-error.js';
import { assertPendingSuggestion } from '../errors/suggestion-lifecycle-error.js';
import { VocabularyValidationError } from '../errors/vocabulary-validation-error.js';
import type { StakeholderId } from '../identifiers/stakeholder-id.js';
import type { SuggestionId } from '../identifiers/suggestion-id.js';
import type { WorkspaceId } from '../identifiers/workspace-id.js';
import type { SuggestionState } from '../lifecycle/suggestion-state.js';

export interface SuggestionRejectedDecision {
  readonly decision: 'rejected';
  readonly workspaceId: WorkspaceId;
  readonly suggestionId: SuggestionId;
  readonly decidedByStakeholderId: StakeholderId;
  readonly decidedAt: string;
}

export function rejectSuggestion(
  currentState: SuggestionState,
  actor: ActorContext,
  workspaceId: WorkspaceId,
  suggestionId: SuggestionId,
  decidedAt: string,
): SuggestionRejectedDecision {
  assertHumanActor(actor, 'reject a suggestion');
  assertPendingSuggestion(currentState, 'reject');
  const trimmedAt = decidedAt.trim();
  if (!trimmedAt || !/^\d{4}-\d{2}-\d{2}/.test(trimmedAt) || Number.isNaN(Date.parse(trimmedAt))) {
    throw new VocabularyValidationError(
      'SuggestionRejectedDecision.decidedAt',
      'decidedAt must be a valid ISO-8601 date string',
    );
  }
  return Object.freeze({
    decision: 'rejected',
    workspaceId,
    suggestionId,
    decidedByStakeholderId: actor.stakeholderId,
    decidedAt: trimmedAt,
  });
}
