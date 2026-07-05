import type { Branch } from './branch.js';
import type { ActorContext, HumanActorContext } from './types/actor/actor-context.js';

export class BranchLifecycleError extends Error {
  constructor(reason: string) {
    super(`Invalid branch lifecycle operation: ${reason}`);
    this.name = 'BranchLifecycleError';
  }
}

export function assertIsHumanActor(actor: ActorContext): asserts actor is HumanActorContext {
  if (actor.kind !== 'human') {
    throw new BranchLifecycleError(`expected human actor, received ${actor.kind}`);
  }
}

export function assertSubmitDiscipline(
  actor: ActorContext,
  branch: Pick<Branch, 'discipline'>,
): void {
  if (actor.discipline !== branch.discipline) {
    throw new BranchLifecycleError(
      `actor discipline ${actor.discipline} does not match branch discipline ${branch.discipline}`,
    );
  }
}

export function assertDraftStatus(branch: Pick<Branch, 'status'>): void {
  if (branch.status !== 'draft') {
    throw new BranchLifecycleError(`expected draft branch, received ${branch.status}`);
  }
}
