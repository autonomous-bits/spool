import type { Branch } from '../domain/branch.js';
import type { BranchStatus } from '../domain/types/vocabulary/branch-status.js';
import type { Discipline } from '../domain/types/vocabulary/discipline.js';

/**
 * HTTP-facing shape of a persisted Branch, per Meridian IDEA-52/IDEA-34. Kept as an explicit
 * interface (rather than returning the `Branch` domain entity directly) so the API response
 * contract is typed independently of the domain entity's internal shape. `divergedAt`,
 * `submittedAt`, and `verifiedAt` are serialized as ISO-8601 strings (`submittedAt`/`verifiedAt`
 * are nullable until the branch has been submitted/verified, and are cleared back to null on
 * reject per Meridian IDEA-81). `mergedAt` is nullable until the branch has been merged into
 * mainline (Meridian IDEA-40/IDEA-74, G06).
 */
export interface BranchResponse {
  id: string;
  name: string;
  discipline: Discipline;
  status: BranchStatus;
  divergedAt: string;
  submittedAt: string | null;
  verifiedAt: string | null;
  mergedAt: string | null;
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
    verifiedAt: branch.verifiedAt?.toISOString() ?? null,
    mergedAt: branch.mergedAt?.toISOString() ?? null,
    createdAt: branch.createdAt,
    updatedAt: branch.updatedAt,
    createdByStakeholderId: branch.createdByStakeholderId,
  } satisfies BranchResponse;
}
