import { BadRequestException, ForbiddenException } from '@nestjs/common';
import { isDiscipline } from '../domain/types/vocabulary/discipline.js';
import type { HumanActorContext } from '../domain/types/actor/actor-context.js';
import type { StakeholderDisciplineRepository } from '../persistence/stakeholder-discipline.repository.js';
import type { StakeholderRepository } from '../persistence/stakeholder.repository.js';
import type { SessionTokenClaims } from './session-token.service.js';

export type ResolveHumanActorOptions =
  | { requireDiscipline: false; actionDescription: string }
  | { requireDiscipline: true; actionDescription: string; activeDiscipline: string | null | undefined };

/**
 * Resolves a `HumanActorContext` for a write that requires an existing stakeholder (branch
 * submit/verify/reject/merge, suggestion accept/reject) — never from a client-supplied
 * `stakeholderId` body field, always from `claims.stakeholderId` (verified session token) looked
 * up against the `stakeholders` table (Meridian IDEA-139/G16 SG4). Previously duplicated
 * near-verbatim across `BranchesService` and `SuggestionsService`; centralized here so a further
 * copy isn't added.
 *
 * G21 SG3 (Meridian IDEA-142/IDEA-143): discipline is no longer a token-baked, single-column
 * value. When `requireDiscipline` is `true` (branch submission, Meridian IDEA-11/G05), the caller
 * must supply a per-request `activeDiscipline`, which is validated to be a non-blank, closed-
 * vocabulary value (400 otherwise) and then checked against the caller's per-workspace allow-list
 * (`stakeholder_disciplines`, `StakeholderDisciplineRepository.isAllowed`) — 403 if the
 * stakeholder isn't allowed to act as that discipline in the token's workspace. When `false`
 * (verify/reject/merge/accept/reject, discipline-agnostic per G05/G07's resolved questions),
 * behavior is unchanged from before this rewrite: a missing/invalid legacy `stakeholders.discipline`
 * resolves to `null` rather than failing, and no allow-list check is performed.
 */
export async function resolveHumanActorContext(
  stakeholderRepository: StakeholderRepository,
  stakeholderDisciplineRepository: StakeholderDisciplineRepository,
  claims: SessionTokenClaims,
  options: ResolveHumanActorOptions,
): Promise<HumanActorContext> {
  const stakeholder = await stakeholderRepository.findById(claims.stakeholderId);
  if (stakeholder === undefined) {
    throw new BadRequestException(
      `Stakeholder ${claims.stakeholderId} must exist to ${options.actionDescription}`,
    );
  }

  if (!options.requireDiscipline) {
    const discipline = isDiscipline(stakeholder.discipline) ? stakeholder.discipline : null;
    return {
      kind: 'human',
      stakeholderId: claims.stakeholderId,
      discipline,
    };
  }

  const { activeDiscipline } = options;
  if (!isDiscipline(activeDiscipline)) {
    throw new BadRequestException(
      `A valid activeDiscipline is required to ${options.actionDescription}`,
    );
  }

  const workspaceId = claims.workspaceId;
  if (workspaceId === null || workspaceId.trim().length === 0) {
    throw new BadRequestException(
      `A workspace is required to ${options.actionDescription}`,
    );
  }

  const allowed = await stakeholderDisciplineRepository.isAllowed(
    workspaceId,
    claims.stakeholderId,
    activeDiscipline,
  );
  if (!allowed) {
    throw new ForbiddenException(
      `Stakeholder ${claims.stakeholderId} is not allowed to act as ${activeDiscipline} in workspace ${workspaceId}`,
    );
  }

  return {
    kind: 'human',
    stakeholderId: claims.stakeholderId,
    discipline: activeDiscipline,
  };
}
