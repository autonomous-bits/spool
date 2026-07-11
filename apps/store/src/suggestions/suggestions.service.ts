import {
  BadRequestException,
  ConflictException,
  ForbiddenException,
  Injectable,
  NotFoundException,
} from '@nestjs/common';
import type { SessionTokenClaims } from '../auth/session-token.service.js';
import { assertIsHumanActor } from '../domain/branch-lifecycle.js';
import { Suggestion } from '../domain/suggestion.js';
import { isSuggestionStatus } from '../domain/types/vocabulary/suggestion-status.js';
import { assertWorkspaceScope, WorkspaceScopeViolationError } from '../domain/workspace-scope.js';
import { resolveHumanActorContext } from '../auth/resolve-human-actor.helper.js';
import type { BranchResponse } from '../branches/branch-response.dto.js';
import { toBranchResponse } from '../branches/branch-response.dto.js';
import { StakeholderRepository } from '../persistence/stakeholder.repository.js';
import { SuggestionRepository } from '../persistence/suggestion.repository.js';
import { WorkspaceRepository } from '../persistence/workspace.repository.js';
import { toSuggestionResponse, type SuggestionResponse } from './suggestion-response.dto.js';
import type { AcceptSuggestionRequest } from './accept-suggestion-request.dto.js';
import type { CreateSuggestionRequest } from './create-suggestion-request.dto.js';

const FOREIGN_KEY_VIOLATION = '23503';
const UNIQUE_VIOLATION = '23505';

function isPgErrorWithCode(error: unknown, code: string): boolean {
  return (
    typeof error === 'object' &&
    error !== null &&
    'code' in error &&
    (error).code === code
  );
}

/**
 * Application service for suggestion submission and acceptance (Meridian IDEA-27/IDEA-28/
 * IDEA-49), sitting between the HTTP controller and the `SuggestionRepository`/
 * `StakeholderRepository` persistence layers. Submission is a delegated-actor operation
 * (Meridian IDEA-9/IDEA-75): the server always assigns `submittedByActorKind: 'delegated'`,
 * never trusting a client-supplied value. Acceptance is human-only (Meridian IDEA-75): the
 * server always assigns `kind: 'human'` from verified session-token claims.
 *
 * G11 SG5 (Meridian IDEA-98/IDEA-100): `create`/`findAll`/`findById` sit on the delegated,
 * tokenless auth tier (the caller-declared `stakeholderId` is validated against
 * `workspace_memberships`); `accept`/`reject` are already token-gated (human-only), so they sit
 * on the token tier and additionally validate `X-Workspace-Id` against the token's
 * `workspaceId` claim, mirroring `BranchesService`.
 */
@Injectable()
export class SuggestionsService {
  constructor(
    private readonly suggestionRepository: SuggestionRepository,
    private readonly stakeholderRepository: StakeholderRepository,
    private readonly workspaceRepository: WorkspaceRepository,
  ) {}

  private async assertScope(
    headerWorkspaceId: string | null | undefined,
    claims: SessionTokenClaims,
  ): Promise<string> {
    const isMember =
      headerWorkspaceId === null ||
      headerWorkspaceId === undefined ||
      headerWorkspaceId.trim().length === 0
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
    request: CreateSuggestionRequest,
    headerWorkspaceId: string | null | undefined,
    claims: SessionTokenClaims,
  ): Promise<SuggestionResponse> {
    const workspaceId = await this.assertScope(headerWorkspaceId, claims);

    let suggestion: Suggestion;
    try {
      suggestion = new Suggestion({
        workspaceId,
        variant: request.variant,
        discipline: request.discipline,
        submittedByStakeholderId: request.stakeholderId,
        submittedByActorKind: 'delegated',
      });
    } catch (error) {
      // Domain invariants (blank label/content/from/to labels, same from/to, invalid vocab)
      // surfaced as 400s, not 500s.
      const message = error instanceof Error ? error.message : 'Invalid suggestion';
      throw new BadRequestException(message);
    }

    try {
      const created = await this.suggestionRepository.create(suggestion);
      return toSuggestionResponse(created);
    } catch (error) {
      if (isPgErrorWithCode(error, FOREIGN_KEY_VIOLATION)) {
        throw new BadRequestException(`Unknown stakeholderId: ${request.stakeholderId}`);
      }
      throw error;
    }
  }

