import { ConflictException, Injectable, NotFoundException } from '@nestjs/common';
import { BranchLifecycleError } from '../domain/branch-lifecycle.js';
import { VerificationSignalRepository } from '../persistence/verification-signal.repository.js';
import type { CreateVerificationSignalRequest } from './create-verification-signal-request.dto.js';
import {
  toVerificationSignalResponse,
  type VerificationSignalResponse,
} from './verification-signal-response.dto.js';

/**
 * Application service for verification-signal submission and reads (Meridian IDEA-21/IDEA-43/
 * IDEA-31), sitting between the HTTP controller and the `VerificationSignalRepository`
 * persistence layer. Submission is deliberately unauthenticated (mirrors `POST /suggestions`
 * precedent): IDEA-21's "agents, tools, or other humans" is broader than the registered-
 * stakeholder set, so there is no ActorContext/session-token to verify here.
 */
@Injectable()
export class VerificationSignalsService {
  constructor(private readonly verificationSignalRepository: VerificationSignalRepository) {}

  /**
   * Submits a verification signal against a branch. Only allowed while the branch is
   * `submitted` or `verified` (Meridian IDEA-20/IDEA-43); never mutates the branch's own status.
   */
  async create(
    branchId: string,
    request: CreateVerificationSignalRequest,
  ): Promise<VerificationSignalResponse> {
    const result = await this.verificationSignalRepository.create({
      branchId,
      verifierName: request.verifierName,
      status: request.status,
      ...(request.reason === undefined ? {} : { reason: request.reason }),
    });

    switch (result.kind) {
      case 'not_found':
        throw new NotFoundException(`Branch ${branchId} not found`);
      case 'not_reviewable':
        throw new ConflictException(
          new BranchLifecycleError(
            `expected submitted or verified branch, received ${result.branchStatus}`,
          ).message,
        );
      case 'created':
        return toVerificationSignalResponse(result.signal);
    }
  }

  /**
   * Lists verification signals for a branch ordered oldest-first (G09 SG1). Deliberately
   * unauthenticated, matching this codebase's existing GET /branches, GET /suggestions
   * precedent of unauthenticated reads.
   */
  async findAllForBranch(branchId: string): Promise<VerificationSignalResponse[]> {
    const signals = await this.verificationSignalRepository.findByBranchId(branchId);
    return signals.map(toVerificationSignalResponse);
  }
}
