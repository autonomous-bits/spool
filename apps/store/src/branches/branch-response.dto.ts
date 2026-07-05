import type { Branch } from '../domain/branch.js';
import type { BranchStatus } from '../domain/types/vocabulary/branch-status.js';
import type { Discipline } from '../domain/types/vocabulary/discipline.js';

/**
 * HTTP-facing shape of a persisted Branch, per Meridian IDEA-52/IDEA-34. Kept as an explicit
 * interface (rather than returning the `Branch` domain entity directly) so the API response
 * contract is typed independently of the domain entity's internal shape. `divergedAt` and
 * `submittedAt` are serialized as ISO-8601 strings (`submittedAt` is nullable when the branch has
 * not been submitted yet).
 */
export interface BranchResponse {
  id: string;
  name: string;
  discipline: Discipline;
  status: BranchStatus;
  divergedAt: string;
  submittedAt: string | null;
  createdAt: Date;
  updatedAt: Date;
  createdByStakeholderId: string;
}

export function toBranchResponse(branch: Branch): BranchResponse {
  return {
    id: branch.id,
    name: branch.name,
    discipline: branch.discipline,
    status: branch.status,
    divergedAt: branch.divergedAt.toISOString(),
    submittedAt: branch.submittedAt?.toISOString() ?? null,
    createdAt: branch.createdAt,
    updatedAt: branch.updatedAt,
    createdByStakeholderId: branch.createdByStakeholderId,
  } satisfies BranchResponse;
}
