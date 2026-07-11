import {
  BadRequestException,
  ConflictException,
  ForbiddenException,
  Injectable,
  NotFoundException,
} from '@nestjs/common';
import type { SessionTokenClaims } from '../auth/session-token.service.js';
import { resolveHumanActorContext } from '../auth/resolve-human-actor.helper.js';
import {
  BranchLifecycleError,
  assertDraftStatus,
  assertIsHumanActor,
  assertMergeableStatus,
  assertRejectableStatus,
  assertSubmitDiscipline,
  assertSubmittedStatus,
} from '../domain/branch-lifecycle.js';
import { Branch } from '../domain/branch.js';
import type { HumanActorContext } from '../domain/types/actor/actor-context.js';
import { assertWorkspaceScope, WorkspaceScopeViolationError } from '../domain/workspace-scope.js';
import { BranchRepository } from '../persistence/branch.repository.js';
import { StakeholderRepository } from '../persistence/stakeholder.repository.js';
import { WorkspaceRepository } from '../persistence/workspace.repository.js';
import { toBranchResponse, type BranchResponse } from './branch-response.dto.js';
import type { CreateBranchRequest } from './create-branch-request.dto.js';

const FOREIGN_KEY_VIOLATION = '23503';
const UNIQUE_VIOLATION = '23505';
const NON_DRAFT_BRANCH_MESSAGE =
  'Invalid branch lifecycle operation: expected draft branch, received non-draft';

function isPgErrorWithCode(error: unknown, code: string): boolean {
  return (
    typeof error === 'object' &&
    error !== null &&
    'code' in error &&
    (error).code === code
  );
}

function toLifecycleConflict(error: BranchLifecycleError): ConflictException {
  return new ConflictException(error.message);
}

/**
 * Application service for branch creation, submission, and retrieval (Meridian IDEA-52/IDEA-34),
 * sitting between the HTTP controller and the persistence/domain layers.
 *
 * G17 SG2 (Meridian IDEA-24/IDEA-17, mirroring G16 SG5's IDEA-139 pattern): every route sits on
 * the single-tier, session-token-verified auth model — the request's `X-Workspace-Id` header is
 * validated against a `workspace_memberships` row for `claims.stakeholderId` (the verified
 * token's stakeholder, never a client-supplied value), and branch authorship attribution is
 * likewise derived from `claims.stakeholderId`.
 */
@Injectable()
export class BranchesService {
  constructor(
    private readonly branchRepository: BranchRepository,
    private readonly stakeholderRepository: StakeholderRepository,
    private readonly workspaceRepository: WorkspaceRepository,
  ) {}

  private async assertScope(
    headerWorkspaceId: string | null | undefined,
    claims: SessionTokenClaims,
  ): Promise<string> {
    const isMember =
      headerWorkspaceId === null || headerWorkspaceId === undefined || headerWorkspaceId.trim().length === 0
        ? false
        : await this.workspaceRepository.isMember(headerWorkspaceId, claims.stakeholderId);

    try {
      assertWorkspaceScope(headerWorkspaceId, { workspaceIdClaim: claims.workspaceId, isMember });
    } catch (error) {
      if (error instanceof WorkspaceScopeViolationError) {
        throw new ForbiddenException(error.message);
      }
      throw error;
    }

    return headerWorkspaceId;
  }

  async create(
    request: CreateBranchRequest,
    headerWorkspaceId: string | null | undefined,
    claims: SessionTokenClaims,
  ): Promise<BranchResponse> {
    const workspaceId = await this.assertScope(headerWorkspaceId, claims);

    let branch: Branch;
    try {
      branch = new Branch({
        workspaceId,
        name: request.name,
        discipline: request.discipline,
        createdByStakeholderId: claims.stakeholderId,
      });
    } catch (error) {
      // Domain invariants (blank name/discipline, etc.) surfaced as 400s, not 500s.
      const message = error instanceof Error ? error.message : 'Invalid branch';
      throw new BadRequestException(message);
    }

    try {
      const created = await this.branchRepository.create(branch);
      return toBranchResponse(created);
    } catch (error) {
      if (isPgErrorWithCode(error, FOREIGN_KEY_VIOLATION)) {
        throw new BadRequestException(`Unknown stakeholderId: ${claims.stakeholderId}`);
      }
      if (isPgErrorWithCode(error, UNIQUE_VIOLATION)) {
        throw new BadRequestException(`Branch name already active: ${request.name}`);
      }
      throw error;
    }
  }

