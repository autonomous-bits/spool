import { describe, expect, it } from 'vitest';
import { isDiscipline, parseDiscipline } from './discipline.js';

describe('Discipline', () => {
  it.each([
    'product',
    'architecture',
    'design',
    'engineering',
    'security',
    'governance',
  ])('accepts %s', (value) => {
    expect(isDiscipline(value)).toBe(true);
    expect(parseDiscipline(value)).toBe(value);
  });

  it.each(['marketing', '', 'PRODUCT', 123, null, undefined])(
    'rejects %s',
    (value) => {
      expect(isDiscipline(value)).toBe(false);
      expect(() => parseDiscipline(value)).toThrow(TypeError);
    },
  );
});
