import { describe, expect, it } from 'vitest';
import { isContextKind, parseContextKind } from './context-kind.js';

describe('ContextKind', () => {
  it.each(['permanent', 'transient'])('accepts %s', (value) => {
    expect(isContextKind(value)).toBe(true);
    expect(parseContextKind(value)).toBe(value);
  });

  it.each(['temporary', '', 'PERMANENT', 123, null, undefined])(
    'rejects %s',
    (value) => {
      expect(isContextKind(value)).toBe(false);
      expect(() => parseContextKind(value)).toThrow(TypeError);
    },
  );
});
