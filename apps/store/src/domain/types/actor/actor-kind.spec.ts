import { describe, expect, it } from 'vitest';
import { isActorKind, parseActorKind } from './actor-kind.js';

describe('ActorKind', () => {
  it.each(['human', 'delegated'])('accepts %s', (value) => {
    expect(isActorKind(value)).toBe(true);
    expect(parseActorKind(value)).toBe(value);
  });

  it.each(['agent', '', 'HUMAN', 123, null, undefined])('rejects %s', (value) => {
    expect(isActorKind(value)).toBe(false);
    expect(() => parseActorKind(value)).toThrow(TypeError);
  });
});
