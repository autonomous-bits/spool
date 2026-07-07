import { randomUUID } from 'node:crypto';
import type { Discipline } from './types/vocabulary/discipline.js';
import { parseDiscipline } from './types/vocabulary/discipline.js';
import type { SuggestionStatus } from './types/vocabulary/suggestion-status.js';
import { parseSuggestionStatus } from './types/vocabulary/suggestion-status.js';
import type { EdgeType } from './types/vocabulary/edge-type.js';
import { parseEdgeType } from './types/vocabulary/edge-type.js';
import type { ActorKind } from './types/actor/actor-kind.js';
import { parseActorKind } from './types/actor/actor-kind.js';

/**
 * The chunk-shaped or edge-shaped content a suggestion proposes, per Meridian IDEA-49's
 * `check_suggestion_type` discriminator (label+content XOR from/to/relationshipType). Modeled as
 * a discriminated union so only one of the two shapes is ever representable in memory.
 */
export type SuggestionVariant =
  | { kind: 'chunk'; label: string; content: string }
  | { kind: 'edge'; fromChunkLabel: string; toChunkLabel: string; relationshipType: EdgeType };

export interface SuggestionProps {
  id?: string;
  workspaceId: string;
  variant: SuggestionVariant;
  discipline: Discipline;
  status?: SuggestionStatus;
  submittedByStakeholderId: string;
  submittedByActorKind: ActorKind;
  decidedByStakeholderId?: string;
  decidedAt?: Date;
  createdAt?: Date;
  updatedAt?: Date;
}

function requireNonBlank(value: string, fieldName: string): string {
  if (value.trim().length === 0) {
    throw new TypeError(`Suggestion ${fieldName} must not be empty or blank`);
  }
  return value;
}

function validateVariant(variant: SuggestionVariant): SuggestionVariant {
  if (variant.kind === 'chunk') {
    return {
      kind: 'chunk',
      label: requireNonBlank(variant.label, 'label'),
      content: requireNonBlank(variant.content, 'content'),
    };
  }

  const fromChunkLabel = requireNonBlank(variant.fromChunkLabel, 'fromChunkLabel');
  const toChunkLabel = requireNonBlank(variant.toChunkLabel, 'toChunkLabel');
  if (fromChunkLabel === toChunkLabel) {
    throw new TypeError('Suggestion fromChunkLabel and toChunkLabel must not be the same label');
  }

  return {
    kind: 'edge',
    fromChunkLabel,
    toChunkLabel,
    relationshipType: parseEdgeType(variant.relationshipType),
  };
}

/**
 * Suggestion entity: a chunk or edge modification proposed by a delegated actor on behalf of a
 * human stakeholder, pending human review (Meridian IDEA-27/IDEA-28/IDEA-49). This goal's write
 * path only ever produces `status: 'pending'` with `submittedByActorKind: 'delegated'`;
 * accept/reject transitions (setting decidedByStakeholderId/decidedAt) are out of scope here.
 */
export class Suggestion {
  readonly id: string;
  readonly workspaceId: string;
  readonly variant: SuggestionVariant;
  readonly discipline: Discipline;
  readonly status: SuggestionStatus;
  readonly submittedByStakeholderId: string;
  readonly submittedByActorKind: ActorKind;
  readonly decidedByStakeholderId?: string;
  readonly decidedAt?: Date;
  readonly createdAt: Date;
  readonly updatedAt: Date;

  constructor(props: SuggestionProps) {
    this.workspaceId = requireNonBlank(props.workspaceId, 'workspaceId');
    this.variant = validateVariant(props.variant);
    this.discipline = parseDiscipline(props.discipline);

    if (props.submittedByStakeholderId.trim().length === 0) {
      throw new TypeError('Suggestion requires a non-blank submittedByStakeholderId');
    }

    this.id = props.id ?? randomUUID();
    this.status = props.status === undefined ? 'pending' : parseSuggestionStatus(props.status);
    this.submittedByStakeholderId = props.submittedByStakeholderId;
    this.submittedByActorKind = parseActorKind(props.submittedByActorKind);
    if (props.decidedByStakeholderId !== undefined) {
      this.decidedByStakeholderId = props.decidedByStakeholderId;
    }
    if (props.decidedAt !== undefined) {
      this.decidedAt = props.decidedAt;
    }
    this.createdAt = props.createdAt ?? new Date();
    this.updatedAt = props.updatedAt ?? this.createdAt;
  }
}
