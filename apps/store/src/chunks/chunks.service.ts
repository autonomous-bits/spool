import { BadRequestException, ConflictException, ForbiddenException, Injectable, NotFoundException } from '@nestjs/common';
import { resolveHumanActorContext } from '../auth/resolve-human-actor.helper.js';
import { Chunk } from '../domain/chunk.js';
import { assertWorkspaceScope, WorkspaceScopeViolationError } from '../domain/workspace-scope.js';
import { BranchRepository } from '../persistence/branch.repository.js';
import { ChunkRepository } from '../persistence/chunk.repository.js';
import { StakeholderDisciplineRepository } from '../persistence/stakeholder-discipline.repository.js';
import { StakeholderRepository } from '../persistence/stakeholder.repository.js';
import { WorkspaceRepository } from '../persistence/workspace.repository.js';
import type { SearchChunksFilters } from '../persistence/chunk.repository.js';
import type { SessionTokenClaims } from '../auth/session-token.service.js';
import { toChunkResponse, type ChunkResponse, type NeighbourResponse } from './chunk-response.dto.js';
import type { CreateChunkRequest } from './create-chunk-request.dto.js';

const FOREIGN_KEY_VIOLATION = '23503';

function isForeignKeyViolation(error: unknown): boolean {
  return (
    typeof error === 'object' &&
    error !== null &&
    'code' in error &&
    (error).code === FOREIGN_KEY_VIOLATION
  );
}

/**
 * Application service for chunk capture and retrieval (Meridian IDEA-52/IDEA-34), sitting between
 * the HTTP controller and the `ChunkRepository`/`BranchRepository` persistence layers.
 *
 * G16 SG5 (Meridian IDEA-139): every route sits on the single-tier, session-token-verified auth
 * model — the request's `X-Workspace-Id` header is validated against a `workspace_memberships`
 * row for `claims.stakeholderId` (the verified token's stakeholder, never a client-supplied
 * value), and chunk authorship attribution is likewise derived from `claims.stakeholderId`.
 */
@Injectable()
export class ChunksService {
  constructor(
    private readonly chunkRepository: ChunkRepository,
    private readonly branchRepository: BranchRepository,
    private readonly workspaceRepository: WorkspaceRepository,
    private readonly stakeholderDisciplineRepository: StakeholderDisciplineRepository,
    private readonly stakeholderRepository: StakeholderRepository,
  ) {}

