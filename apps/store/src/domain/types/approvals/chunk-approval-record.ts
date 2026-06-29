import type { ActorContext } from '../actor/actor-context.js';
import { assertHumanActor } from '../errors/human-control-error.js';
import { VocabularyValidationError } from '../errors/vocabulary-validation-error.js';
import type { IdeaLabel } from '../identifiers/idea-label.js';
import type { StakeholderId } from '../identifiers/stakeholder-id.js';
import type { WorkspaceId } from '../identifiers/workspace-id.js';

export interface ChunkApprovalRecord {
  readonly workspaceId: WorkspaceId;
  readonly chunkLabel: IdeaLabel;
  readonly approvedByStakeholderId: StakeholderId;
  readonly approvedAt: string;
}

export function approveChunk(
  actor: ActorContext,
  workspaceId: WorkspaceId,
  chunkLabel: IdeaLabel,
  approvedAt: string,
): ChunkApprovalRecord {
  assertHumanActor(actor, 'approve a chunk');
  const trimmedAt = approvedAt.trim();
  if (
    !trimmedAt ||
    !/^\d{4}-\d{2}-\d{2}/.test(trimmedAt) ||
    Number.isNaN(Date.parse(trimmedAt))
  ) {
    throw new VocabularyValidationError(
      'ChunkApprovalRecord.approvedAt',
      'approvedAt must be a valid ISO-8601 date string',
    );
  }
  return Object.freeze({
    workspaceId,
    chunkLabel,
    approvedByStakeholderId: actor.stakeholderId,
    approvedAt: trimmedAt,
  });
}
