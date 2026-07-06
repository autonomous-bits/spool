import type { Discipline } from '../vocabulary/discipline.js';
import type { ActorKind } from './actor-kind.js';

type BaseActorContext = Readonly<{
  kind: ActorKind;
  stakeholderId: string;
  discipline: Discipline | null;
}>;

/**
 * Authenticated actor context for branch/domain writes. `stakeholderId` and `discipline` must come
 * from SG0's verified session-token claims after server-side validation/narrowing, while `kind`
 * must be assigned by trusted server execution flow. Never hydrate this type from client-supplied
 * body fields such as `discipline` or `actorKind`.
 */
export type ActorContext = HumanActorContext | DelegatedActorContext;

export type HumanActorContext = BaseActorContext & { kind: 'human' };

export type DelegatedActorContext = BaseActorContext & { kind: 'delegated' };