  /**
   * Accepts a pending suggestion, creating a linked draft branch (Meridian IDEA-27/IDEA-49).
   * `claims` must come from a verified session token (Meridian IDEA-75: human-only); the
   * resulting branch's discipline is taken from the suggestion, never the request body.
   */
  async accept(
    suggestionId: string,
    request: AcceptSuggestionRequest,
    claims: SessionTokenClaims,
    headerWorkspaceId: string | null | undefined,
  ): Promise<BranchResponse> {
    const workspaceId = await this.assertScope(headerWorkspaceId, claims);

    const actor = await resolveHumanActorContext(this.stakeholderRepository, claims, {
      requireDiscipline: false,
      actionDescription: 'accept a suggestion',
    });
    assertIsHumanActor(actor);

    let result;
    try {
      result = await this.suggestionRepository.accept(
        suggestionId,
        request.name,
        claims.stakeholderId,
        workspaceId,
      );
    } catch (error) {
      if (isPgErrorWithCode(error, UNIQUE_VIOLATION)) {
        throw new BadRequestException(`Branch name already active: ${request.name}`);
      }
      throw error;
    }

    switch (result.kind) {
      case 'not_found':
        throw new NotFoundException(`Suggestion ${suggestionId} not found`);
      case 'not_pending':
        throw new ConflictException(
          `Invalid suggestion lifecycle operation: expected pending suggestion, received a different status`,
        );
      case 'accepted':
        return toBranchResponse(result.branch);
    }
  }

  /**
   * Rejects a pending suggestion (Meridian IDEA-27, G07 SG3). `claims` must come from a verified
   * session token (Meridian IDEA-75: human-only), matching `accept`'s auth requirements. Never
   * creates a branch.
   */
  async reject(
    suggestionId: string,
    claims: SessionTokenClaims,
    headerWorkspaceId: string | null | undefined,
  ): Promise<SuggestionResponse> {
    const workspaceId = await this.assertScope(headerWorkspaceId, claims);

    const actor = await resolveHumanActorContext(this.stakeholderRepository, claims, {
      requireDiscipline: false,
      actionDescription: 'reject a suggestion',
    });
    assertIsHumanActor(actor);

    const result = await this.suggestionRepository.reject(
      suggestionId,
      claims.stakeholderId,
      workspaceId,
    );

    switch (result.kind) {
      case 'not_found':
        throw new NotFoundException(`Suggestion ${suggestionId} not found`);
      case 'not_pending':
        throw new ConflictException(
          `Invalid suggestion lifecycle operation: expected pending suggestion, received a different status`,
        );
      case 'rejected': {
        const rejected = await this.suggestionRepository.findById(suggestionId, workspaceId);
        if (rejected === undefined) {
          throw new Error(
            `SuggestionsService.reject: suggestion ${suggestionId} vanished after rejection`,
          );
        }
        return toSuggestionResponse(rejected);
      }
    }
  }

  /**
   * Reads a single suggestion by id (G07 SG3). G16 SG2 (Meridian IDEA-139) requires a verified
   * session token whose claims are both workspace-claim-matched and currently a member.
   */
  async findById(
    suggestionId: string,
    claims: SessionTokenClaims,
    headerWorkspaceId: string | null | undefined,
  ): Promise<SuggestionResponse> {
    const workspaceId = await this.assertScope(headerWorkspaceId, claims);

    const suggestion = await this.suggestionRepository.findById(suggestionId, workspaceId);
    if (suggestion === undefined) {
      throw new NotFoundException(`Suggestion ${suggestionId} not found`);
    }
    return toSuggestionResponse(suggestion);
  }

  /**
   * Lists suggestions, optionally filtered to a single status, ordered oldest-first (G07 SG3).
   * G16 SG2 (Meridian IDEA-139) requires a verified session token whose claims are both
   * workspace-claim-matched and currently a member.
   */
  async findAll(
    status: string | undefined,
    claims: SessionTokenClaims,
    headerWorkspaceId: string | null | undefined,
  ): Promise<SuggestionResponse[]> {
    const workspaceId = await this.assertScope(headerWorkspaceId, claims);

    if (status !== undefined && !isSuggestionStatus(status)) {
      throw new BadRequestException(`Invalid status filter: ${status}`);
    }
    const suggestions = await this.suggestionRepository.findAll(status, workspaceId);
    return suggestions.map(toSuggestionResponse);
  }
}