  /**
   * Resolves the Meridian IDEA-139 single-tier `WorkspaceScopeCheck` (an async membership lookup,
   * since `assertWorkspaceScope` itself stays pure/sync) and asserts it, mapping a violation to a
   * 403. A missing/blank header short-circuits the membership lookup (no workspace to look up).
   */
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
    request: CreateChunkRequest,
    headerWorkspaceId: string | null | undefined,
    claims: SessionTokenClaims,
  ): Promise<ChunkResponse> {
    const workspaceId = await this.assertScope(headerWorkspaceId, claims);

    let branchId: string | undefined;

    if (request.branchId !== undefined) {
      const branch = await this.branchRepository.findById(request.branchId, workspaceId);
      if (branch === undefined) {
        throw new NotFoundException(`Branch ${request.branchId} not found`);
      }
      if (branch.discipline !== request.discipline) {
        throw new ConflictException(
          `Branch discipline (${branch.discipline}) does not match request discipline (${request.discipline})`,
        );
      }
      if (branch.status !== 'draft') {
        throw new ConflictException(`Branch ${branch.id} is not in draft status`);
      }
      branchId = branch.id;
    }

    let chunk: Chunk;
    try {
      chunk = new Chunk({
        workspaceId,
        label: request.label,
        content: request.content,
        discipline: request.discipline,
        chunkType: request.chunkType,
        contextKind: request.contextKind,
        createdByStakeholderId: claims.stakeholderId,
        ...(branchId === undefined ? {} : { branchId, originBranchId: branchId }),
      });
    } catch (error) {
      // Domain invariants (blank label/content, etc.) surfaced as 400s, not 500s.
      const message = error instanceof Error ? error.message : 'Invalid chunk';
      throw new BadRequestException(message);
    }

    try {
      const created = await this.chunkRepository.create(chunk);
      return toChunkResponse(created);
    } catch (error) {
      if (isForeignKeyViolation(error)) {
        throw new BadRequestException(`Unknown stakeholderId: ${claims.stakeholderId}`);
      }
      throw error;
    }
  }

  async findById(
    id: string,
    headerWorkspaceId: string | null | undefined,
    claims: SessionTokenClaims,
  ): Promise<ChunkResponse> {
    const workspaceId = await this.assertScope(headerWorkspaceId, claims);

    const chunk = await this.chunkRepository.findById(id, workspaceId);
    if (chunk === undefined) {
      throw new NotFoundException(`Chunk ${id} not found`);
    }
    return toChunkResponse(chunk);
  }

  async search(
    filters: SearchChunksFilters,
    limit: number,
    claims: SessionTokenClaims,
    cursor?: string,
    activeDiscipline?: string,
  ): Promise<{ chunks: ChunkResponse[]; nextCursor: string | null }> {
    await this.assertScope(filters.workspaceId, claims);

    if (filters.branchId !== undefined) {
      // G21 SG4 (Meridian IDEA-142/IDEA-143): the per-request `activeDiscipline` query param
      // replaces the interim branch-discipline stand-in — validated (400) and allow-list-checked
      // (403) via the shared `resolveHumanActorContext` resolution path, required whenever a
      // branch-scoped search is requested (regardless of whether the branch id itself resolves).
      await resolveHumanActorContext(
        this.stakeholderRepository,
        this.stakeholderDisciplineRepository,
        claims,
        {
          requireDiscipline: true,
          actionDescription: 'search chunks scoped to a branch',
          activeDiscipline,
        },
      );
    }

    const result = await this.chunkRepository.search(filters, limit, cursor);
    return {
      chunks: result.chunks.map(toChunkResponse),
      nextCursor: result.nextCursor,
    };
  }

  async getNeighbourhood(
    id: string,
    headerWorkspaceId: string | null | undefined,
    claims: SessionTokenClaims,
    depth: number,
    branchId?: string,
    activeDiscipline?: string,
  ): Promise<{ chunk: ChunkResponse; neighbours: NeighbourResponse[] }> {
    const workspaceId = await this.assertScope(headerWorkspaceId, claims);

    let branchDiscipline: string | undefined;
    if (branchId !== undefined) {
      // G21 SG4: see the identical comment in search() — the client-supplied `activeDiscipline`
      // is validated/allow-list-checked via `resolveHumanActorContext`. The branch's own
      // persisted discipline is looked up separately, purely to filter returned neighbours by
      // content discipline (unrelated to actor authorization).
      await resolveHumanActorContext(
        this.stakeholderRepository,
        this.stakeholderDisciplineRepository,
        claims,
        {
          requireDiscipline: true,
          actionDescription: 'view a branch-scoped chunk neighbourhood',
          activeDiscipline,
        },
      );

      const branch = await this.branchRepository.findById(branchId, workspaceId);
      if (branch !== undefined) {
        branchDiscipline = branch.discipline;
      }
    }

    const chunk = await this.chunkRepository.findById(id, workspaceId);
    if (chunk === undefined) {
      throw new NotFoundException(`Chunk ${id} not found`);
    }

    const neighbours = await this.chunkRepository.getNeighbourhood(chunk.label, workspaceId, depth, branchId);

    if (branchId !== undefined && branchDiscipline !== undefined) {
      for (const neighbour of neighbours) {
        if (neighbour.discipline !== branchDiscipline) {
          throw new ForbiddenException(
            `Neighbour chunk discipline (${neighbour.discipline}) does not match branch discipline (${branchDiscipline})`,
          );
        }
      }
    }

    return {
      chunk: toChunkResponse(chunk),
      neighbours,
    };
  }
}
