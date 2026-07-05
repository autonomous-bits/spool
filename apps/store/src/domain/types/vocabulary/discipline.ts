/**
 * Vocabulary: Discipline enum, per Meridian IDEA-4/IDEA-14 (discipline provenance/attribution)
 * and the authoritative Postgres schema (IDEA-31). Closed to exactly these six values.
 */
export type Discipline =
  | 'product'
  | 'architecture'
  | 'design'
  | 'engineering'
  | 'security'
  | 'governance';

const DISCIPLINES: readonly Discipline[] = [
  'product',
  'architecture',
  'design',
  'engineering',
  'security',
  'governance',
];

export function isDiscipline(value: unknown): value is Discipline {
  return typeof value === 'string' && (DISCIPLINES as readonly string[]).includes(value);
}

export function parseDiscipline(value: unknown): Discipline {
  if (!isDiscipline(value)) {
    throw new TypeError(`Invalid Discipline: ${JSON.stringify(value)}`);
  }
  return value;
}
