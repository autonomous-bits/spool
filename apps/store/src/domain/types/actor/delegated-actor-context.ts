import type { ActorContext } from './actor-context.js';

export type DelegatedActorContext = ActorContext & {
  readonly kind: 'delegated';
};