  async submit(
    branchId: string,
    headerWorkspaceId: string | null | undefined,
    claims: SessionTokenClaims,
  ): Promise<BranchResponse> {
    const workspaceId = await this.assertScope(headerWorkspaceId, claims);

    const actor = await resolveHumanActorContext(this.stakeholderRepository, claims, {
      requireDiscipline: true,
      actionDescription: 'submit a branch',
    });

    const branch = await this.branchRepository.findById(branchId, workspaceId);
    if (branch === undefined) {
      throw new NotFoundException(`Branch ${branchId} not found`);
    }

    try {
      assertIsHumanActor(actor);
      assertSubmitDiscipline(actor, branch);
      assertDraftStatus(branch);
    } catch (error) {
      if (error instanceof BranchLifecycleError) {
        throw toLifecycleConflict(error);
      }
      throw error;
    }

    const submitted = await this.branchRepository.submit(branchId, workspaceId);
    if (submitted === undefined) {
      throw new ConflictException(NON_DRAFT_BRANCH_MESSAGE);
    }

    return toBranchResponse(submitted);
  }

  async verify(
    branchId: string,
    headerWorkspaceId: string | null | undefined,
    claims: SessionTokenClaims,
  ): Promise<BranchResponse> {
    const workspaceId = await this.assertScope(headerWorkspaceId, claims);
    const actor = await this.resolveActorForVerification(branchId, workspaceId, claims);

    try {
      assertIsHumanActor(actor);
      assertSubmittedStatus(actor.branch);
    } catch (error) {
      if (error instanceof BranchLifecycleError) {
        throw toLifecycleConflict(error);
      }
      throw error;
    }

    const verified = await this.branchRepository.verify(branchId, workspaceId);
    if (verified === undefined) {
      throw new ConflictException(
        'Invalid branch lifecycle operation: expected submitted branch, received a different status',
      );
    }

    return toBranchResponse(verified);
  }

  async reject(
    branchId: string,
    headerWorkspaceId: string | null | undefined,
    claims: SessionTokenClaims,
  ): Promise<BranchResponse> {
    const workspaceId = await this.assertScope(headerWorkspaceId, claims);
    const actor = await this.resolveActorForVerification(branchId, workspaceId, claims);

    try {
      assertIsHumanActor(actor);
      assertRejectableStatus(actor.branch);
    } catch (error) {
      if (error instanceof BranchLifecycleError) {
        throw toLifecycleConflict(error);
      }
      throw error;
    }

    const rejected = await this.branchRepository.reject(branchId, workspaceId);
    if (rejected === undefined) {
      throw new ConflictException(
        'Invalid branch lifecycle operation: expected submitted or verified branch, received a different status',
      );
    }

    return toBranchResponse(rejected);
  }

  /**
   * Merges a verified branch into mainline (Meridian IDEA-40's verified -> merged transition,
   * IDEA-74's merge-lineage/provenance shape, IDEA-46's conflict gate scoped per G06 OQ2). Reuses
   * the same discipline-agnostic, human-only actor resolution as verify/reject (matches G05's
   * resolved "merging authority" interpretation of IDEA-11: any human stakeholder may merge,
   * regardless of discipline).
   */
  async merge(
    branchId: string,
    headerWorkspaceId: string | null | undefined,
    claims: SessionTokenClaims,
  ): Promise<BranchResponse> {
    const workspaceId = await this.assertScope(headerWorkspaceId, claims);
    const actor = await this.resolveActorForVerification(branchId, workspaceId, claims);

    try {
      assertIsHumanActor(actor);
      assertMergeableStatus(actor.branch);
    } catch (error) {
      if (error instanceof BranchLifecycleError) {
        throw toLifecycleConflict(error);
      }
      throw error;
    }

    const result = await this.branchRepository.merge(branchId, workspaceId, claims.stakeholderId);
    if (result === undefined) {
      throw new ConflictException(
        'Invalid branch lifecycle operation: expected verified branch, received a different status',
      );
    }
    if (result.kind === 'conflict') {
      throw new ConflictException(result.reason);
    }

    return toBranchResponse(result.branch);
  }

  /**
   * Shared verify/reject/merge preamble (Meridian IDEA-81): looks up the branch first (404 before
   * any actor/status check), then resolves the acting stakeholder. Unlike submit(), a null
   * discipline is accepted here — verify/reject/merge are discipline-agnostic per this goal's
   * resolved question.
   */
  private async resolveActorForVerification(
    branchId: string,
    workspaceId: string,
    claims: SessionTokenClaims,
  ): Promise<HumanActorContext & { branch: Branch }> {
    const branch = await this.branchRepository.findById(branchId, workspaceId);
    if (branch === undefined) {
      throw new NotFoundException(`Branch ${branchId} not found`);
    }

    const actor = await resolveHumanActorContext(this.stakeholderRepository, claims, {
      requireDiscipline: false,
      actionDescription: 'verify, reject, or merge a branch',
    });

    return { ...actor, branch };
  }

  async findById(
    id: string,
    headerWorkspaceId: string | null | undefined,
    claims: SessionTokenClaims,
  ): Promise<BranchResponse> {
    const workspaceId = await this.assertScope(headerWorkspaceId, claims);

    const branch = await this.branchRepository.findById(id, workspaceId);
    if (branch === undefined) {
      throw new NotFoundException(`Branch ${id} not found`);
    }
    return toBranchResponse(branch);
  }
}
