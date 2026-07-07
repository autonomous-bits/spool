import { Body, Controller, Get, Headers, Param, Post } from '@nestjs/common';
import { parseCreateVerificationSignalRequest } from './create-verification-signal-request.dto.js';
import type { VerificationSignalResponse } from './verification-signal-response.dto.js';
import { VerificationSignalsService } from './verification-signals.service.js';

/**
 * G09 SG1 exposes submission and reads only (Meridian IDEA-21/IDEA-43/IDEA-31); both routes are
 * deliberately unauthenticated (mirrors `POST /suggestions`/`GET /suggestions` precedent) beyond
 * the mandatory `X-Workspace-Id` header (G11 SG5). Nests under the existing `branches` path,
 * mirroring `POST /suggestions/:id/accept` returning a cross-domain resource -- kept in its own
 * module/controller rather than folded into `BranchesController`, per the existing per-domain
 * module layout (branches/, suggestions/).
 */
@Controller('branches')
export class VerificationSignalsController {
  constructor(private readonly verificationSignals: VerificationSignalsService) {}

  @Post(':id/verification-signals')
  async create(
    @Param('id') id: string,
    @Body() body: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<VerificationSignalResponse> {
    const request = parseCreateVerificationSignalRequest(body);
    return this.verificationSignals.create(id, request, workspaceId);
  }

  @Get(':id/verification-signals')
  async findAll(
    @Param('id') id: string,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<VerificationSignalResponse[]> {
    return this.verificationSignals.findAllForBranch(id, workspaceId);
  }
}
