import { BadRequestException } from '@nestjs/common';
import { isDiscipline } from '../domain/types/vocabulary/discipline.js';
import type { HumanActorContext } from '../domain/types/actor/actor-context.js';
import type { StakeholderRepository } from '../persistence/stakeholder.repository.js';
import type { SessionTokenClaims } from './session-token.service.js';

/**
 * Resolves a `HumanActorContext` for a write that requires an existing stakeholder (branch
 * submit/verify/reject/merge, suggestion accept/reject) — never from a client-supplied
 * `stakeholderId`/`discipline` body field, always from `claims.stakeholderId` (verified session
 * token) looked up against the `stakeholders` table (Meridian IDEA-139/G16 SG4). Previously
 * duplicated near-verbatim across `BranchesService` and `SuggestionsService`; centralized here so
 * a further copy isn't added.
 *
 * When `requireDiscipline` is `true` (branch submission, Meridian IDEA-11/G05), a stakeholder with
 * no valid discipline is treated the same as a missing stakeholder. When `false` (verify/reject/
 * merge/accept/reject, discipline-agnostic per G05/G07's resolved questions), a missing/invalid
 * discipline resolves to `null` rather than failing.
 */
export async function resolveHumanActorContext(
  stakeholderRepository: StakeholderRepository,
  claims: SessionTokenClaims,
  options: { requireDiscipline: boolean; actionDescription: string },
): Promise<HumanActorContext> {
  const stakeholder = await stakeholderRepository.findById(claims.stakeholderId);
  const discipline = stakeholder !== undefined && isDiscipline(stakeholder.discipline)
    ? stakeholder.discipline
    : null;

  if (stakeholder === undefined || (options.requireDiscipline && discipline === null)) {
    const disciplineClause = options.requireDiscipline ? ' with a valid discipline' : '';
    throw new BadRequestException(
      `Stakeholder ${claims.stakeholderId} must exist${disciplineClause} to ${options.actionDescription}`,
    );
  }

  return {
    kind: 'human',
    stakeholderId: claims.stakeholderId,
    discipline,
  };
}
