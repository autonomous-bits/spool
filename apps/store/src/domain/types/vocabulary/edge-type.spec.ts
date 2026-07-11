import { describe, expect, it } from 'vitest';
import { isEdgeType, parseEdgeType } from './edge-type.js';

const VALID_TYPES = [
  'refines',
  'depends-on',
  'contradicts',
  'derives-from',
  'blocks',
  'implements',
  'constrains',
  'feedback-on',
] as const;

describe('EdgeType', () => {
  it.each(VALID_TYPES)('accepts valid type %j', (type) => {
    expect(isEdgeType(type)).toBe(true);
    expect(parseEdgeType(type)).toBe(type);
  });

  it('rejects an unknown type', () => {
    expect(isEdgeType('relates-to')).toBe(false);
    expect(() => parseEdgeType('relates-to')).toThrow(TypeError);
  });

  it('rejects a non-string value', () => {
    expect(isEdgeType(42)).toBe(false);
    expect(() => parseEdgeType(undefined)).toThrow(TypeError);
  });
});
