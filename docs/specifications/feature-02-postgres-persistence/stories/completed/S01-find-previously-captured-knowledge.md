# S01: Find previously captured ideas and relationships after time away

## Business value

Stakeholders and agents need the workspace's ideas and their relationships to survive beyond a
single chat session so returning later does not mean starting over. Without durable storage,
approved and draft ideas and the relationships between them would vanish when a session ends.

## Fidelity references

- Functional spec: `docs/specifications/feature-02-postgres-persistence/functional-specification.md`
- Technical spec: `docs/specifications/feature-02-postgres-persistence/technical-specification.md`
- Meridian workspace: `dbb786ac-1d61-41c9-a46a-2c279dd50cc3`
- Meridian starting points and neighbourhoods: `IDEA-31` / `1b0bd656-e365-4c79-a85e-e52066895dc5`

If Meridian, the functional spec, and this story disagree, Meridian wins.

## Acceptance criteria

1. A stakeholder can return to a workspace after the originating chat session has ended and find
   previously captured ideas and relationships unchanged.
2. A stakeholder can identify current approved ideas and relationships in a workspace after a
   restart of the store.
3. An implementation agent can request a workspace's ideas and relationships without needing an
   active session or in-memory cache to have survived.

## Deliverable

A persistence adapter in `apps/store` that durably stores the versioned chunk and edge graph so
it survives process restarts, plus adapter-level tests proving data written in one process
lifetime is readable in a subsequent one. Content must align with the "Store owns persistence"
row of the technical specification and `IDEA-31`.

## Out of scope

Delta-based branch storage, conflict detection, merge transactions, suggestion/notification
persistence, and delivery subscriptions are out of scope for this story.
