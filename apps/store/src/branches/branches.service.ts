import {
  BadRequestException,
  ConflictException,
  ForbiddenException,
  Injectable,
  NotFoundException,
} from '@nestjs/common';
import type { SessionTokenClaims } from '../auth/session-token.service.js';
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
import { isDiscipline } from '../domain/types/vocabulary/discipline.js';
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
 * G11 SG4 (Meridian IDEA-98/IDEA-100, two-tier auth): `create` and `findById` sit on the
 * delegated, tokenless tier (X-Workspace-Id validated against a workspace_memberships row for the
 * caller-declared stakeholderId). `submit`/`verify`/`reject`/`merge` already require a human
 * session token, so they additionally validate X-Workspace-Id against the token's workspaceId
 * claim (the token tier).
 */
@Injectable()
export class BranchesService {
  constructor(
    private readonly branchRepository: BranchRepository,
    private readonly stakeholderRepository: StakeholderRepository,
    private readonly workspaceRepository: WorkspaceRepository,
  ) {}

  private async assertDelegatedScope(
    headerWorkspaceId: string | null | undefined,
    stakeholderId: string,
  ): Promise<string> {
    const isMember =
      headerWorkspaceId === null || headerWorkspaceId === undefined || headerWorkspaceId.trim().length === 0
        ? false
        : await this.workspaceRepository.isMember(headerWorkspaceId, stakeholderId);

    try {
      assertWorkspaceScope(headerWorkspaceId, { tier: 'delegated', isMember });
    } catch (error) {
      if (error instanceof WorkspaceScopeViolationError) {
        throw new ForbiddenException(error.message);
      }
      throw error;
    }

    return headerWorkspaceId;
  }

  private assertTokenScope(headerWorkspaceId: string | null | undefined, claims: SessionTokenClaims): string {
    try {
      assertWorkspaceScope(headerWorkspaceId, { tier: 'token', workspaceIdClaim: claims.workspaceId });
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
  ): Promise<BranchResponse> {
    const workspaceId = await this.assertDelegatedScope(headerWorkspaceId, request.stakeholderId);

    let branch: Branch;
    try {
      branch = new Branch({
        workspaceId,
        name: request.name,
        discipline: request.discipline,
        createdByStakeholderId: request.stakeholderId,
      });
    } catch (error) {
      // Domain invariants (blank name/discipline/stakeholderId) surfaced as 400s, not 500s.
      const message = error instanceof Error ? error.message : 'Invalid branch';
      throw new BadRequestException(message);
    }

    try {
      const created = await this.branchRepository.create(branch);
      return toBranchResponse(created);
    } catch (error) {
      if (isPgErrorWithCode(error, FOREIGN_KEY_VIOLATION)) {
        throw new BadRequestException(`Unknown stakeholderId: ${request.stakeholderId}`);
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
    const workspaceId = this.assertTokenScope(headerWorkspaceId, claims);

    const stakeholder = await this.stakeholderRepository.findById(claims.stakeholderId);
    if (stakeholder === undefined || !isDiscipline(stakeholder.discipline)) {
      throw new BadRequestException(
        `Stakeholder ${claims.stakeholderId} must exist with a valid discipline to submit a branch`,
      );
    }

    const actor = {
      kind: 'human',
      stakeholderId: claims.stakeholderId,
      discipline: stakeholder.discipline,
    } satisfies HumanActorContext;

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
    const workspaceId = this.assertTokenScope(headerWorkspaceId, claims);
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
    const workspaceId = this.assertTokenScope(headerWorkspaceId, claims);
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
    const workspaceId = this.assertTokenScope(headerWorkspaceId, claims);
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

    const stakeholder = await this.stakeholderRepository.findById(claims.stakeholderId);
    if (stakeholder === undefined) {
      throw new BadRequestException(
        `Stakeholder ${claims.stakeholderId} must exist to verify, reject, or merge a branch`,
      );
    }

    const actor = {
      kind: 'human',
      stakeholderId: claims.stakeholderId,
      discipline: isDiscipline(stakeholder.discipline) ? stakeholder.discipline : null,
    } satisfies HumanActorContext;

    return { ...actor, branch };
  }

  async findById(
    id: string,
    headerWorkspaceId: string | null | undefined,
    stakeholderId: string,
  ): Promise<BranchResponse> {
    const workspaceId = await this.assertDelegatedScope(headerWorkspaceId, stakeholderId);

    const branch = await this.branchRepository.findById(id, workspaceId);
    if (branch === undefined) {
      throw new NotFoundException(`Branch ${id} not found`);
    }
    return toBranchResponse(branch);
  }
}
