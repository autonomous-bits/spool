import type { Suggestion } from '../domain/suggestion.js';
import type { ActorKind } from '../domain/types/actor/actor-kind.js';
import type { Discipline } from '../domain/types/vocabulary/discipline.js';
import type { EdgeType } from '../domain/types/vocabulary/edge-type.js';
import type { SuggestionStatus } from '../domain/types/vocabulary/suggestion-status.js';

/**
 * HTTP-facing shape of a persisted Suggestion, per Meridian IDEA-49. Kept as an explicit
 * interface (rather than returning the `Suggestion` domain entity directly) so the API response
 * contract is typed independently of the domain entity's internal shape. Exactly one of the
 * chunk-shaped (`label`/`content`) or edge-shaped (`fromChunkLabel`/`toChunkLabel`/
 * `relationshipType`) field groups is non-null, matching `check_suggestion_type`.
 * `decidedByStakeholderId`/`decidedAt` are `null` until the suggestion is accepted or rejected.
 */
export interface SuggestionResponse {
  id: string;
  label: string | null;
  content: string | null;
  fromChunkLabel: string | null;
  toChunkLabel: string | null;
  relationshipType: EdgeType | null;
  discipline: Discipline;
  status: SuggestionStatus;
  submittedByStakeholderId: string;
  submittedByActorKind: ActorKind;
  decidedByStakeholderId: string | null;
  decidedAt: string | null;
  createdAt: Date;
  updatedAt: Date;
}

export function toSuggestionResponse(suggestion: Suggestion): SuggestionResponse {
  const variant = suggestion.variant;

  return {
    id: suggestion.id,
    label: variant.kind === 'chunk' ? variant.label : null,
    content: variant.kind === 'chunk' ? variant.content : null,
    fromChunkLabel: variant.kind === 'edge' ? variant.fromChunkLabel : null,
    toChunkLabel: variant.kind === 'edge' ? variant.toChunkLabel : null,
    relationshipType: variant.kind === 'edge' ? variant.relationshipType : null,
    discipline: suggestion.discipline,
    status: suggestion.status,
    submittedByStakeholderId: suggestion.submittedByStakeholderId,
    submittedByActorKind: suggestion.submittedByActorKind,
    decidedByStakeholderId: suggestion.decidedByStakeholderId ?? null,
    decidedAt: suggestion.decidedAt?.toISOString() ?? null,
    createdAt: suggestion.createdAt,
    updatedAt: suggestion.updatedAt,
  } satisfies SuggestionResponse;
}
