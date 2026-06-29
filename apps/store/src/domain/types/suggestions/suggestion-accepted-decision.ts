import type { ActorContext } from '../actor/actor-context.js';
import { assertHumanActor } from '../errors/human-control-error.js';
import { VocabularyValidationError } from '../errors/vocabulary-validation-error.js';
import type { StakeholderId } from '../identifiers/stakeholder-id.js';
import type { SuggestionId } from '../identifiers/suggestion-id.js';
import type { WorkspaceId } from '../identifiers/workspace-id.js';
import type { Discipline } from '../vocabulary/discipline.js';

export interface SuggestionAcceptedDecision {
  readonly decision: 'accepted';
  readonly workspaceId: WorkspaceId;
  readonly suggestionId: SuggestionId;
  readonly decidedByStakeholderId: StakeholderId;
  readonly decidedAt: string;
  readonly feedbackBranchDiscipline: Discipline;
}

export function acceptSuggestion(
  actor: ActorContext,
  workspaceId: WorkspaceId,
  suggestionId: SuggestionId,
  feedbackBranchDiscipline: Discipline,
  decidedAt: string,
): SuggestionAcceptedDecision {
  assertHumanActor(actor, 'accept a suggestion');
  const trimmedAt = decidedAt.trim();
  if (
    !trimmedAt ||
    !/^\d{4}-\d{2}-\d{2}/.test(trimmedAt) ||
    Number.isNaN(Date.parse(trimmedAt))
  ) {
    throw new VocabularyValidationError(
      'SuggestionAcceptedDecision.decidedAt',
      'decidedAt must be a valid ISO-8601 date string',
    );
  }
  return Object.freeze({
    decision: 'accepted',
    workspaceId,
    suggestionId,
    decidedByStakeholderId: actor.stakeholderId,
    decidedAt: trimmedAt,
    feedbackBranchDiscipline,
  });
}
