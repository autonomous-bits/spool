/**
 * Vocabulary: ContextKind enum, ratified by Meridian IDEA-73. Every idea chunk carries a
 * `contextKind` of exactly `permanent` (durable, reusable knowledge) or `transient` (time-bound
 * or situational content).
 */
export type ContextKind = 'permanent' | 'transient';

const CONTEXT_KINDS: readonly ContextKind[] = ['permanent', 'transient'];

export function isContextKind(value: unknown): value is ContextKind {
  return typeof value === 'string' && (CONTEXT_KINDS as readonly string[]).includes(value);
}

export function parseContextKind(value: unknown): ContextKind {
  if (!isContextKind(value)) {
    throw new TypeError(`Invalid ContextKind: ${JSON.stringify(value)}`);
  }
  return value;
}
