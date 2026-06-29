import type { ActorContext } from '../actor/actor-context.js';
import { isHumanActor } from '../actor/actor-context.js';
import type { HumanActorContext } from '../actor/human-actor-context.js';

export class HumanControlError extends Error {
  override readonly name = 'HumanControlError';
  readonly code = 'unauthorized-actor' as const;

  constructor(readonly reason: string) {
    super(reason);
  }
}

export function assertHumanActor(
  actor: ActorContext,
  operation: string,
): asserts actor is HumanActorContext {
  if (!isHumanActor(actor)) {
    throw new HumanControlError(
      `only a direct human stakeholder may ${operation}; delegated actors cannot perform this operation`,
    );
  }
}
