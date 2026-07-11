/**
 * Vocabulary: VerificationSignalStatus enum, per Meridian IDEA-21/IDEA-31 (promoted): a
 * dedicated agent, tool, or human reviewer logs a pass/fail evaluation against a submitted
 * branch. Recorded as feedback only -- IDEA-43 is explicit that a signal never auto-transitions
 * the branch's own status.
 */
export type VerificationSignalStatus = 'pass' | 'fail';

const VERIFICATION_SIGNAL_STATUSES: readonly VerificationSignalStatus[] = ['pass', 'fail'];

export function isVerificationSignalStatus(value: unknown): value is VerificationSignalStatus {
  return (
    typeof value === 'string' &&
    (VERIFICATION_SIGNAL_STATUSES as readonly string[]).includes(value)
  );
}

export function parseVerificationSignalStatus(value: unknown): VerificationSignalStatus {
  if (!isVerificationSignalStatus(value)) {
    throw new TypeError(`Invalid VerificationSignalStatus: ${JSON.stringify(value)}`);
  }
  return value;
}
