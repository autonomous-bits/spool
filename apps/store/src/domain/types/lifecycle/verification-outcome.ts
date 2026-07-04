/**
 * Outcome recorded by a single verification signal.
 *
 * Story S07 AC2: "A stakeholder can review passing, failing, or mixed
 * feedback before deciding what happens next."
 */
export type VerificationOutcome = 'passing' | 'failing' | 'mixed';
