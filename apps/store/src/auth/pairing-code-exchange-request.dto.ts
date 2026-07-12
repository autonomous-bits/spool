import { BadRequestException } from '@nestjs/common';

export interface PairingCodeExchangeRequest {
  code: string;
}

export function parsePairingCodeExchangeRequest(body: unknown): PairingCodeExchangeRequest {
  if (typeof body !== 'object' || body === null) {
    throw new BadRequestException('Missing or invalid code');
  }

  const code = (body as Record<string, unknown>).code;
  if (typeof code !== 'string' || code.trim().length === 0) {
    throw new BadRequestException('Missing or invalid code');
  }

  return { code };
}
