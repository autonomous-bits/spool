import type { StakeholderId } from '../identifiers/stakeholder-id.js';
import type { DelegatedActorContext } from './delegated-actor-context.js';
import type { HumanActorContext } from './human-actor-context.js';
import type { ActorKind } from './actor-kind.js';

export interface ActorContext {
  readonly kind: ActorKind;
  readonly stakeholderId: StakeholderId;
}

export function humanActor(id: StakeholderId): HumanActorContext {
  return { kind: 'human', stakeholderId: id };
}

export function delegatedActor(id: StakeholderId): DelegatedActorContext {
  return { kind: 'delegated', stakeholderId: id };
}

export function isHumanActor(actor: ActorContext): actor is HumanActorContext {
  return actor.kind === 'human';
}

export function isDelegatedActor(
  actor: ActorContext,
): actor is DelegatedActorContext {
  return actor.kind === 'delegated';
}
