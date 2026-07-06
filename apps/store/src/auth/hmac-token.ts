/**
 * Dependency-free HMAC-signed token codec, shared by `SessionTokenService` (Meridian IDEA-81's
 * store-issued session token) and `OAuthStateService` (self-contained CSRF `state` — the store
 * has no server-side session store, so `state` must carry its own integrity/expiry rather than
 * being looked up).
 *
 * Format: `${base64url(JSON payload)}.${base64url(HMAC-SHA256 signature)}`. Not a JWT: no
 * algorithm negotiation, no header, one fixed signing algorithm — deliberately smaller surface
 * area than a general-purpose JWT library for a single internal use case.
 */
import { createHmac, timingSafeEqual } from 'node:crypto';

export class InvalidHmacTokenError extends Error {
  constructor(reason: string) {
    super(`Invalid token: ${reason}`);
    this.name = 'InvalidHmacTokenError';
  }
}

interface EnvelopedPayload {
  claims: Record<string, unknown>;
  iat: number;
  exp: number;
}

function base64UrlEncode(value: string): string {
  return Buffer.from(value, 'utf8').toString('base64url');
}

function base64UrlDecode(value: string): string {
  return Buffer.from(value, 'base64url').toString('utf8');
}

function sign(payload: string, secret: string): string {
  return createHmac('sha256', secret).update(payload).digest('base64url');
}

function nowSeconds(): number {
  return Math.floor(Date.now() / 1000);
}

/**
 * Signs `claims` into a token that expires `maxAgeSeconds` after issuance.
 */
export function signHmacToken(
  claims: Record<string, unknown>,
  secret: string,
  maxAgeSeconds: number,
): string {
  const issuedAt = nowSeconds();
  const envelope: EnvelopedPayload = {
    claims,
    iat: issuedAt,
    exp: issuedAt + maxAgeSeconds,
  };
  const encodedPayload = base64UrlEncode(JSON.stringify(envelope));
  const signature = sign(encodedPayload, secret);
  return `${encodedPayload}.${signature}`;
}

function parseEnvelope(raw: string): EnvelopedPayload {
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    throw new InvalidHmacTokenError('payload is not valid JSON');
  }

  if (typeof parsed !== 'object' || parsed === null) {
    throw new InvalidHmacTokenError('payload is not an object');
  }

  const candidate = parsed as Record<string, unknown>;
  const claims = candidate.claims;
  const iat = candidate.iat;
  const exp = candidate.exp;

  if (typeof claims !== 'object' || claims === null) {
    throw new InvalidHmacTokenError('payload.claims is missing or not an object');
  }
  if (typeof iat !== 'number' || typeof exp !== 'number') {
    throw new InvalidHmacTokenError('payload.iat/exp are missing or not numbers');
  }

  return { claims: claims as Record<string, unknown>, iat, exp };
}

/**
 * Verifies signature and expiry, returning the enveloped claims. Throws `InvalidHmacTokenError`
 * on malformed input, signature mismatch, or expiry.
 */
export function verifyHmacToken(token: string, secret: string): Record<string, unknown> {
  const parts = token.split('.');
  if (parts.length !== 2) {
    throw new InvalidHmacTokenError('malformed token structure');
  }

  const [encodedPayload, signature] = parts;
  if (encodedPayload === undefined || encodedPayload === '' || signature === undefined) {
    throw new InvalidHmacTokenError('malformed token structure');
  }

  const expectedSignature = sign(encodedPayload, secret);
  const actual = Buffer.from(signature);
  const expected = Buffer.from(expectedSignature);
  if (actual.length !== expected.length || !timingSafeEqual(actual, expected)) {
    throw new InvalidHmacTokenError('signature mismatch');
  }

  const envelope = parseEnvelope(base64UrlDecode(encodedPayload));
  if (nowSeconds() >= envelope.exp) {
    throw new InvalidHmacTokenError('token expired');
  }

  return envelope.claims;
}
