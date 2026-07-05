import { describe, expect, it, vi } from 'vitest';
import { InvalidHmacTokenError, signHmacToken, verifyHmacToken } from './hmac-token.js';

describe('hmac-token', () => {
  it('round-trips claims through sign and verify', () => {
    const token = signHmacToken({ hello: 'world' }, 'secret', 60);
    const claims = verifyHmacToken(token, 'secret');
    expect(claims).toEqual({ hello: 'world' });
  });

  it('rejects a token verified with the wrong secret', () => {
    const token = signHmacToken({ hello: 'world' }, 'secret', 60);
    expect(() => verifyHmacToken(token, 'wrong-secret')).toThrow(InvalidHmacTokenError);
  });

  it('rejects a tampered payload', () => {
    const token = signHmacToken({ hello: 'world' }, 'secret', 60);
    const [, signature] = token.split('.');
    const tamperedPayload = Buffer.from(JSON.stringify({ claims: { hello: 'tampered' }, iat: 0, exp: 9_999_999_999 }), 'utf8').toString('base64url');
    expect(() => verifyHmacToken(`${tamperedPayload}.${signature}`, 'secret')).toThrow(
      InvalidHmacTokenError,
    );
  });

  it('rejects a malformed token structure', () => {
    expect(() => verifyHmacToken('not-a-valid-token', 'secret')).toThrow(InvalidHmacTokenError);
  });

  it('rejects an expired token', () => {
    vi.useFakeTimers();
    try {
      vi.setSystemTime(0);
      const token = signHmacToken({ hello: 'world' }, 'secret', 60);
      vi.setSystemTime(61_000);
      expect(() => verifyHmacToken(token, 'secret')).toThrow(InvalidHmacTokenError);
    } finally {
      vi.useRealTimers();
    }
  });
});
