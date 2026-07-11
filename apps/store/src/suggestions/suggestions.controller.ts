import {
  Body,
  Controller,
  Get,
  Headers,
  Param,
  Post,
  Query,
} from '@nestjs/common';
import { SessionTokenService } from '../auth/session-token.service.js';
import { verifySessionClaims } from '../auth/session-claims.helper.js';
import type { BranchResponse } from '../branches/branch-response.dto.js';
import { parseAcceptSuggestionRequest } from './accept-suggestion-request.dto.js';
import type { SuggestionResponse } from './suggestion-response.dto.js';
import { parseCreateSuggestionRequest } from './create-suggestion-request.dto.js';
import { SuggestionsService } from './suggestions.service.js';

/**
 * SG1 exposes submission only (Meridian IDEA-28); SG2 adds human-only acceptance (Meridian
 * IDEA-27/IDEA-75). SG3 adds human-only rejection plus GET /suggestions and GET /suggestions/:id
 * reads, matching this codebase's existing GET /branches, GET /chunks, GET /edges precedent
 * (Meridian IDEA-27). G16 SG2 (Meridian IDEA-139) requires every route — including
 * create/findAll/findById — to present a verified session token: `X-Workspace-Id` must match the
 * token's `workspaceId` claim and the token's stakeholder must currently be a workspace member.
 */
@Controller('suggestions')
export class SuggestionsController {
  constructor(
    private readonly suggestions: SuggestionsService,
    private readonly sessionTokenService: SessionTokenService,
  ) {}

  @Post()
  async create(
    @Body() body: unknown,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<SuggestionResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    const request = parseCreateSuggestionRequest(body);
    return this.suggestions.create(request, workspaceId, claims);
  }

  @Post(':id/accept')
  async accept(
    @Param('id') id: string,
    @Body() body: unknown,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<BranchResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    const request = parseAcceptSuggestionRequest(body);
    return this.suggestions.accept(id, request, claims, workspaceId);
  }

  @Post(':id/reject')
  async reject(
    @Param('id') id: string,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<SuggestionResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    return this.suggestions.reject(id, claims, workspaceId);
  }

  @Get()
  async findAll(
    @Query('status') status: string | undefined,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<SuggestionResponse[]> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    return this.suggestions.findAll(status, claims, workspaceId);
  }

  @Get(':id')
  async findOne(
    @Param('id') id: string,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<SuggestionResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    return this.suggestions.findById(id, claims, workspaceId);
  }
}
