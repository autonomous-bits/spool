/**
 * Vocabulary: ActorKind enum, per Meridian IDEA-75/IDEA-40. Closed to exactly these two values:
 * a stakeholder acting directly (`human`) or an AI/implementation agent acting on the
 * stakeholder's behalf (`delegated`). This is a business-logic-only distinction (delegated agents
 * may author, not merge/submit/verify, per IDEA-40) — it never changes how an `ActorContext` is
 * derived: every `ActorContext`, regardless of `kind`, must still be built only from verified
 * `SessionTokenClaims` (Meridian IDEA-139/G16 SG4), never a client-supplied field.
 */
export type ActorKind = 'human' | 'delegated';

const ACTOR_KINDS: readonly ActorKind[] = ['human', 'delegated'];

export function isActorKind(value: unknown): value is ActorKind {
  return typeof value === 'string' && (ACTOR_KINDS as readonly string[]).includes(value);
}

export function parseActorKind(value: unknown): ActorKind {
  if (!isActorKind(value)) {
    throw new TypeError(`Invalid ActorKind: ${JSON.stringify(value)}`);
  }
  return value;
}
