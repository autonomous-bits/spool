import { BadRequestException, ConflictException, ForbiddenException, Injectable, NotFoundException } from '@nestjs/common';
import { Chunk } from '../domain/chunk.js';
import { assertWorkspaceScope, WorkspaceScopeViolationError } from '../domain/workspace-scope.js';
import { BranchRepository } from '../persistence/branch.repository.js';
import { ChunkRepository } from '../persistence/chunk.repository.js';
import { WorkspaceRepository } from '../persistence/workspace.repository.js';
import type { SearchChunksFilters, SearchChunksResult } from '../persistence/chunk.repository.js';
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
 * G11 SG4 (Meridian IDEA-98/IDEA-100): both `create` and `findById` sit on the delegated,
 * tokenless auth tier — the request's `X-Workspace-Id` header is validated against a
 * `workspace_memberships` row for the caller-declared `stakeholderId` (the same identity already
 * used for discipline attribution on these routes), not a session token.
 */
@Injectable()
export class ChunksService {
  constructor(
    private readonly chunkRepository: ChunkRepository,
    private readonly branchRepository: BranchRepository,
    private readonly workspaceRepository: WorkspaceRepository,
  ) {}

  /**
   * Resolves SG3's delegated-tier `WorkspaceScopeCheck` (an async membership lookup, since
   * `assertWorkspaceScope` itself stays pure/sync) and asserts it, mapping a violation to a 403.
   * A missing/blank header short-circuits the membership lookup (no workspace to look up).
   */
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

  async create(request: CreateChunkRequest, headerWorkspaceId: string | null | undefined): Promise<ChunkResponse> {
    const workspaceId = await this.assertDelegatedScope(headerWorkspaceId, request.stakeholderId);

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
        createdByStakeholderId: request.stakeholderId,
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
        throw new BadRequestException(`Unknown stakeholderId: ${request.stakeholderId}`);
      }
      throw error;
    }
  }

  async findById(
    id: string,
    headerWorkspaceId: string | null | undefined,
    stakeholderId: string,
  ): Promise<ChunkResponse> {
    const workspaceId = await this.assertDelegatedScope(headerWorkspaceId, stakeholderId);

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
  ): Promise<{ chunks: ChunkResponse[]; nextCursor: string | null }> {
    try {
      assertWorkspaceScope(filters.workspaceId, { tier: 'token', workspaceIdClaim: claims.workspaceId });
    } catch (error) {
      if (error instanceof WorkspaceScopeViolationError) {
        throw new ForbiddenException(error.message);
      }
      throw error;
    }

    if (filters.branchId !== undefined) {
      if (claims.discipline === null) {
        throw new BadRequestException('Discipline is required to query a branch-scoped chunk');
      }
      const branch = await this.branchRepository.findById(filters.branchId, filters.workspaceId);
      if (branch !== undefined && branch.discipline !== claims.discipline) {
        throw new ForbiddenException(
          `Branch discipline (${branch.discipline}) does not match token discipline (${claims.discipline})`,
        );
      }
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
  ): Promise<{ chunk: ChunkResponse; neighbours: NeighbourResponse[] }> {
    try {
      assertWorkspaceScope(headerWorkspaceId, { tier: 'token', workspaceIdClaim: claims.workspaceId });
    } catch (error) {
      if (error instanceof WorkspaceScopeViolationError) {
        throw new ForbiddenException(error.message);
      }
      throw error;
    }
    const workspaceId = claims.workspaceId;

    if (branchId !== undefined) {
      if (claims.discipline === null) {
        throw new BadRequestException('Discipline is required to query a branch-scoped chunk');
      }
      const branch = await this.branchRepository.findById(branchId, workspaceId);
      if (branch !== undefined && branch.discipline !== claims.discipline) {
        throw new ForbiddenException(
          `Branch discipline (${branch.discipline}) does not match token discipline (${claims.discipline})`,
        );
      }
    }

    const chunk = await this.chunkRepository.findById(id, workspaceId);
    if (chunk === undefined) {
      throw new NotFoundException(`Chunk ${id} not found`);
    }

    const neighbours = await this.chunkRepository.getNeighbourhood(chunk.label, workspaceId, depth, branchId);

    if (branchId !== undefined) {
      for (const neighbour of neighbours) {
        if (neighbour.discipline !== claims.discipline) {
          throw new ForbiddenException(
            `Neighbour chunk discipline (${neighbour.discipline}) does not match token discipline (${claims.discipline})`,
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
