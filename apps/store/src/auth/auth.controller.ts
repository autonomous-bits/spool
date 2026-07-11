import { Controller, Get, Query, Redirect } from '@nestjs/common';
import { AuthService } from './auth.service.js';
import { toSessionTokenResponse, type SessionTokenResponse } from './session-token-response.dto.js';

interface LoginRedirect {
  url: string;
  statusCode: number;
}

/**
 * GitHub OAuth login/callback endpoints (Meridian IDEA-81). Human-only per IDEA-9/IDEA-40/
 * IDEA-57 — there is deliberately no MCP-facing equivalent (see G04's scope notes).
 */
@Controller('auth/github')
export class AuthController {
  constructor(private readonly authService: AuthService) {}

  /**
   * `workspaceId` is optional (Meridian IDEA-92/IDEA-101, G11 SG2): supplying it requests a
   * workspace-bound token (subject to membership at the callback); omitting it is only valid for
   * a stakeholder with zero workspace memberships, who receives a workspace-less bootstrap token.
   */
  @Get('login')
  @Redirect()
  login(@Query('workspaceId') workspaceId?: string): LoginRedirect {
    const url = this.authService.buildLoginRedirectUrl(workspaceId);
    return { url, statusCode: 302 };
  }

  @Get('callback')
  async callback(
    @Query('code') code: unknown,
    @Query('state') state: unknown,
  ): Promise<SessionTokenResponse> {
    const sessionToken = await this.authService.handleCallback(code, state);
    return toSessionTokenResponse(sessionToken);
  }
}
