import type { StakeholderDiscipline } from '../domain/stakeholder-discipline.js';
import type { Discipline } from '../domain/types/vocabulary/discipline.js';

export interface StakeholderDisciplineResponse {
  workspaceId: string;
  stakeholderId: string;
  discipline: Discipline;
  createdAt: string;
}

export function toStakeholderDisciplineResponse(
  stakeholderDiscipline: StakeholderDiscipline,
): StakeholderDisciplineResponse {
  return {
    workspaceId: stakeholderDiscipline.workspaceId,
    stakeholderId: stakeholderDiscipline.stakeholderId,
    discipline: stakeholderDiscipline.discipline,
    createdAt: stakeholderDiscipline.createdAt.toISOString(),
  } satisfies StakeholderDisciplineResponse;
}
