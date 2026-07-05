/**
 * DivergencePoint: an opaque, validated ISO-8601 timestamp recording the mainline state at the
 * moment a branch was created (`diverged_at`), per Meridian IDEA-41 and IDEA-74's authoritative
 * shape. Defaults to "now" at construction. This goal only needs the value type itself;
 * MergeLineage/BranchGraphProvenance (also defined by IDEA-74) are deferred to a later
 * merge-related goal.
 */
export class DivergencePoint {
  private readonly isoString: string;

  constructor(value?: string) {
    const candidate = value ?? new Date().toISOString();
    const parsed = new Date(candidate);

    if (candidate.trim().length === 0 || Number.isNaN(parsed.getTime())) {
      throw new TypeError(`Invalid DivergencePoint: ${JSON.stringify(candidate)}`);
    }

    this.isoString = parsed.toISOString();
  }

  toISOString(): string {
    return this.isoString;
  }

  toDate(): Date {
    return new Date(this.isoString);
  }
}
