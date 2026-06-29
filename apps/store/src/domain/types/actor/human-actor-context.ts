import type { ActorContext } from './actor-context.js';

export type HumanActorContext = ActorContext & { readonly kind: 'human' };
