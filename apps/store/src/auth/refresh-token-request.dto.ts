import { BadRequestException } from '@nestjs/common';

export interface RefreshTokenRequest {
  refreshToken: string;
}

export function parseRefreshTokenRequest(body: unknown): RefreshTokenRequest {
  if (typeof body !== 'object' || body === null) {
    throw new BadRequestException('Missing or invalid refreshToken');
  }

  const refreshToken = (body as Record<string, unknown>)['refreshToken'];
  if (typeof refreshToken !== 'string' || refreshToken.trim().length === 0) {
    throw new BadRequestException('Missing or invalid refreshToken');
  }

  return { refreshToken };
}
