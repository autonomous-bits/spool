import { Body, Controller, Get, Headers, Param, Post } from '@nestjs/common';
import { verifySessionClaims } from '../auth/session-claims.helper.js';
import { SessionTokenService } from '../auth/session-token.service.js';
import { parseCreateVerificationSignalRequest } from './create-verification-signal-request.dto.js';
import type { VerificationSignalResponse } from './verification-signal-response.dto.js';
import { VerificationSignalsService } from './verification-signals.service.js';

/**
 * Verification-signal routes nested under branches (Meridian IDEA-21/IDEA-31). Per Meridian
 * IDEA-139 every route now requires a verified bearer session token; `verifierName` remains
 * untrusted caller-supplied text while authenticated reporter identity is derived separately from
 * token claims.
 */
@Controller('branches')
export class VerificationSignalsController {
  constructor(
    private readonly verificationSignals: VerificationSignalsService,
    private readonly sessionTokenService: SessionTokenService,
  ) {}

  @Post(':id/verification-signals')
  async create(
    @Param('id') id: string,
    @Body() body: unknown,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<VerificationSignalResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    const request = parseCreateVerificationSignalRequest(body);
    return this.verificationSignals.create(id, request, workspaceId, claims);
  }

  @Get(':id/verification-signals')
  async findAll(
    @Param('id') id: string,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<VerificationSignalResponse[]> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    return this.verificationSignals.findAllForBranch(id, workspaceId, claims);
  }
}
