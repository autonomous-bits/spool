import type { Discipline } from '../vocabulary/discipline.js';
import type { ActorKind } from './actor-kind.js';

type BaseActorContext = Readonly<{
  kind: ActorKind;
  stakeholderId: string;
  discipline: Discipline | null;
}>;

/**
 * Authenticated actor context for branch/domain writes. Every `ActorContext` must be constructed
 * only from a verified `SessionTokenClaims` (Meridian IDEA-139/G16 SG4): `stakeholderId` is always
 * `claims.stakeholderId`, `discipline` is always looked up server-side against the `stakeholders`
 * table for that id (never a request body/query field), and `kind` is always assigned by trusted
 * server execution flow (`'human'` for every current call site — `BranchesService.submit`/
 * `resolveActorForVerification` and `SuggestionsService.accept`/`reject`, both via the shared
 * `resolveHumanActorContext` helper in `auth/resolve-human-actor.helper.ts`). No call site may ever
 * hydrate this type from a client-supplied `stakeholderId`, `discipline`, or `actorKind` field.
 */
export type ActorContext = HumanActorContext | DelegatedActorContext;

export type HumanActorContext = BaseActorContext & { kind: 'human' };

export type DelegatedActorContext = BaseActorContext & { kind: 'delegated' };
