import type { Discipline } from './types/vocabulary/discipline.js';

export interface StakeholderDisciplineProps {
  workspaceId: string;
  stakeholderId: string;
  discipline: Discipline;
  createdAt?: Date;
}

/**
 * StakeholderDiscipline: one row of a stakeholder's per-workspace allow-list of disciplines,
 * per Meridian IDEA-142 (single active discipline chosen per-request, scoped per-workspace) and
 * IDEA-143 (new join table alongside the flat, unextended WorkspaceMembership from IDEA-95).
 * Mirrors WorkspaceMembership's flat style: no roles, no extra metadata beyond the allow-list
 * membership itself.
 */
export class StakeholderDiscipline {
  readonly workspaceId: string;
  readonly stakeholderId: string;
  readonly discipline: Discipline;
  readonly createdAt: Date;

  constructor(props: StakeholderDisciplineProps) {
    if (props.workspaceId.trim().length === 0) {
      throw new TypeError('StakeholderDiscipline requires a non-blank workspaceId');
    }
    if (props.stakeholderId.trim().length === 0) {
      throw new TypeError('StakeholderDiscipline requires a non-blank stakeholderId');
    }

    this.workspaceId = props.workspaceId;
    this.stakeholderId = props.stakeholderId;
    this.discipline = props.discipline;
    this.createdAt = props.createdAt ?? new Date();
  }
}
