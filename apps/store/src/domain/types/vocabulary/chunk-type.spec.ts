import { describe, expect, it } from 'vitest';
import { isChunkType, parseChunkType } from './chunk-type.js';

describe('ChunkType', () => {
  it.each(['feature', 'capability', 'constraint', 'adr', 'spike'])(
    'accepts %s',
    (value) => {
      expect(isChunkType(value)).toBe(true);
      expect(parseChunkType(value)).toBe(value);
    },
  );

  it.each(['epic', 'bug', '', 'FEATURE', 123, null, undefined])(
    'rejects %s',
    (value) => {
      expect(isChunkType(value)).toBe(false);
      expect(() => parseChunkType(value)).toThrow(TypeError);
    },
  );
});
