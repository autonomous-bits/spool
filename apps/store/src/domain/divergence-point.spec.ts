import { describe, expect, it } from 'vitest';
import { DivergencePoint } from './divergence-point.js';

describe('DivergencePoint', () => {
  it('defaults to now when constructed with no argument', () => {
    const before = Date.now();
    const point = new DivergencePoint();
    const after = Date.now();

    const value = new Date(point.toISOString()).getTime();
    expect(value).toBeGreaterThanOrEqual(before);
    expect(value).toBeLessThanOrEqual(after);
  });

  it('accepts a valid ISO-8601 timestamp and normalizes it', () => {
    const point = new DivergencePoint('2026-07-05T17:23:03.335Z');

    expect(point.toISOString()).toBe('2026-07-05T17:23:03.335Z');
    expect(point.toDate()).toEqual(new Date('2026-07-05T17:23:03.335Z'));
  });

  it.each(['', '   ', 'not-a-date', 'yesterday'])('rejects invalid timestamp %j', (value) => {
    expect(() => new DivergencePoint(value)).toThrow(TypeError);
  });
});
