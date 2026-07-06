import {
  BadRequestException,
  ConflictException,
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
import { BranchRepository } from '../persistence/branch.repository.js';
import { StakeholderRepository } from '../persistence/stakeholder.repository.js';
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
    (error as { code: unknown }).code === code
  );
}

function toLifecycleConflict(error: BranchLifecycleError): ConflictException {
  return new ConflictException(error.message);
}

/**
 * Application service for branch creation, submission, and retrieval (Meridian IDEA-52/IDEA-34),
 * sitting between the HTTP controller and the persistence/domain layers.
 */
@Injectable()
export class BranchesService {
  constructor(
    private readonly branchRepository: BranchRepository,
    private readonly stakeholderRepository: StakeholderRepository,
  ) {}

  async create(request: CreateBranchRequest): Promise<BranchResponse> {
    let branch: Branch;
    try {
      branch = new Branch({
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

  async submit(branchId: string, claims: SessionTokenClaims): Promise<BranchResponse> {
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

    const branch = await this.branchRepository.findById(branchId);
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

    const submitted = await this.branchRepository.submit(branchId);
    if (submitted === undefined) {
      throw new ConflictException(NON_DRAFT_BRANCH_MESSAGE);
    }

    return toBranchResponse(submitted);
  }

  async verify(branchId: string, claims: SessionTokenClaims): Promise<BranchResponse> {
    const actor = await this.resolveActorForVerification(branchId, claims);

    try {
      assertIsHumanActor(actor);
      assertSubmittedStatus(actor.branch);
    } catch (error) {
      if (error instanceof BranchLifecycleError) {
        throw toLifecycleConflict(error);
      }
      throw error;
    }

    const verified = await this.branchRepository.verify(branchId);
    if (verified === undefined) {
      throw new ConflictException(
        'Invalid branch lifecycle operation: expected submitted branch, received a different status',
      );
    }

    return toBranchResponse(verified);
  }

  async reject(branchId: string, claims: SessionTokenClaims): Promise<BranchResponse> {
    const actor = await this.resolveActorForVerification(branchId, claims);

    try {
      assertIsHumanActor(actor);
      assertRejectableStatus(actor.branch);
    } catch (error) {
      if (error instanceof BranchLifecycleError) {
        throw toLifecycleConflict(error);
      }
      throw error;
    }

    const rejected = await this.branchRepository.reject(branchId);
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
  async merge(branchId: string, claims: SessionTokenClaims): Promise<BranchResponse> {
    const actor = await this.resolveActorForVerification(branchId, claims);

    try {
      assertIsHumanActor(actor);
      assertMergeableStatus(actor.branch);
    } catch (error) {
      if (error instanceof BranchLifecycleError) {
        throw toLifecycleConflict(error);
      }
      throw error;
    }

    const result = await this.branchRepository.merge(branchId, claims.stakeholderId);
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
    claims: SessionTokenClaims,
  ): Promise<HumanActorContext & { branch: Branch }> {
    const branch = await this.branchRepository.findById(branchId);
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

  async findById(id: string): Promise<BranchResponse> {
    const branch = await this.branchRepository.findById(id);
    if (branch === undefined) {
      throw new NotFoundException(`Branch ${id} not found`);
    }
    return toBranchResponse(branch);
  }
}
