import { BadRequestException, Injectable, NotFoundException } from '@nestjs/common';
import { Branch } from '../domain/branch.js';
import { BranchRepository } from '../persistence/branch.repository.js';
import { toBranchResponse, type BranchResponse } from './branch-response.dto.js';
import type { CreateBranchRequest } from './create-branch-request.dto.js';

const FOREIGN_KEY_VIOLATION = '23503';
const UNIQUE_VIOLATION = '23505';

function isPgErrorWithCode(error: unknown, code: string): boolean {
  return (
    typeof error === 'object' &&
    error !== null &&
    'code' in error &&
    (error as { code: unknown }).code === code
  );
}

/**
 * Application service for branch creation and retrieval (Meridian IDEA-52/IDEA-34), sitting
 * between the HTTP controller and the `BranchRepository` persistence layer.
 */
@Injectable()
export class BranchesService {
  constructor(private readonly branchRepository: BranchRepository) {}

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

  async findById(id: string): Promise<BranchResponse> {
    const branch = await this.branchRepository.findById(id);
    if (branch === undefined) {
      throw new NotFoundException(`Branch ${id} not found`);
    }
    return toBranchResponse(branch);
  }
}
