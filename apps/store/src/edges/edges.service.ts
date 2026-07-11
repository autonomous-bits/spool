import { BadRequestException, ConflictException, ForbiddenException, Injectable, NotFoundException } from '@nestjs/common';
import { Edge } from '../domain/edge.js';
import { assertWorkspaceScope, WorkspaceScopeViolationError } from '../domain/workspace-scope.js';
import { BranchRepository } from '../persistence/branch.repository.js';
import { ChunkRepository } from '../persistence/chunk.repository.js';
import { EdgeRepository } from '../persistence/edge.repository.js';
import { WorkspaceRepository } from '../persistence/workspace.repository.js';
import { toEdgeResponse, type EdgeResponse } from './edge-response.dto.js';
import type { CreateEdgeRequest } from './create-edge-request.dto.js';

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
 * Application service for typed edge creation and retrieval (Meridian IDEA-36/IDEA-37/IDEA-38),
 * sitting between the HTTP controller and the `EdgeRepository`/`ChunkRepository`/`BranchRepository`
 * persistence layers. Enforces branch-local endpoint existence and the single-active-edge
 * invariant; this goal's write path only ever produces 'active' edges.
 *
 * G11 SG4 (Meridian IDEA-98/IDEA-100): both `create` and `findById` sit on the delegated,
 * tokenless auth tier — the request's `X-Workspace-Id` header is validated against a
 * `workspace_memberships` row for the caller-declared `stakeholderId`, not a session token.
 */
@Injectable()
export class EdgesService {
  constructor(
    private readonly edgeRepository: EdgeRepository,
    private readonly chunkRepository: ChunkRepository,
    private readonly branchRepository: BranchRepository,
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

  async create(request: CreateEdgeRequest, headerWorkspaceId: string | null | undefined): Promise<EdgeResponse> {
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

    const fromChunk = await this.chunkRepository.findByLabel(request.fromChunkLabel, branchId, workspaceId);
    if (fromChunk === undefined) {
      throw new NotFoundException(
        `Chunk with label ${request.fromChunkLabel} not found in this scope`,
      );
    }

    const toChunk = await this.chunkRepository.findByLabel(request.toChunkLabel, branchId, workspaceId);
    if (toChunk === undefined) {
      throw new NotFoundException(
        `Chunk with label ${request.toChunkLabel} not found in this scope`,
      );
    }

    let edge: Edge;
    try {
      edge = new Edge({
        workspaceId,
        fromChunkLabel: request.fromChunkLabel,
        toChunkLabel: request.toChunkLabel,
        type: request.type,
        discipline: request.discipline,
        createdByStakeholderId: request.stakeholderId,
        ...(branchId === undefined ? {} : { branchId, originBranchId: branchId }),
      });
    } catch (error) {
      // Domain invariants (blank labels, same from/to, invalid vocab) surfaced as 400s, not 500s.
      const message = error instanceof Error ? error.message : 'Invalid edge';
      throw new BadRequestException(message);
    }

    try {
      const created = await this.edgeRepository.create(edge);
      return toEdgeResponse(created);
    } catch (error) {
      if (isPgErrorWithCode(error, FOREIGN_KEY_VIOLATION)) {
        throw new BadRequestException(`Unknown stakeholderId: ${request.stakeholderId}`);
      }
      if (isPgErrorWithCode(error, UNIQUE_VIOLATION)) {
        throw new ConflictException(
          `An active ${request.type} edge already exists from ${request.fromChunkLabel} to ${request.toChunkLabel} in this scope`,
        );
      }
      throw error;
    }
  }

  async findById(
    id: string,
    headerWorkspaceId: string | null | undefined,
    stakeholderId: string,
  ): Promise<EdgeResponse> {
    const workspaceId = await this.assertDelegatedScope(headerWorkspaceId, stakeholderId);

    const edge = await this.edgeRepository.findById(id, workspaceId);
    if (edge === undefined) {
      throw new NotFoundException(`Edge ${id} not found`);
    }
    return toEdgeResponse(edge);
  }
}
